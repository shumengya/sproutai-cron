"""
飞书 / Lark Markdown 通知 + 任务结果汇总。
内置 lark-notice-api 与 mengya-mail-api，无需外部 subprocess。
"""

from __future__ import annotations

import os
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING, Callable

if TYPE_CHECKING:
    from runner import TaskContext

CRON_FEISHU_WEBHOOK_DEFAULT = (
    ""
)

_VENDOR_ROOT = Path(__file__).resolve().parent / "vendor"
_LARK_SRC = _VENDOR_ROOT / "lark-notice-api" / "src"
_MAIL_SRC = _VENDOR_ROOT / "mengya-mail-api" / "src"
_MAIL_ENV_DEFAULT = _VENDOR_ROOT / "mengya-mail-api" / ".env"


def _ensure_vendor_imports() -> None:
    for src in (_LARK_SRC, _MAIL_SRC):
        text = str(src)
        if src.is_dir() and text not in sys.path:
            sys.path.insert(0, text)


@dataclass
class TaskResult:
    """任务执行结果，汇总 ok / skip / fail 三类条目。"""

    ok: list[str] = field(default_factory=list)
    skip: list[str] = field(default_factory=list)
    fail: list[str] = field(default_factory=list)

    def add_ok(self, item: str) -> None:
        self.ok.append(item)

    def add_skip(self, item: str) -> None:
        self.skip.append(item)

    def add_fail(self, item: str) -> None:
        self.fail.append(item)

    @property
    def success(self) -> bool:
        return not self.fail

    @property
    def status_cn(self) -> str:
        return "成功" if self.success else "失败"

    @property
    def total(self) -> int:
        return len(self.ok) + len(self.skip) + len(self.fail)


def markdown_list(items: list[str]) -> str:
    """将多行条目格式化为 Markdown 列表。"""
    if not items:
        return "- 无"
    return "\n".join(f"- {x}" for x in items if x)


def markdown_fields(items: list[tuple[str, object]]) -> str:
    """将键值对格式化为更简洁的 Markdown 列表。"""
    if not items:
        return "- 无"
    lines: list[str] = []
    for key, value in items:
        text = "-" if value is None or value == "" else str(value)
        lines.append(f"- **{key}**：{text}")
    return "\n".join(lines)


def send_task_summary(
    ctx: "TaskContext",
    result: TaskResult,
    start_time: str,
    *,
    log: Callable[[str], None],
    extra_fields: list[tuple[str, object]] | None = None,
) -> None:
    """发送标准任务汇总通知（飞书 + 邮件降级）。"""
    from runner import task_output_prefix

    end_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    subject = f"{task_output_prefix(ctx.task_id)} {result.status_cn} {end_time}"

    fields: list[tuple[str, object]] = [
        ("任务", ctx.task_id),
        ("开始", start_time),
        ("结束", end_time),
    ]
    if extra_fields:
        fields.extend(extra_fields)
    fields.extend([
        ("总数", result.total),
        ("成功", len(result.ok)),
        ("跳过", len(result.skip)),
        ("失败", len(result.fail)),
    ])

    md = "\n".join([
        markdown_fields(fields),
        "", "**成功列表**", markdown_list(result.ok),
        "", "**跳过列表**", markdown_list(result.skip),
        "", "**失败列表**", markdown_list(result.fail),
    ])
    send_feishu_markdown(subject, md, log=log)


def send_feishu_markdown(
    title: str,
    markdown: str,
    *,
    log: Callable[[str], None],
) -> None:
    """发送飞书 Markdown（失败时自动降级为邮件通知）。"""
    if not title or not markdown:
        log("飞书通知内容为空，跳过。")
        return

    webhook = os.environ.get("LARK_NOTICE_WEBHOOK", CRON_FEISHU_WEBHOOK_DEFAULT)
    if not webhook:
        log("未配置飞书 webhook，跳过飞书通知。")
        return

    _ensure_vendor_imports()
    from lark_notice_api.client import LarkNoticeError, LarkWebhookClient

    try:
        LarkWebhookClient(webhook).send_markdown(title, markdown)
        return
    except LarkNoticeError as exc:
        log(f"发送飞书通知失败：{exc}")
    except ImportError:
        log("发送飞书通知失败：内置 lark-notice-api 不可用。")

    log("飞书通知失败，尝试通过邮件发送…")
    _send_mail_fallback(title, markdown, log)


def _send_mail_fallback(title: str, markdown: str, log: Callable[[str], None]) -> None:
    """通过内置 mengya-mail-api 发送邮件（失败仅记日志）。"""
    enabled = os.environ.get("CRON_MAIL_ENABLED", "0")
    if enabled != "1":
        log("邮件通知未开启（export CRON_MAIL_ENABLED=1 可启用），跳过。")
        return

    env_file = os.environ.get("MAIL_API_ENV_FILE", str(_MAIL_ENV_DEFAULT))
    if env_file and os.path.isfile(env_file):
        os.environ.setdefault("MENGYA_MAIL_ENV_FILE", env_file)

    mail_to = os.environ.get("MAIL_TO", "mail@smyhub.com")
    from_name = os.environ.get("MAIL_FROM_NAME", "cron")

    _ensure_vendor_imports()
    from mengya_mail_api.config import ConfigError, MailConfig
    from mengya_mail_api.email_client import MailClient, MailClientError

    try:
        config = MailConfig.from_env()
        MailClient(config).send_email(
            to=mail_to,
            subject=title,
            html_body=markdown,
            from_name=from_name,
        )
        log("邮件通知发送成功。")
    except ConfigError as exc:
        log(f"邮件配置错误：{exc}")
    except MailClientError as exc:
        log(f"邮件通知发送失败：{exc}")
    except ImportError:
        log("邮件通知发送失败：内置 mengya-mail-api 不可用。")

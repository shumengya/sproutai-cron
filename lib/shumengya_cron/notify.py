"""
飞书 / Lark Markdown 通知 + 任务结果汇总。
"""

from __future__ import annotations

import os
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime
from typing import TYPE_CHECKING, Callable

if TYPE_CHECKING:
    from shumengya_cron.runner import TaskContext

CRON_FEISHU_WEBHOOK_DEFAULT = (
    ""
)
CRON_LARK_NOTICE_API_SRC_DEFAULT = "/shumengya/project/python/lark-notice-api/src"


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
    """发送标准任务汇总通知（飞书 + 邮件降级）。

    extra_fields 会插入在 结束时间 和 总数 之间，适合放主机名、目标路径等任务特有字段。
    """
    from shumengya_cron.runner import task_output_prefix

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
    """发送飞书 Markdown（失败时自动降级为邮件通知，邮件也失败则结束）。"""
    if not title or not markdown:
        log("飞书通知内容为空，跳过。")
        return

    webhook = os.environ.get("LARK_NOTICE_WEBHOOK", CRON_FEISHU_WEBHOOK_DEFAULT)
    api_src = os.environ.get("LARK_NOTICE_API_SRC", CRON_LARK_NOTICE_API_SRC_DEFAULT)
    if not webhook:
        log("未配置飞书 webhook，跳过飞书通知。")
        return

    env = os.environ.copy()
    pp = api_src
    if env.get("PYTHONPATH"):
        pp = f"{api_src}:{env['PYTHONPATH']}"
    env["PYTHONPATH"] = pp

    feishu_ok = False
    try:
        r = subprocess.run(
            [
                sys.executable,
                "-m",
                "lark_notice_api",
                "--webhook",
                webhook,
                "send-markdown",
                "--title",
                title,
                "--markdown",
                markdown,
            ],
            cwd=api_src if os.path.isdir(api_src) else None,
            env=env,
            check=False,
            capture_output=True,
            text=True,
        )
        if r.returncode == 0:
            feishu_ok = True
        else:
            detail = (r.stderr or r.stdout or "").strip()
            if detail:
                log(f"发送飞书通知失败（退出码 {r.returncode}）：{detail}")
            else:
                log(f"发送飞书通知失败（退出码 {r.returncode}）。")
    except OSError:
        log("发送飞书通知失败：lark-notice-api 调用异常。")

    if feishu_ok:
        return

    log("飞书通知失败，尝试通过邮件发送…")
    _send_mail_fallback(title, markdown, log)


# ── 邮件降级 ──────────────────────────────────────────────

_MAIL_SCRIPT_DEFAULT = "/shumengya/project/skills/mengya-mail-skills/scripts/mengya-mail-api.py"
_MAIL_ENV_FILE_DEFAULT = "/shumengya/project/python/mengya-mail-api/.env"


def _send_mail_fallback(title: str, markdown: str, log: Callable[[str], None]) -> None:
    """通过 mengya-mail-api 发送报告邮件（失败仅记日志，不再继续降级）。"""
    enabled = os.environ.get("CRON_MAIL_ENABLED", "0")
    if enabled != "1":
        log("邮件通知未开启（export CRON_MAIL_ENABLED=1 可启用），跳过。")
        return

    script = os.environ.get("MAIL_API_SCRIPT", _MAIL_SCRIPT_DEFAULT)
    env_file = os.environ.get("MAIL_API_ENV_FILE", _MAIL_ENV_FILE_DEFAULT)
    mail_to = os.environ.get("MAIL_TO", "mail@smyhub.com")
    from_name = os.environ.get("MAIL_FROM_NAME", "cron")

    if not os.path.isfile(script):
        log(f"邮件脚本不存在：{script}，跳过邮件通知。")
        return

    try:
        r = subprocess.run(
            [sys.executable, script, "--env-file", env_file, "--format", "json",
             "send-email", "--to", mail_to, "--subject", title,
             "--from-name", from_name, "--html-body", markdown],
            capture_output=True, text=True, check=False,
        )
        if r.returncode == 0:
            log("邮件通知发送成功。")
        else:
            detail = (r.stderr or r.stdout or "").strip()
            log(f"邮件通知发送失败（退出码 {r.returncode}）：{detail or '无详细信息'}")
    except OSError as exc:
        log(f"邮件通知发送异常：{exc}")

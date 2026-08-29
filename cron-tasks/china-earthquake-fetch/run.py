"""
抓取中国地震台网最新地震信息。

数据源：https://www.ceic.ac.cn/data/data.json
每天 08:00 汇总过去 24 小时内的地震并写入日志，可选飞书通知。

禁用 / 互斥锁 / 日志由 cronctl（Go）统一处理。
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any, Callable
from urllib import error, request

CEIC_DATA_URL = os.environ.get(
    "CEIC_DATA_URL",
    "https://www.ceic.ac.cn/data/data.json",
)
LOOKBACK_HOURS = int(os.environ.get("CEIC_LOOKBACK_HOURS", "24"))
MAX_ITEMS = int(os.environ.get("CEIC_MAX_ITEMS", "20"))
DEFAULT_WEBHOOK = (
    ""
)


@dataclass
class TaskResult:
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


def log(msg: str) -> None:
    print(msg, flush=True)


def send_feishu_markdown(title: str, markdown: str) -> None:
    if not title or not markdown:
        log("飞书通知内容为空，跳过。")
        return
    webhook = os.environ.get("LARK_NOTICE_WEBHOOK", DEFAULT_WEBHOOK)
    if not webhook:
        log("未配置飞书 webhook，跳过飞书通知。")
        return
    text = markdown.replace("\r\n", "\n").replace("\r", "\n")
    payload = {
        "msg_type": "interactive",
        "card": {
            "schema": "2.0",
            "config": {"wide_screen_mode": True, "enable_forward": True},
            "header": {"title": {"tag": "plain_text", "content": title}},
            "body": {"elements": [{"tag": "markdown", "content": text}]},
        },
    }
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = request.Request(
        webhook,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with request.urlopen(req, timeout=10) as resp:
            body = resp.read().decode("utf-8", errors="replace")
        result = json.loads(body)
        if result.get("code") not in (0, None):
            log(f"发送飞书通知失败：code={result.get('code')} msg={result.get('msg')}")
            return
        log("飞书通知发送成功。")
    except Exception as exc:  # noqa: BLE001
        log(f"发送飞书通知失败：{exc}")


def send_task_summary(
    task_id: str,
    result: TaskResult,
    start_time: str,
    *,
    extra_fields: list[tuple[str, object]] | None = None,
) -> None:
    end_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    subject = f"[{task_id}] {result.status_cn} {end_time}"
    fields: list[tuple[str, object]] = [
        ("任务", task_id),
        ("开始", start_time),
        ("结束", end_time),
    ]
    if extra_fields:
        fields.extend(extra_fields)
    fields.extend(
        [
            ("总数", result.total),
            ("成功", len(result.ok)),
            ("跳过", len(result.skip)),
            ("失败", len(result.fail)),
        ]
    )

    def md_fields(items: list[tuple[str, object]]) -> str:
        return "\n".join(f"- **{k}**：{v if v not in (None, '') else '-'}" for k, v in items)

    def md_list(items: list[str]) -> str:
        return "\n".join(f"- {x}" for x in items) if items else "- 无"

    md = "\n".join(
        [
            md_fields(fields),
            "",
            "**成功列表**",
            md_list(result.ok),
            "",
            "**跳过列表**",
            md_list(result.skip),
            "",
            "**失败列表**",
            md_list(result.fail),
        ]
    )
    send_feishu_markdown(subject, md)


def _parse_time(value: str) -> datetime | None:
    for fmt in ("%Y-%m-%d %H:%M:%S", "%Y/%m/%d %H:%M:%S"):
        try:
            return datetime.strptime(value, fmt)
        except ValueError:
            continue
    return None


def fetch_earthquakes(log_fn: Callable[[str], None]) -> list[dict[str, Any]]:
    req = request.Request(
        CEIC_DATA_URL,
        headers={"User-Agent": "sproutai-cron/1.0"},
        method="GET",
    )
    try:
        with request.urlopen(req, timeout=30) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
    except error.URLError as exc:
        raise RuntimeError(f"请求地震数据失败：{exc}") from exc

    if not isinstance(payload, list):
        raise RuntimeError("地震数据格式异常：期望 JSON 数组")

    log_fn(f"已获取 {len(payload)} 条地震记录")
    return payload


def filter_recent(events: list[dict[str, Any]], *, hours: int) -> list[dict[str, Any]]:
    cutoff = datetime.now() - timedelta(hours=hours)
    recent: list[tuple[datetime, dict[str, Any]]] = []

    for item in events:
        when = _parse_time(str(item.get("time", "")))
        if when is None or when < cutoff:
            continue
        recent.append((when, item))

    recent.sort(key=lambda pair: pair[0], reverse=True)
    return [item for _, item in recent[:MAX_ITEMS]]


def format_event(item: dict[str, Any]) -> str:
    return (
        f"{item.get('time')} M{item.get('magnitude')} "
        f"{item.get('location')} "
        f"({item.get('latitude')}, {item.get('longitude')}) "
        f"深度{item.get('depth')}km"
    )


def main() -> int:
    task_id = os.environ.get(
        "CRON_TASK_ID",
        os.path.basename(os.path.dirname(os.path.abspath(__file__))),
    )
    start_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    result = TaskResult()

    try:
        events = fetch_earthquakes(log)
        recent = filter_recent(events, hours=LOOKBACK_HOURS)
        log(f"过去 {LOOKBACK_HOURS} 小时内地震 {len(recent)} 条")

        if not recent:
            result.add_skip(f"过去 {LOOKBACK_HOURS} 小时无新地震记录")
            log("no recent earthquakes")
        else:
            for item in recent:
                line = format_event(item)
                result.add_ok(line)
                log(line)

        send_task_summary(
            task_id,
            result,
            start_time,
            extra_fields=[
                ("数据源", CEIC_DATA_URL),
                ("统计窗口", f"过去 {LOOKBACK_HOURS} 小时"),
            ],
        )
        return 0 if result.success else 1
    except Exception as exc:  # noqa: BLE001
        result.add_fail(str(exc))
        log(f"error: {exc}")
        send_task_summary(task_id, result, start_time)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

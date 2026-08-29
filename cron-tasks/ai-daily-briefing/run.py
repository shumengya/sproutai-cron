"""
AI 早报：按日期拉取 https://daily.juya.uk/markdown/YYYY-MM-DD.md，
排版后写入日志并发送飞书通知。

默认每天下午 14:00；URL 使用本地「当天」日期。
发送前强制核对：目标日期 == URL 日期 == 正文标题日期，不一致则拒发。

禁用 / 互斥锁 / 日志由 cronctl（Go）统一处理。
"""

from __future__ import annotations

import json
import os
import re
from datetime import date, datetime, timedelta
from typing import Callable
from urllib import error, request

BASE_URL = os.environ.get(
    "AI_DAILY_BASE_URL",
    "https://daily.juya.uk/markdown",
).rstrip("/")
# 覆盖日期，如 AI_DAILY_DATE=2026-07-22（调试用）；正式运行勿设，始终用当天
DATE_OVERRIDE = os.environ.get("AI_DAILY_DATE", "").strip()
# 仅当当天 404 时，是否允许改用昨天（默认否，避免误发旧报）
ALLOW_YESTERDAY = os.environ.get("AI_DAILY_ALLOW_YESTERDAY", "0").strip() == "1"
# full=全文清理后发送；overview=仅标题+概览+原文链接
MODE = os.environ.get("AI_DAILY_MODE", "full").strip().lower()
MAX_CHARS = int(os.environ.get("AI_DAILY_MAX_CHARS", "12000"))
USER_AGENT = os.environ.get("AI_DAILY_UA", "sproutai-cron/ai-daily-briefing")
DEFAULT_WEBHOOK = (
    ""
)


def log(msg: str) -> None:
    print(msg, flush=True)


def send_feishu_markdown(title: str, markdown: str, *, log: Callable[[str], None]) -> None:
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


# 从正文解析日期：# AI 早报 2026-07-21 / AI早报 2026年7月21日 等
_CONTENT_DATE_PATTERNS = (
    re.compile(
        r"AI\s*早报\s*[:：]?\s*(\d{4})-(\d{1,2})-(\d{1,2})",
        re.IGNORECASE,
    ),
    re.compile(
        r"AI\s*早报\s*[:：]?\s*(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日",
        re.IGNORECASE,
    ),
    re.compile(
        r"^#\s*.*?(\d{4})-(\d{1,2})-(\d{1,2})",
        re.MULTILINE,
    ),
)


def _resolve_target_day() -> date:
    if DATE_OVERRIDE:
        return datetime.strptime(DATE_OVERRIDE, "%Y-%m-%d").date()
    return date.today()


def markdown_url(day: date) -> str:
    return f"{BASE_URL}/{day.isoformat()}.md"


def fetch_markdown(url: str) -> str:
    req = request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/markdown,text/plain,*/*",
        },
        method="GET",
    )
    try:
        with request.urlopen(req, timeout=45) as resp:
            charset = resp.headers.get_content_charset() or "utf-8"
            return resp.read().decode(charset, errors="replace")
    except error.HTTPError as exc:
        if exc.code == 404:
            raise FileNotFoundError(f"早报不存在（404）: {url}") from exc
        raise RuntimeError(f"拉取早报失败 HTTP {exc.code}: {url}") from exc
    except error.URLError as exc:
        raise RuntimeError(f"拉取早报网络错误: {exc}") from exc


def extract_content_date(raw: str) -> date | None:
    """从 Markdown 正文提取期号日期。"""
    # 优先扫前 30 行（标题区）
    head = "\n".join(raw.replace("\r\n", "\n").split("\n")[:30])
    for pattern in _CONTENT_DATE_PATTERNS:
        m = pattern.search(head)
        if not m:
            continue
        y, mo, d = int(m.group(1)), int(m.group(2)), int(m.group(3))
        try:
            return date(y, mo, d)
        except ValueError:
            continue
    return None


def assert_dates_match(
    *,
    local_today: date,
    target_day: date,
    url_day: date,
    content_day: date | None,
    source_url: str,
    log: Callable[[str], None],
) -> None:
    """发送前三重日期核对；任一不一致则抛错拒发。"""
    log(
        "日期核对: "
        f"本地今天={local_today.isoformat()} "
        f"目标={target_day.isoformat()} "
        f"URL={url_day.isoformat()} "
        f"正文={content_day.isoformat() if content_day else '未解析到'}"
    )
    log(f"源地址: {source_url}")

    if url_day != target_day:
        raise RuntimeError(
            f"日期不一致：URL 日期 {url_day} ≠ 目标日期 {target_day}，拒发"
        )
    if content_day is None:
        raise RuntimeError(
            f"正文未解析到日期（标题应含 AI 早报 YYYY-MM-DD），"
            f"目标={target_day}，拒发以免错期"
        )
    if content_day != target_day:
        raise RuntimeError(
            f"日期不一致：正文标题日期 {content_day} ≠ 目标日期 {target_day}，拒发"
        )
    if not DATE_OVERRIDE and target_day != local_today:
        raise RuntimeError(
            f"日期不一致：目标 {target_day} ≠ 本地今天 {local_today}，拒发"
            f"（未设置 AI_DAILY_DATE 时只允许发当天）"
        )
    log(
        f"日期核对通过：{target_day.isoformat()} "
        f"(today={local_today.isoformat()}, override={DATE_OVERRIDE or '无'})"
    )


def _collapse_blank_lines(text: str) -> str:
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip() + "\n"


def _strip_images(text: str) -> str:
    return re.sub(r"!\[[^\]]*\]\([^)]+\)\s*", "", text)


def _extract_overview(text: str) -> str:
    lines = text.replace("\r\n", "\n").split("\n")
    head: list[str] = []
    overview: list[str] = []
    in_overview = False
    saw_title = False

    for line in lines:
        if line.startswith("# ") and not saw_title:
            head.append(line)
            saw_title = True
            continue
        if not in_overview and ("视频版" in line or line.startswith("**视频版**")):
            head.append(line)
            continue
        if re.match(r"^##\s+概览\s*$", line.strip()):
            in_overview = True
            overview.append(line)
            continue
        if in_overview:
            if re.match(r"^##\s+\S", line) and not re.match(r"^##\s+概览", line.strip()):
                break
            overview.append(line)

    parts = [p for p in ("\n".join(head).strip(), "\n".join(overview).strip()) if p]
    return "\n\n".join(parts) + "\n" if parts else text


def format_daily_markdown(raw: str, *, day: date, source_url: str) -> str:
    body = _strip_images(raw)
    body = _collapse_blank_lines(body)

    if MODE == "overview":
        body = _extract_overview(body)
        body = _collapse_blank_lines(body)

    header = "\n".join(
        [
            f"**AI 早报 · {day.isoformat()}**",
            f"原文：[{source_url}]({source_url})",
            "",
            "---",
            "",
        ]
    )
    formatted = header + body

    if len(formatted) > MAX_CHARS:
        cut = formatted[: MAX_CHARS - 80].rstrip()
        nl = cut.rfind("\n\n")
        if nl > MAX_CHARS // 2:
            cut = cut[:nl]
        formatted = (
            cut
            + "\n\n---\n\n"
            + f"> 内容过长已截断，完整版见原文：{source_url}\n"
        )
    return formatted


def _fetch_for_day(day: date, log: Callable[[str], None]) -> tuple[str, str]:
    url = markdown_url(day)
    log(f"拉取: {url}")
    raw = fetch_markdown(url)
    return raw, url


def main() -> int:
    task_id = os.environ.get(
        "CRON_TASK_ID",
        os.path.basename(os.path.dirname(os.path.abspath(__file__))),
    )
    local_today = date.today()
    target = _resolve_target_day()
    log(
        f"调度日核对: 本地今天={local_today.isoformat()} "
        f"目标期号={target.isoformat()} "
        f"AI_DAILY_DATE={DATE_OVERRIDE or '(未设置→用今天)'}"
    )

    try:
        if not DATE_OVERRIDE and target != local_today:
            raise RuntimeError(f"目标日期 {target} 不是今天 {local_today}，拒发")

        raw: str | None = None
        source = ""
        used_day = target

        try:
            raw, source = _fetch_for_day(target, log)
        except FileNotFoundError as exc:
            log(str(exc))
            if ALLOW_YESTERDAY and not DATE_OVERRIDE and target == local_today:
                used_day = local_today - timedelta(days=1)
                log(
                    f"当天 404，ALLOW_YESTERDAY=1，改试昨天 {used_day.isoformat()}"
                )
                raw, source = _fetch_for_day(used_day, log)
                target = used_day
            else:
                raise RuntimeError(
                    f"今天 {local_today.isoformat()} 的早报尚不存在，"
                    f"拒发旧报。URL: {markdown_url(target)}"
                ) from exc

        assert raw is not None
        content_day = extract_content_date(raw)
        url_day = used_day

        assert_dates_match(
            local_today=local_today,
            target_day=target,
            url_day=url_day,
            content_day=content_day,
            source_url=source,
            log=log,
        )

        body = format_daily_markdown(raw, day=target, source_url=source)
        title = f"AI 早报 {target.isoformat()}"
        log(f"排版完成 mode={MODE} chars={len(body)} day={target}")
        for line in body.splitlines()[:40]:
            log(line)
        if body.count("\n") > 40:
            log(f"... 共 {len(body.splitlines())} 行")

        log(f"日期已通过，准备发送飞书：{title}")
        send_feishu_markdown(title, body, log=log)
        return 0
    except Exception as exc:  # noqa: BLE001
        log(f"error: {exc}")
        send_feishu_markdown(
            "AI 早报 失败（未发送正文）",
            "\n".join(
                [
                    f"- **任务**：`{task_id}`",
                    f"- **本地今天**：{local_today.isoformat()}",
                    f"- **错误**：{exc}",
                    "",
                    "已拦截发送，避免错期早报。",
                ]
            ),
            log=log,
        )
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

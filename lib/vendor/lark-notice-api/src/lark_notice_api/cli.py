from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any

from .client import LarkNoticeError, LarkWebhookClient


def _split_global_args(argv: list[str]) -> tuple[list[str], str | None, float | None]:
    cleaned: list[str] = []
    webhook: str | None = None
    timeout: float | None = None
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--webhook" and i + 1 < len(argv):
            webhook = argv[i + 1]
            i += 2
            continue
        if arg == "--timeout" and i + 1 < len(argv):
            timeout = float(argv[i + 1])
            i += 2
            continue
        cleaned.append(arg)
        i += 1
    return cleaned, webhook, timeout


def _client_from_args(args: argparse.Namespace) -> LarkWebhookClient:
    webhook = args.webhook or os.getenv("LARK_NOTICE_WEBHOOK", "")
    if not webhook:
        raise LarkNoticeError("缺少 webhook，请传 --webhook 或设置 LARK_NOTICE_WEBHOOK")
    return LarkWebhookClient(webhook=webhook, timeout=args.timeout)


def _normalize_cli_markdown(markdown: str) -> str:
    if "\n" in markdown or "\r" in markdown:
        return markdown
    return markdown.replace(r"\n", "\n").replace(r"\r", "\r")


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="lark-notice-api")
    parser.add_argument("--webhook", help="Feishu webhook URL")
    parser.add_argument("--timeout", type=float, default=10.0, help="HTTP timeout seconds")

    sub = parser.add_subparsers(dest="command", required=True)

    text = sub.add_parser("send-text", help="Send plain text message")
    text.add_argument("--text", required=True)

    post = sub.add_parser("send-post", help="Send Feishu post message")
    post.add_argument("--title", required=True)
    post.add_argument("--line", action="append", dest="lines", required=True)

    md = sub.add_parser("send-markdown", help="Send interactive markdown card")
    md.add_argument("--title", required=True)
    md.add_argument("--markdown", required=True)

    demo = sub.add_parser("demo", help="Send a markdown demo card")
    demo.add_argument("--title", default="飞书消息示例")
    demo.add_argument(
        "--markdown",
        default=(
            "# 飞书 Markdown 通知测试\n"
            "\n"
            "这是一条 **JSON 2.0** 富文本消息。\n"
            "\n"
            "- 支持 *斜体*、**粗体**、~~删除线~~\n"
            "- [打开示例网站](https://feishu.cn)\n"
            "- 代码块示例：\n"
            "\n"
            "```json\n"
            "{\"hello\": \"world\"}\n"
            "```"
        ),
    )

    return parser


def _emit(result: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(result, ensure_ascii=False, indent=2) + "\n")


def main(argv: list[str] | None = None) -> int:
    raw_argv = sys.argv[1:] if argv is None else argv
    cleaned_argv, webhook, timeout = _split_global_args(raw_argv)
    args = _parser().parse_args(cleaned_argv)
    if webhook is not None:
        args.webhook = webhook
    if timeout is not None:
        args.timeout = timeout

    try:
        client = _client_from_args(args)
        if args.command == "send-text":
            result = client.send_text(args.text)
        elif args.command == "send-post":
            result = client.send_post(args.title, args.lines)
        elif args.command == "send-markdown":
            result = client.send_markdown(args.title, _normalize_cli_markdown(args.markdown))
        elif args.command == "demo":
            result = client.send_markdown(args.title, args.markdown)
        else:
            raise LarkNoticeError(f"未知命令: {args.command}")
    except LarkNoticeError as exc:
        sys.stderr.write(f"ERROR: {exc}\n")
        return 2

    _emit(result)
    return 0

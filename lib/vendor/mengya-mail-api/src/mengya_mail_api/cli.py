from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any

from .config import ConfigError, MailConfig
from .email_client import MailClient, MailClientError
from .templates import TemplateError, list_templates, render_template


def _load_env_file(path: str | None) -> None:
    if path:
        os.environ["MENGYA_MAIL_ENV_FILE"] = path



def _split_global_args(argv: list[str]) -> tuple[list[str], str | None, str | None]:
    cleaned: list[str] = []
    env_file: str | None = None
    output_format: str | None = None
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--env-file" and i + 1 < len(argv):
            env_file = argv[i + 1]
            i += 2
            continue
        if arg == "--format" and i + 1 < len(argv):
            output_format = argv[i + 1]
            i += 2
            continue
        cleaned.append(arg)
        i += 1
    return cleaned, env_file, output_format


def _client() -> MailClient:
    return MailClient(MailConfig.from_env())


def _parse_var_pairs(values: list[str] | None) -> dict[str, str]:
    result: dict[str, str] = {}
    for item in values or []:
        if "=" not in item:
            raise ValueError(f"变量格式应为 key=value: {item}")
        key, value = item.split("=", 1)
        key = key.strip()
        if not key:
            raise ValueError(f"变量键不能为空: {item}")
        result[key] = value
    return result


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="mengya-mail-api")
    parser.add_argument("--env-file", help="Load env vars from a specific .env file")
    parser.add_argument(
        "--format",
        choices=("json", "md"),
        default="json",
        help="Output format",
    )

    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("test-connection", help="Check SMTP/IMAP connectivity")

    send = sub.add_parser("send-email", help="Send a mail")
    send.add_argument("--to", nargs="+", required=True, help="Primary recipients")
    send.add_argument("--subject", required=True, help="Subject")
    send.add_argument("--text-body", help="Plain text body")
    send.add_argument("--html-body", help="HTML body")
    send.add_argument("--cc", nargs="+", help="CC recipients")
    send.add_argument("--bcc", nargs="+", help="BCC recipients")
    send.add_argument("--from-name", help="Display name of the sender")

    listed = sub.add_parser("list-emails", help="List messages")
    listed.add_argument("--folder", help="IMAP folder")
    listed.add_argument("--criteria", nargs="+", help="IMAP search criteria")
    listed.add_argument("--subject", help="Filter by subject")
    listed.add_argument("--from-address", dest="from_address", help="Filter by sender")
    listed.add_argument("--to-address", dest="to_address", help="Filter by recipient")
    listed.add_argument("--limit", type=int, default=10, help="Max messages")
    listed.add_argument("--mark-seen", action="store_true", help="Mark as seen")

    read = sub.add_parser("read-email", help="Read one message by UID")
    read.add_argument("--uid", required=True, help="Message UID")
    read.add_argument("--folder", help="IMAP folder")
    read.add_argument("--mark-seen", action="store_true", help="Mark as seen")

    tmpl = sub.add_parser("send-template", help="Send a template mail")
    tmpl.add_argument("--template", required=True, choices=sorted(list(item["key"] for item in list_templates())), help="Template key")
    tmpl.add_argument("--to", nargs="+", required=True, help="Primary recipients")
    tmpl.add_argument("--var", action="append", help="Template variable key=value")
    tmpl.add_argument("--cc", nargs="+", help="CC recipients")
    tmpl.add_argument("--bcc", nargs="+", help="BCC recipients")
    tmpl.add_argument("--from-name", help="Display name of the sender")

    sub.add_parser("templates", help="List template keys")
    return parser


def _to_markdown(command: str, result: dict[str, Any]) -> str:
    if command == "test-connection":
        lines = [
            "# Mail Connection",
            "",
            f"- Address: {result.get('address', '')}",
            f"- SMTP: {result.get('smtp', '')}",
            f"- IMAP: {result.get('imap', '')}",
            f"- Ready: {result.get('ready', False)}",
        ]
        return "\n".join(lines) + "\n"

    if command == "templates":
        templates = result.get("templates") or []
        lines = ["# Templates", "", f"- Count: {result.get('count', 0)}"]
        for item in templates:
            lines.append(f"- {item.get('key', '')}: {item.get('title', '')}")
        return "\n".join(lines) + "\n"

    if command in {"send-email", "send-template"}:
        lines = [
            "# Mail Sent",
            "",
            f"- Status: {result.get('status', '')}",
            f"- From: {result.get('from', '')}",
            f"- To: {', '.join(result.get('to') or [])}",
            f"- Subject: {result.get('subject', '')}",
        ]
        if result.get("template"):
            lines.append(f"- Template: {result.get('template')}")
        if result.get("date"):
            lines.append(f"- Date: {result.get('date')}")
        if result.get("message_id"):
            lines.append(f"- Message-ID: {result.get('message_id')}")
        return "\n".join(lines) + "\n"

    if command == "list-emails":
        lines = [
            "# Mail List",
            "",
            f"- Folder: {result.get('folder', '')}",
            f"- Count: {result.get('count', 0)}",
        ]
        for item in result.get("messages") or []:
            lines.extend(
                [
                    "",
                    f"## {item.get('subject', '(no subject)')}",
                    "",
                    f"- UID: {item.get('uid', '')}",
                    f"- From: {item.get('from', '')}",
                    f"- To: {item.get('to', '')}",
                    f"- Date: {item.get('date', '') or ''}",
                    f"- Seen: {item.get('seen', False)}",
                    f"- Attachments: {item.get('has_attachments', False)}",
                    f"- Snippet: {item.get('snippet', '')}",
                ]
            )
        return "\n".join(lines) + "\n"

    if command == "read-email":
        lines = [
            "# Mail Detail",
            "",
            f"- UID: {result.get('uid', '')}",
            f"- Subject: {result.get('subject', '')}",
            f"- From: {result.get('from', '')}",
            f"- To: {result.get('to', '')}",
            f"- CC: {result.get('cc', '')}",
            f"- Date: {result.get('date', '') or ''}",
            f"- Message-ID: {result.get('message_id', '')}",
            f"- Seen: {result.get('seen', False)}",
            f"- Attachments: {result.get('has_attachments', False)}",
        ]
        attachments = result.get("attachments") or []
        if attachments:
            lines.extend(["", "## Attachments", ""] + [f"- {item}" for item in attachments])
        text_body = result.get("text_body") or ""
        html_body = result.get("html_body") or ""
        if text_body:
            lines.extend(["", "## Text Body", "", text_body])
        if html_body:
            lines.extend(["", "## HTML Body", "", html_body])
        return "\n".join(lines) + "\n"

    return json.dumps(result, ensure_ascii=False, indent=2) + "\n"


def _emit(command: str, result: dict[str, Any], fmt: str) -> None:
    if fmt == "md":
        sys.stdout.write(_to_markdown(command, result))
    else:
        sys.stdout.write(json.dumps(result, ensure_ascii=False, indent=2) + "\n")


def main(argv: list[str] | None = None) -> int:
    raw_argv = sys.argv[1:] if argv is None else argv
    cleaned_argv, env_file, output_format = _split_global_args(raw_argv)
    args = _build_parser().parse_args(cleaned_argv)
    _load_env_file(env_file or args.env_file)

    try:
        client = _client()
        if args.command == "test-connection":
            result = client.test_connection()
        elif args.command == "send-email":
            result = client.send_email(
                to=args.to,
                subject=args.subject,
                text_body=args.text_body,
                html_body=args.html_body,
                cc=args.cc,
                bcc=args.bcc,
                from_name=args.from_name,
            )
        elif args.command == "list-emails":
            result = client.list_emails(
                folder=args.folder,
                criteria=args.criteria,
                limit=args.limit,
                mark_seen=args.mark_seen,
                subject=args.subject,
                from_address=args.from_address,
                to_address=args.to_address,
            )
        elif args.command == "read-email":
            result = client.read_email(uid=args.uid, folder=args.folder, mark_seen=args.mark_seen)
        elif args.command == "send-template":
            variables = _parse_var_pairs(args.var)
            subject, text_body, html_body = render_template(args.template, variables)
            result = client.send_email(
                to=args.to,
                subject=subject,
                text_body=text_body,
                html_body=html_body,
                cc=args.cc,
                bcc=args.bcc,
                from_name=args.from_name,
            )
            result["template"] = args.template
        elif args.command == "templates":
            template_items = list_templates()
            result = {"count": len(template_items), "templates": template_items}
        else:
            sys.stderr.write(f"ERROR: unsupported command: {args.command}\n")
            return 2
    except (ConfigError, MailClientError, TemplateError, ValueError) as exc:
        sys.stderr.write(f"ERROR: {exc}\n")
        return 2

    final_format = output_format or args.format
    _emit(args.command, result, final_format)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

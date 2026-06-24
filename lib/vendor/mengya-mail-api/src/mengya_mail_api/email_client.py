from __future__ import annotations

import email
import imaplib
import json
import re
import shlex
import smtplib
from dataclasses import dataclass
from datetime import datetime
from email import policy
from email.header import decode_header, make_header
from email.message import EmailMessage, Message
from email.utils import formataddr, formatdate, make_msgid, parsedate_to_datetime
from html.parser import HTMLParser
from typing import Any

from .config import MailConfig


class MailClientError(RuntimeError):
    pass


@dataclass
class MailSummary:
    uid: str
    subject: str
    from_address: str
    to: str
    date: str | None
    seen: bool
    has_attachments: bool
    snippet: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "uid": self.uid,
            "subject": self.subject,
            "from": self.from_address,
            "to": self.to,
            "date": self.date,
            "seen": self.seen,
            "has_attachments": self.has_attachments,
            "snippet": self.snippet,
        }


class _HTMLTextExtractor(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self._chunks: list[str] = []

    def handle_data(self, data: str) -> None:
        if data:
            self._chunks.append(data)

    def text(self) -> str:
        return " ".join(chunk.strip() for chunk in self._chunks if chunk.strip())


def _decode_header_value(value: str | None) -> str:
    if not value:
        return ""
    try:
        return str(make_header(decode_header(value)))
    except Exception:
        return value


def _normalize_recipients(value: str | list[str] | tuple[str, ...] | None) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        items = [part.strip() for part in value.split(",")]
        return [item for item in items if item]
    return [item.strip() for item in value if item and item.strip()]


def _strip_html(html: str) -> str:
    parser = _HTMLTextExtractor()
    parser.feed(html)
    parser.close()
    return parser.text()


def _clean_text(text: str, limit: int | None = None) -> str:
    normalized = re.sub(r"\s+", " ", text).strip()
    if limit is not None and len(normalized) > limit:
        return normalized[: limit - 1] + "…"
    return normalized


def _pick_text_part(message: Message) -> tuple[str, str]:
    text_body = ""
    html_body = ""

    if message.is_multipart():
        for part in message.walk():
            disposition = part.get_content_disposition()
            if disposition == "attachment":
                continue
            content_type = part.get_content_type()
            payload = part.get_payload(decode=True)
            if payload is None:
                continue
            charset = part.get_content_charset() or "utf-8"
            try:
                body = payload.decode(charset, errors="replace")
            except LookupError:
                body = payload.decode("utf-8", errors="replace")
            if content_type == "text/plain" and not text_body:
                text_body = body
            elif content_type == "text/html" and not html_body:
                html_body = body
    else:
        payload = message.get_payload(decode=True)
        if payload is not None:
            charset = message.get_content_charset() or "utf-8"
            try:
                text_body = payload.decode(charset, errors="replace")
            except LookupError:
                text_body = payload.decode("utf-8", errors="replace")

    if not text_body and html_body:
        text_body = _strip_html(html_body)
    return text_body.strip(), html_body.strip()


def _has_attachments(message: Message) -> bool:
    for part in message.walk():
        if part.get_content_disposition() == "attachment":
            return True
    return False


def _attachment_names(message: Message) -> list[str]:
    names: list[str] = []
    for part in message.walk():
        if part.get_content_disposition() == "attachment":
            filename = part.get_filename()
            if filename:
                names.append(_decode_header_value(filename))
    return names


def _parse_date(value: str | None) -> str | None:
    if not value:
        return None
    try:
        dt = parsedate_to_datetime(value)
        if isinstance(dt, datetime):
            return dt.isoformat()
    except Exception:
        return value
    return value


def _contains(haystack: str, needle: str | None) -> bool:
    if not needle:
        return True
    return needle.casefold() in haystack.casefold()


class MailClient:
    def __init__(self, config: MailConfig) -> None:
        self.config = config

    def send_email(
        self,
        *,
        to: str | list[str],
        subject: str,
        text_body: str | None = None,
        html_body: str | None = None,
        cc: str | list[str] | None = None,
        bcc: str | list[str] | None = None,
        from_name: str | None = None,
    ) -> dict[str, Any]:
        to_list = _normalize_recipients(to)
        cc_list = _normalize_recipients(cc)
        bcc_list = _normalize_recipients(bcc)
        all_recipients = to_list + cc_list + bcc_list
        if not to_list:
            raise MailClientError("至少需要一个主收件人")
        if not all_recipients:
            raise MailClientError("至少需要一个收件人")
        if not text_body and not html_body:
            raise MailClientError("text_body 与 html_body 至少提供一个")

        message = EmailMessage()
        message["From"] = formataddr((from_name, self.config.address)) if from_name else self.config.address
        message["To"] = ", ".join(to_list)
        if cc_list:
            message["Cc"] = ", ".join(cc_list)
        message["Subject"] = subject
        message["Date"] = formatdate(localtime=True)
        message["Message-ID"] = make_msgid(domain=self.config.address.split("@", 1)[-1])

        if text_body:
            message.set_content(text_body)
        elif html_body:
            message.set_content(_strip_html(html_body) or "请查看 HTML 正文")

        if html_body:
            message.add_alternative(html_body, subtype="html")

        try:
            with smtplib.SMTP_SSL(
                self.config.smtp_host,
                self.config.smtp_port,
                timeout=self.config.timeout_seconds,
            ) as smtp:
                smtp.login(self.config.address, self.config.password)
                smtp.send_message(message, from_addr=self.config.address, to_addrs=all_recipients)
        except Exception as exc:
            raise MailClientError(f"SMTP 发送失败: {exc}") from exc

        return {
            "status": "sent",
            "from": message["From"],
            "to": to_list,
            "cc": cc_list,
            "bcc": bcc_list,
            "subject": subject,
            "date": _parse_date(message["Date"]),
            "message_id": message["Message-ID"],
        }

    def test_connection(self) -> dict[str, Any]:
        smtp_status = "ok"
        imap_status = "ok"

        try:
            with smtplib.SMTP_SSL(
                self.config.smtp_host,
                self.config.smtp_port,
                timeout=self.config.timeout_seconds,
            ) as smtp:
                smtp.login(self.config.address, self.config.password)
        except Exception as exc:
            smtp_status = f"failed: {exc}"

        try:
            with imaplib.IMAP4_SSL(
                self.config.imap_host,
                self.config.imap_port,
                timeout=self.config.timeout_seconds,
            ) as imap:
                imap.login(self.config.address, self.config.password)
        except Exception as exc:
            imap_status = f"failed: {exc}"

        return {
            "address": self.config.address,
            "smtp": smtp_status,
            "imap": imap_status,
            "ready": smtp_status == "ok" and imap_status == "ok",
        }

    def list_emails(
        self,
        *,
        folder: str | None = None,
        criteria: str | list[str] | None = None,
        limit: int = 10,
        mark_seen: bool = False,
        subject: str | None = None,
        from_address: str | None = None,
        to_address: str | None = None,
    ) -> dict[str, Any]:
        selected_folder = folder or self.config.default_folder
        if limit < 1:
            raise MailClientError("limit 必须大于 0")

        use_client_side_filters = any([subject, from_address, to_address])
        base_search_terms = self._build_base_search_terms(criteria=criteria, use_all=use_client_side_filters)

        try:
            with imaplib.IMAP4_SSL(
                self.config.imap_host,
                self.config.imap_port,
                timeout=self.config.timeout_seconds,
            ) as imap:
                imap.login(self.config.address, self.config.password)
                status, _ = imap.select(selected_folder, readonly=not mark_seen)
                if status != "OK":
                    raise MailClientError(f"无法选择文件夹 {selected_folder}")

                status, data = imap.uid("search", None, *base_search_terms)
                if status != "OK":
                    raise MailClientError(f"IMAP 搜索失败: {' '.join(base_search_terms)}")

                uids = [uid.decode() for uid in data[0].split()] if data and data[0] else []
                candidate_limit = max(limit * 10, 50) if use_client_side_filters else limit
                selected_uids = list(reversed(uids[-candidate_limit:]))

                messages: list[dict[str, Any]] = []
                for uid in selected_uids:
                    summary = self._fetch_summary(imap, uid, mark_seen=mark_seen)
                    if use_client_side_filters and not self._matches_summary_filters(
                        summary,
                        subject=subject,
                        from_address=from_address,
                        to_address=to_address,
                    ):
                        continue
                    messages.append(summary.to_dict())
                    if len(messages) >= limit:
                        break
        except MailClientError:
            raise
        except Exception as exc:
            raise MailClientError(f"IMAP 列取邮件失败: {exc}") from exc

        return {
            "folder": selected_folder,
            "criteria": base_search_terms,
            "count": len(messages),
            "messages": messages,
        }

    def read_email(
        self,
        *,
        uid: str,
        folder: str | None = None,
        mark_seen: bool = False,
    ) -> dict[str, Any]:
        selected_folder = folder or self.config.default_folder
        fetch_query = "(RFC822 FLAGS)" if mark_seen else "(BODY.PEEK[] FLAGS)"

        try:
            with imaplib.IMAP4_SSL(
                self.config.imap_host,
                self.config.imap_port,
                timeout=self.config.timeout_seconds,
            ) as imap:
                imap.login(self.config.address, self.config.password)
                status, _ = imap.select(selected_folder, readonly=not mark_seen)
                if status != "OK":
                    raise MailClientError(f"无法选择文件夹 {selected_folder}")
                status, data = imap.uid("fetch", uid, fetch_query)
                if status != "OK" or not data or data[0] is None:
                    raise MailClientError(f"找不到 UID={uid} 的邮件")
                metadata = data[0][0].decode(errors="replace") if isinstance(data[0], tuple) else str(data[0])
                raw_message = data[0][1] if isinstance(data[0], tuple) else b""
                message = email.message_from_bytes(raw_message, policy=policy.default)
        except MailClientError:
            raise
        except Exception as exc:
            raise MailClientError(f"读取邮件失败: {exc}") from exc

        text_body, html_body = _pick_text_part(message)
        return {
            "uid": uid,
            "subject": _decode_header_value(message.get("Subject")),
            "from": _decode_header_value(message.get("From")),
            "to": _decode_header_value(message.get("To")),
            "cc": _decode_header_value(message.get("Cc")),
            "date": _parse_date(message.get("Date")),
            "message_id": _decode_header_value(message.get("Message-ID")),
            "seen": "\\Seen" in metadata,
            "has_attachments": _has_attachments(message),
            "attachments": _attachment_names(message),
            "text_body": text_body[:20000],
            "html_body": html_body[:20000],
        }

    def _fetch_summary(self, imap: imaplib.IMAP4_SSL, uid: str, *, mark_seen: bool) -> MailSummary:
        fetch_query = "(RFC822 FLAGS)" if mark_seen else "(BODY.PEEK[] FLAGS)"
        status, data = imap.uid("fetch", uid, fetch_query)
        if status != "OK" or not data or data[0] is None:
            raise MailClientError(f"无法读取 UID={uid} 的邮件")

        metadata = data[0][0].decode(errors="replace") if isinstance(data[0], tuple) else str(data[0])
        raw_message = data[0][1] if isinstance(data[0], tuple) else b""
        message = email.message_from_bytes(raw_message, policy=policy.default)
        text_body, html_body = _pick_text_part(message)
        snippet_source = text_body or _strip_html(html_body)
        return MailSummary(
            uid=uid,
            subject=_decode_header_value(message.get("Subject")),
            from_address=_decode_header_value(message.get("From")),
            to=_decode_header_value(message.get("To")),
            date=_parse_date(message.get("Date")),
            seen="\\Seen" in metadata,
            has_attachments=_has_attachments(message),
            snippet=_clean_text(snippet_source, limit=160),
        )

    @staticmethod
    def _build_base_search_terms(*, criteria: str | list[str] | None, use_all: bool) -> list[str]:
        if criteria is None:
            return ["ALL"] if use_all else ["UNSEEN"]
        return MailClient._normalize_search_terms(criteria)

    @staticmethod
    def _matches_summary_filters(
        summary: MailSummary,
        *,
        subject: str | None,
        from_address: str | None,
        to_address: str | None,
    ) -> bool:
        return (
            _contains(summary.subject, subject)
            and _contains(summary.from_address, from_address)
            and _contains(summary.to, to_address)
        )

    @staticmethod
    def _normalize_search_terms(criteria: str | list[str] | None) -> list[str]:
        if criteria is None:
            return ["UNSEEN"]
        if isinstance(criteria, str):
            parsed = shlex.split(criteria)
            return parsed or ["UNSEEN"]
        if not criteria:
            return ["UNSEEN"]
        return [item for item in criteria if item]


def format_result(data: dict[str, Any]) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2)

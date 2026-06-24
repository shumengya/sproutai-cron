from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any
from urllib import error, request


class LarkNoticeError(RuntimeError):
    pass


@dataclass(slots=True)
class LarkWebhookClient:
    webhook: str
    timeout: float = 10.0

    def _post(self, payload: dict[str, Any]) -> dict[str, Any]:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        req = request.Request(
            self.webhook,
            data=data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with request.urlopen(req, timeout=self.timeout) as resp:
                body = resp.read().decode("utf-8", errors="replace")
        except error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace") if exc.fp else ""
            raise LarkNoticeError(f"HTTP {exc.code}: {body or exc.reason}") from exc
        except error.URLError as exc:
            raise LarkNoticeError(f"网络错误: {exc.reason}") from exc

        try:
            result = json.loads(body)
        except json.JSONDecodeError:
            raise LarkNoticeError(f"响应不是 JSON: {body}") from None

        if result.get("code") not in (0, None):
            raise LarkNoticeError(
                f"飞书返回错误: code={result.get('code')} msg={result.get('msg') or result.get('message')}"
            )
        return result

    def send_text(self, text: str) -> dict[str, Any]:
        return self._post({"msg_type": "text", "content": {"text": text}})

    def send_post(self, title: str, lines: list[str]) -> dict[str, Any]:
        content = [[{"tag": "text", "text": line}] for line in lines]
        return self._post(
            {
                "msg_type": "post",
                "content": {
                    "post": {
                        "zh-CN": {
                            "title": title,
                            "content": content,
                        }
                    }
                },
            }
        )

    @staticmethod
    def _normalize_markdown(markdown: str) -> str:
        return markdown.replace("\r\n", "\n").replace("\r", "\n")

    def send_markdown(self, title: str, markdown: str) -> dict[str, Any]:
        return self._post(
            {
                "msg_type": "interactive",
                "card": {
                    "schema": "2.0",
                    "config": {"wide_screen_mode": True, "enable_forward": True},
                    "header": {"title": {"tag": "plain_text", "content": title}},
                    "body": {
                        "elements": [
                            {
                                "tag": "markdown",
                                "content": self._normalize_markdown(markdown),
                            }
                        ]
                    },
                },
            }
        )

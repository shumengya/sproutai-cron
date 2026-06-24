from __future__ import annotations

import os
from pathlib import Path

DEFAULT_TEMPLATE_VARS = {
    "name": "朋友",
    "sender": "树萌芽",
}

TEMPLATES: dict[str, dict[str, str]] = {
    "birthday": {
        "title": "生日祝福",
        "subject": "生日快乐，{name}！",
        "text": (
            "亲爱的{name}：\n\n"
            "祝你生日快乐，愿新的一岁平安顺遂，心想事成！\n\n"
            "{sender}"
        ),
        "html_file": "birthday.html",
    },
    "new_year": {
        "title": "元旦祝福",
        "subject": "元旦快乐，{name}！",
        "text": (
            "亲爱的{name}：\n\n"
            "新年伊始，愿你元旦快乐，万事顺遂，心想事成！\n\n"
            "{sender}"
        ),
        "html_file": "new_year.html",
    },
}


class TemplateError(ValueError):
    pass


def template_dir() -> Path:
    env_dir = os.getenv("MENGYA_MAIL_TEMPLATE_DIR")
    if env_dir:
        return Path(env_dir).expanduser()
    return Path(__file__).resolve().parents[2] / "template"


def read_template_file(filename: str) -> str:
    path = template_dir() / filename
    if not path.is_file():
        raise TemplateError(f"Template file not found: {path}")
    return path.read_text(encoding="utf-8")


def render_template(template_key: str, variables: dict[str, str] | None = None) -> tuple[str, str, str | None]:
    template = TEMPLATES.get(template_key)
    if not template:
        raise TemplateError(f"Unknown template: {template_key}")

    merged = {**DEFAULT_TEMPLATE_VARS, **(variables or {})}
    try:
        subject = template["subject"].format(**merged)
        text_body = template["text"].format(**merged)
        html_body = None
        html_file = template.get("html_file")
        if html_file:
            html_body = read_template_file(html_file).format(**merged)
    except KeyError as exc:
        missing = exc.args[0] if exc.args else "unknown"
        raise TemplateError(f"Missing template variable: {missing}") from exc

    return subject, text_body, html_body


def list_templates() -> list[dict[str, str]]:
    return [
        {"key": key, "title": value.get("title", key)}
        for key, value in TEMPLATES.items()
    ]

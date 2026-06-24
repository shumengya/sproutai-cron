from __future__ import annotations

import os
from typing import Any

import uvicorn
from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from . import __version__
from .config import ConfigError, MailConfig
from .email_client import MailClient, MailClientError
from .templates import TemplateError, list_templates as get_template_list, render_template


API_TOKEN_ENV = "MENGYA_MAIL_API_TOKEN"
DEFAULT_API_TOKEN = "shumengya520"
API_HOST_ENV = "MENGYA_MAIL_API_HOST"
API_PORT_ENV = "MENGYA_MAIL_API_PORT"


class SendEmailRequest(BaseModel):
    to: str | list[str]
    subject: str
    text_body: str | None = None
    html_body: str | None = None
    cc: str | list[str] | None = None
    bcc: str | list[str] | None = None
    from_name: str | None = None


class ListEmailsRequest(BaseModel):
    folder: str | None = None
    criteria: str | list[str] | None = None
    subject: str | None = None
    from_address: str | None = None
    to_address: str | None = None
    limit: int | None = None
    mark_seen: bool = False


class ReadEmailRequest(BaseModel):
    uid: str
    folder: str | None = None
    mark_seen: bool = False


class SendTemplateRequest(BaseModel):
    template: str
    to: str | list[str]
    variables: dict[str, str] | None = None
    cc: str | list[str] | None = None
    bcc: str | list[str] | None = None
    from_name: str | None = None


def _get_api_token() -> str:
    return os.getenv(API_TOKEN_ENV, DEFAULT_API_TOKEN)


def _extract_token(request: Request) -> str | None:
    auth_header = request.headers.get("authorization", "")
    if auth_header.lower().startswith("bearer "):
        return auth_header[7:].strip() or None
    return request.headers.get("x-auth-token")


def require_token(request: Request) -> None:
    expected = _get_api_token()
    provided = _extract_token(request)
    if not provided or provided != expected:
        raise HTTPException(status_code=401, detail="Unauthorized")


def create_app(mail_client: MailClient | None = None) -> FastAPI:
    app = FastAPI(
        title="萌芽邮箱 API",
        version=__version__,
        docs_url=None,
        redoc_url=None,
    )
    client = mail_client or MailClient(MailConfig.from_env())

    @app.exception_handler(ConfigError)
    async def handle_config_error(_: Request, exc: ConfigError) -> JSONResponse:
        return JSONResponse({"error": str(exc)}, status_code=400)

    @app.exception_handler(MailClientError)
    async def handle_mail_error(_: Request, exc: MailClientError) -> JSONResponse:
        return JSONResponse({"error": str(exc)}, status_code=400)

    @app.get("/health")
    async def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.post("/api/send-email")
    async def send_email(
        payload: SendEmailRequest,
        _: None = Depends(require_token),
    ) -> dict[str, Any]:
        result = client.send_email(
            **payload.model_dump(exclude_none=True)
        )
        return result

    @app.post("/api/list-emails")
    async def list_emails(
        payload: ListEmailsRequest,
        _: None = Depends(require_token),
    ) -> dict[str, Any]:
        result = client.list_emails(
            **payload.model_dump(exclude_none=True)
        )
        return result

    @app.post("/api/read-email")
    async def read_email(
        payload: ReadEmailRequest,
        _: None = Depends(require_token),
    ) -> dict[str, Any]:
        result = client.read_email(
            **payload.model_dump(exclude_none=True)
        )
        return result

    @app.get("/api/test-connection")
    async def test_connection(_: None = Depends(require_token)) -> dict[str, Any]:
        return client.test_connection()

    @app.get("/api/templates")
    async def list_templates(_: None = Depends(require_token)) -> dict[str, Any]:
        templates = get_template_list()
        return {"count": len(templates), "templates": templates}

    @app.post("/api/send-template")
    async def send_template(
        payload: SendTemplateRequest,
        _: None = Depends(require_token),
    ) -> dict[str, Any]:
        try:
            subject, text_body, html_body = render_template(payload.template, payload.variables)
        except TemplateError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
        result = client.send_email(
            to=payload.to,
            subject=subject,
            text_body=text_body,
            html_body=html_body,
            cc=payload.cc,
            bcc=payload.bcc,
            from_name=payload.from_name,
        )
        result["template"] = payload.template
        return result

    return app


app = create_app()


def main() -> None:
    host = os.getenv(API_HOST_ENV, "0.0.0.0")
    port = int(os.getenv(API_PORT_ENV, "8080"))
    uvicorn.run("mengya_mail_api.http_server:app", host=host, port=port)


if __name__ == "__main__":
    main()

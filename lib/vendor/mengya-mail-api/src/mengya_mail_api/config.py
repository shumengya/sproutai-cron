from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


class ConfigError(ValueError):
    pass


def _strip_quotes(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        return value[1:-1]
    return value


def _candidate_env_files() -> list[Path]:
    candidates: list[Path] = []

    explicit = os.getenv("MENGYA_MAIL_ENV_FILE")
    if explicit:
        candidates.append(Path(explicit).expanduser())

    cwd_env = Path.cwd() / ".env"
    package_env = Path(__file__).resolve().parents[2] / ".env"

    candidates.extend([cwd_env, package_env])

    unique: list[Path] = []
    seen: set[Path] = set()
    for candidate in candidates:
        resolved = candidate.resolve(strict=False)
        if resolved not in seen:
            seen.add(resolved)
            unique.append(candidate)
    return unique


def _load_env_file(path: Path) -> None:
    if not path.is_file():
        return

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = _strip_quotes(value.strip())
        if key and key not in os.environ:
            os.environ[key] = value


def load_env_if_present() -> None:
    for candidate in _candidate_env_files():
        _load_env_file(candidate)


@dataclass(frozen=True)
class MailConfig:
    address: str
    password: str
    smtp_host: str = "smtp.qiye.aliyun.com"
    smtp_port: int = 465
    imap_host: str = "imap.qiye.aliyun.com"
    imap_port: int = 993
    default_folder: str = "INBOX"
    timeout_seconds: float = 30.0

    @classmethod
    def from_env(cls) -> "MailConfig":
        load_env_if_present()

        address = os.getenv("MENGYA_MAIL_ADDRESS") or os.getenv("ALIYUN_MAIL_ADDRESS")
        password = os.getenv("MENGYA_MAIL_PASSWORD") or os.getenv("ALIYUN_MAIL_PASSWORD")
        if not address:
            raise ConfigError("缺少环境变量 MENGYA_MAIL_ADDRESS")
        if not password:
            raise ConfigError("缺少环境变量 MENGYA_MAIL_PASSWORD")

        return cls(
            address=address,
            password=password,
            smtp_host=os.getenv("MENGYA_MAIL_SMTP_HOST", "smtp.qiye.aliyun.com"),
            smtp_port=int(os.getenv("MENGYA_MAIL_SMTP_PORT", "465")),
            imap_host=os.getenv("MENGYA_MAIL_IMAP_HOST", "imap.qiye.aliyun.com"),
            imap_port=int(os.getenv("MENGYA_MAIL_IMAP_PORT", "993")),
            default_folder=os.getenv("MENGYA_MAIL_DEFAULT_FOLDER", "INBOX"),
            timeout_seconds=float(os.getenv("MENGYA_MAIL_TIMEOUT_SECONDS", "30")),
        )

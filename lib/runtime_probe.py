"""检测本机 Python / Node.js / Bash / PowerShell 是否可用及版本。"""

from __future__ import annotations

import re
import shutil
import subprocess
import sys
from dataclasses import asdict, dataclass


@dataclass
class RuntimeProbe:
    id: str
    name: str
    available: bool
    version: str | None = None
    path: str | None = None

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def _first_line(text: str) -> str:
    line = (text or "").strip().splitlines()[0].strip() if text else ""
    return line


def _run(cmd: list[str], *, timeout: float = 8.0) -> tuple[int, str]:
    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
        )
        out = _first_line((proc.stdout or "") + "\n" + (proc.stderr or ""))
        return proc.returncode, out
    except (OSError, subprocess.TimeoutExpired):
        return 1, ""


def _probe_python() -> RuntimeProbe:
    exe = sys.executable
    code, out = _run([exe, "--version"])
    version: str | None = None
    if code == 0 and out:
        version = out.removeprefix("Python ").strip() if out.lower().startswith("python") else out
    elif sys.version:
        version = sys.version.split()[0]
    return RuntimeProbe(
        id="python",
        name="Python",
        available=True,
        version=version,
        path=exe,
    )


def _probe_nodejs() -> RuntimeProbe:
    path = shutil.which("node")
    if not path:
        return RuntimeProbe(id="nodejs", name="JavaScript", available=False)
    code, out = _run([path, "--version"])
    if code != 0 or not out:
        return RuntimeProbe(id="nodejs", name="JavaScript", available=False, path=path)
    version = out.lstrip("vV").strip() or out
    return RuntimeProbe(id="nodejs", name="JavaScript", available=True, version=version, path=path)


def _parse_bash_version(text: str) -> str | None:
    match = re.search(r"version\s+([^\s(]+)", text, re.I)
    return match.group(1) if match else (text or None)


def _probe_bash() -> RuntimeProbe:
    path = shutil.which("bash")
    if not path:
        return RuntimeProbe(id="bash", name="Bash", available=False)
    code, out = _run([path, "--version"])
    if code != 0 or not out:
        return RuntimeProbe(id="bash", name="Bash", available=False, path=path)
    version = _parse_bash_version(out)
    return RuntimeProbe(id="bash", name="Bash", available=True, version=version, path=path)


def _probe_powershell() -> RuntimeProbe:
    for cmd_name in ("pwsh", "powershell"):
        path = shutil.which(cmd_name)
        if not path:
            continue
        code, out = _run(
            [path, "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"]
        )
        if code == 0 and out:
            return RuntimeProbe(
                id="powershell",
                name="PowerShell",
                available=True,
                version=out.strip(),
                path=path,
            )
    return RuntimeProbe(id="powershell", name="PowerShell", available=False)


def probe_all_runtimes() -> list[dict[str, object]]:
    return [
        _probe_python().to_dict(),
        _probe_nodejs().to_dict(),
        _probe_bash().to_dict(),
        _probe_powershell().to_dict(),
    ]

#!/usr/bin/env python3
"""SproutClaw Cron 环境检查（跨平台，仅标准库 + 可选 fastmcp）。"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def main() -> int:
    root = Path(__file__).resolve().parents[4]
    lib = root / "lib"
    cronctl = root / "cronctl.py"
    mcp_server = root / "mcp-server" / "server.py"

    checks: list[dict[str, object]] = []

    def add(name: str, ok: bool, detail: str = "") -> None:
        checks.append({"name": name, "ok": ok, "detail": detail})

    add("cron_root", root.is_dir(), str(root))
    add("cronctl.py", cronctl.is_file(), str(cronctl))
    add("lib/", lib.is_dir(), str(lib))
    add("mcp-server/server.py", mcp_server.is_file(), str(mcp_server))

    try:
        import fastmcp  # noqa: F401

        add("fastmcp", True, "已安装")
    except ImportError:
        add("fastmcp", False, "pip install -r mcp-server/requirements.txt")

    if cronctl.is_file():
        proc = subprocess.run(
            [sys.executable, str(cronctl), "status"],
            cwd=str(root),
            capture_output=True,
            text=True,
            timeout=30,
        )
        add(
            "cronctl status",
            proc.returncode == 0,
            (proc.stdout or proc.stderr).strip()[:500],
        )

    ok_all = all(c["ok"] for c in checks if c["name"] != "fastmcp")
    print(json.dumps({"ok": ok_all, "checks": checks}, ensure_ascii=False, indent=2))
    return 0 if ok_all else 1


if __name__ == "__main__":
    raise SystemExit(main())

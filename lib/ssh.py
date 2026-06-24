"""
远端 SSH：用 bash -lc 执行命令，使 PATH 与登录 shell 一致（非交互 ssh 常缺 /usr/local/bin 等，导致找不到 docker）。
"""

from __future__ import annotations

import subprocess


def ssh_bash_lc(host: str, remote_cmd: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [
            "ssh",
            "-o",
            "BatchMode=yes",
            "-o",
            "StrictHostKeyChecking=accept-new",
            host,
            "bash",
            "-lc",
            remote_cmd,
        ],
        capture_output=True,
        text=True,
    )

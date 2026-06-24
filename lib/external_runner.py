"""非 Python 任务：由 Python 编排 disable / 锁 / 日志，subprocess 执行业务脚本。"""

from __future__ import annotations

import os
import shutil
import subprocess

from runner import (
    TaskContext,
    acquire_cron_lock,
    append_raw_to_log,
    task_is_disabled,
    task_logging,
)
from task_manifest import TaskManifest


def _resolve_interpreter(runtime: str) -> list[str] | None:
    if runtime == "javascript":
        node = shutil.which("node")
        return [node] if node else None
    if runtime == "bash":
        bash = shutil.which("bash")
        return [bash] if bash else None
    if runtime == "powershell":
        pwsh = shutil.which("pwsh")
        if pwsh:
            return [pwsh, "-File"]
        ps = shutil.which("powershell")
        return [ps, "-File"] if ps else None
    return None


def build_command(manifest: TaskManifest, ctx: TaskContext) -> list[str]:
    interpreter = _resolve_interpreter(manifest.runtime)
    if interpreter is None:
        raise FileNotFoundError(f"未找到 {manifest.runtime} 解释器，请确认已安装并在 PATH 中")
    entry = ctx.task_dir / manifest.entry
    if manifest.runtime == "powershell":
        return [*interpreter, str(entry)]
    if manifest.runtime == "bash":
        return [*interpreter, str(entry)]
    return [*interpreter, str(entry)]


def _task_env(ctx: TaskContext) -> dict[str, str]:
    env = os.environ.copy()
    env["CRON_TASK_ID"] = ctx.task_id
    env["CRON_TASK_DIR"] = str(ctx.task_dir)
    env["CRON_ROOT"] = str(ctx.cron_root)
    return env


def run_external_task(ctx: TaskContext, manifest: TaskManifest) -> int:
    if task_is_disabled(ctx):
        return 0

    with task_logging(ctx) as log:
        with acquire_cron_lock(ctx.lock_file, log=log) as locked:
            if not locked:
                return 0

            log("start")
            log(f"runtime={manifest.runtime} entry={manifest.entry}")

            try:
                cmd = build_command(manifest, ctx)
            except FileNotFoundError as exc:
                log(str(exc))
                log("end")
                return 1

            proc = subprocess.run(
                cmd,
                cwd=str(ctx.task_dir),
                env=_task_env(ctx),
                capture_output=True,
                text=True,
                encoding="utf-8",
                errors="replace",
            )
            output = ((proc.stdout or "") + (proc.stderr or "")).strip()
            if output:
                append_raw_to_log(ctx.log_file, output)

            if proc.returncode != 0:
                log(f"exit={proc.returncode}")

            log("end")
            return proc.returncode

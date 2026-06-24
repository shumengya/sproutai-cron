"""跨平台辅助：文件锁、Windows 任务计划程序同步。"""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path


def is_windows() -> bool:
    return sys.platform == "win32"


def lock_ex_nb(fd: int) -> None:
    """非阻塞独占锁；无法获取时抛出 BlockingIOError。"""
    if is_windows():
        import msvcrt

        try:
            msvcrt.locking(fd, msvcrt.LK_NBLCK, 1)
        except OSError as exc:
            raise BlockingIOError from exc
        return

    import fcntl

    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)


def unlock(fd: int) -> None:
    if is_windows():
        import msvcrt

        try:
            msvcrt.locking(fd, msvcrt.LK_UNLCK, 1)
        except OSError:
            pass
        return

    import fcntl

    try:
        fcntl.flock(fd, fcntl.LOCK_UN)
    except OSError:
        pass


def windows_task_name(task_id: str) -> str:
    return f"sproutclaw-{task_id}"


def _cron_to_daily_time(expression: str) -> str | None:
    """将 `分 时 * * *` 转为 schtasks 的 HH:MM；不支持则返回 None。"""
    parts = expression.split()
    if len(parts) != 5:
        return None
    minute, hour, dom, month, dow = parts
    if dom != "*" or month != "*" or dow != "*":
        return None
    if not minute.isdigit() or not hour.isdigit():
        return None
    return f"{int(hour):02d}:{int(minute):02d}"


def sync_windows_task(
    task_id: str,
    cron_root: Path,
    schedule_expression: str | None,
    *,
    enabled: bool,
) -> str | None:
    """在 Windows 任务计划程序中注册/更新定时任务。"""
    if schedule_expression is None:
        return "未找到 cron 表达式，跳过任务计划注册"

    daily_time = _cron_to_daily_time(schedule_expression)
    if daily_time is None:
        return (
            f"暂不支持 cron 表达式「{schedule_expression}」自动转换，"
            "请手动创建任务计划或使用 `分 时 * * *` 形式"
        )

    python_exe = Path(sys.executable).resolve()
    cronctl = (cron_root / "cronctl.py").resolve()
    task_name = windows_task_name(task_id)
    run_cmd = f'"{python_exe}" "{cronctl}" run {task_id}'

    query = subprocess.run(
        ["schtasks", "/Query", "/TN", task_name],
        capture_output=True,
        text=True,
    )
    exists = query.returncode == 0

    if exists:
        subprocess.run(
            ["schtasks", "/Delete", "/TN", task_name, "/F"],
            capture_output=True,
            text=True,
            check=False,
        )

    create = subprocess.run(
        [
            "schtasks",
            "/Create",
            "/TN",
            task_name,
            "/TR",
            run_cmd,
            "/SC",
            "DAILY",
            "/ST",
            daily_time,
            "/F",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if create.returncode != 0:
        err = (create.stderr or create.stdout or "").strip()
        return f"任务计划注册失败: {err or '未知错误'}"

    state = "开启" if enabled else "关闭（计划仍触发，任务自动跳过）"
    action = "已更新" if exists else "已创建"
    return f"{action} Windows 任务「{task_name}」，每天 {daily_time}（{state}）"

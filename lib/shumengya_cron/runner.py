"""
任务运行骨架：路径、日志轮转、互斥锁、stdout 重定向。

按大小轮转日志、flock 互斥、
锁占用时退出码 0（避免 cron 误报）。
"""

from __future__ import annotations

import fcntl
import os
import sys
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Callable, Iterator, TextIO


class LogMode(str, Enum):
    """日志模式：整段重定向到文件（多数任务）或直接写文件（如 AI CLI 更新）。"""

    REDIRECT_STD = "redirect_std"  # 等价于 bash: exec >>LOG 2>&1
    DIRECT_FILE = "direct_file"  # 等价于 cron_log_file


def _cron_root() -> Path:
    # .../lib/shumengya_cron/runner.py -> cron 根目录
    return Path(__file__).resolve().parents[2]


DISABLED_DIR_NAME = ".disabled"


def disabled_tasks_root(cron_root: Path | None = None) -> Path:
    root = cron_root or _cron_root()
    return root / DISABLED_DIR_NAME


def active_task_dir(cron_root: Path, task_id: str) -> Path:
    return cron_root / task_id


def disabled_task_dir(cron_root: Path, task_id: str) -> Path:
    return disabled_tasks_root(cron_root) / task_id


def _legacy_disabled_task_dir(cron_root: Path, task_id: str) -> Path:
    return cron_root / f"{task_id}.disabled"


# 固定缩写，遇到这些词时全大写而非首字母大写
_ACRONYMS = frozenset({"ai", "cli", "ssh", "api", "db", "url"})


def task_output_prefix(task_id: str) -> str:
    """将 task_id 格式化为统一输出前缀：[smallmengya][AI-CLI-Update]。"""
    family, suffix = (task_id.split("-", 1) + [task_id])[:2]
    display = "-".join(
        p.upper() if p.lower() in _ACRONYMS else p.capitalize()
        for p in suffix.split("-")
    )
    return f"[{family}][{display}]"


@dataclass
class TaskContext:
    """单次任务运行的上下文（每个任务目录对应一个 task_id）。"""

    task_id: str
    cron_root: Path
    task_dir: Path
    log_dir: Path
    log_file: Path
    lock_file: Path
    log_mode: LogMode

    @classmethod
    def from_task_id(
        cls,
        task_id: str,
        *,
        log_mode: LogMode = LogMode.REDIRECT_STD,
        cron_root: Path | None = None,
    ) -> TaskContext:
        """根据 task_id 创建上下文，自动识别 .disabled/<task-id>/ 目录。"""
        root = cron_root or _cron_root()
        disabled_dir = disabled_task_dir(root, task_id).resolve()
        active_dir = active_task_dir(root, task_id).resolve()
        legacy_disabled_dir = _legacy_disabled_task_dir(root, task_id).resolve()

        if disabled_dir.is_dir():
            task_dir = disabled_dir
        elif legacy_disabled_dir.is_dir():
            task_dir = legacy_disabled_dir
        elif active_dir.is_dir():
            task_dir = active_dir
        else:
            task_dir = active_dir
        log_dir = task_dir / "logs"
        log_file = log_dir / f"{task_id}.log"
        lock_file = task_dir / f"{task_id}.lock"
        return cls(
            task_id=task_id,
            cron_root=root,
            task_dir=task_dir,
            log_dir=log_dir,
            log_file=log_file,
            lock_file=lock_file,
            log_mode=log_mode,
        )


def task_is_enabled(ctx: TaskContext) -> bool:
    """任务位于 cron 根目录下为开启，位于 .disabled/ 或 *.disabled 下为关闭。"""
    if ctx.task_dir.name.endswith(".disabled"):
        return False
    disabled_root = disabled_tasks_root(ctx.cron_root).resolve()
    try:
        return not ctx.task_dir.resolve().is_relative_to(disabled_root)
    except AttributeError:
        return ctx.task_dir.resolve().parent != disabled_root


def task_is_disabled(ctx: TaskContext) -> bool:
    return not task_is_enabled(ctx)


def set_task_enabled(ctx: TaskContext, enabled: bool) -> None:
    """通过移动目录切换任务状态：启用时在根目录，禁用时在 .disabled/ 下。"""
    root = ctx.cron_root
    active_dir = active_task_dir(root, ctx.task_id)
    disabled_dir = disabled_task_dir(root, ctx.task_id)
    legacy_disabled_dir = _legacy_disabled_task_dir(root, ctx.task_id)

    if legacy_disabled_dir.is_dir() and not disabled_dir.is_dir():
        disabled_dir.parent.mkdir(parents=True, exist_ok=True)
        legacy_disabled_dir.rename(disabled_dir)

    if enabled:
        if disabled_dir.is_dir() and not active_dir.is_dir():
            active_dir.parent.mkdir(parents=True, exist_ok=True)
            disabled_dir.rename(active_dir)
            ctx.task_dir = active_dir
        elif active_dir.is_dir():
            ctx.task_dir = active_dir
        return

    disabled_dir.parent.mkdir(parents=True, exist_ok=True)
    if active_dir.is_dir() and not disabled_dir.is_dir():
        active_dir.rename(disabled_dir)
        ctx.task_dir = disabled_dir
    elif disabled_dir.is_dir():
        ctx.task_dir = disabled_dir


def task_enabled_text(ctx: TaskContext) -> str:
    return "开启" if task_is_enabled(ctx) else "关闭"


def cron_log_rotate(log_file: Path, max_bytes: int | None = None) -> None:
    """单日志超过阈值则轮转（默认 10MB），与 legacy cron_log_rotate 一致。"""
    mb = max_bytes if max_bytes is not None else int(
        os.environ.get("CRON_LOG_MAX_BYTES", str(10 * 1024 * 1024))
    )
    try:
        if not log_file.is_file():
            return
        if log_file.stat().st_size <= mb:
            return
        stamp = datetime.now().strftime("%Y-%m-%d-%H%M%S")
        log_file.rename(log_file.with_name(f"{log_file.name}.{stamp}"))
    except OSError:
        pass


@contextmanager
def acquire_cron_lock(
    lock_file: Path,
    *,
    busy_message: str | None = None,
    log: Callable[[str], None],
) -> Iterator[bool]:
    """
    获取互斥锁；若已被占用则记录日志并 yield False（调用方应 sys.exit(0)）。
    使用 flock（与 bash 版一致）。
    """
    msg = busy_message or os.environ.get(
        "CRON_LOCK_BUSY_MSG", "已有同名任务在运行，跳过本次。"
    )
    lock_file.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(lock_file), os.O_RDWR | os.O_CREAT, 0o644)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            log(msg)
            yield False
            return
        yield True
    finally:
        try:
            fcntl.flock(fd, fcntl.LOCK_UN)
        except OSError:
            pass
        os.close(fd)


def _tee_streams(log_fp: TextIO) -> tuple[TextIO, TextIO]:
    """将 stdout/stderr 同时写到日志与原始 fd（便于调试时仍可在终端看到）。"""

    class Tee(TextIO):
        def __init__(self, *streams: TextIO) -> None:
            self._streams = streams

        def write(self, s: str) -> int:
            n = 0
            for st in self._streams:
                n = st.write(s)
                st.flush()
            return n

        def flush(self) -> None:
            for st in self._streams:
                st.flush()

    # 保留原 stdout/stderr 供 Tee
    orig_out = sys.__stdout__
    orig_err = sys.__stderr__
    out = Tee(log_fp, orig_out)
    err = Tee(log_fp, orig_err)
    return out, err


@contextmanager
def task_logging(ctx: TaskContext) -> Iterator[Callable[[str], None]]:
    """
    按 log_mode 配置日志。

    - REDIRECT_STD：轮转后重定向 stdout/stderr 到日志文件（并 tee 到原终端）。
    - DIRECT_FILE：不重定向，返回 log_line() 仅写文件。
    """
    ctx.log_dir.mkdir(parents=True, exist_ok=True)
    cron_log_rotate(ctx.log_file)
    prefix = task_output_prefix(ctx.task_id)

    if ctx.log_mode == LogMode.DIRECT_FILE:

        def log_line(message: str) -> None:
            ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            with ctx.log_file.open("a", encoding="utf-8") as fp:
                fp.write(f"[{ts}] {prefix} {message}\n")

        yield log_line
        return

    log_fp = ctx.log_file.open("a", encoding="utf-8")
    try:
        out, err = _tee_streams(log_fp)
        sys.stdout, sys.stderr = out, err

        def log_line(message: str) -> None:
            ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            print(f"[{ts}] {prefix} {message}", flush=True)

        yield log_line
    finally:
        if ctx.log_mode == LogMode.REDIRECT_STD:
            sys.stdout = sys.__stdout__
            sys.stderr = sys.__stderr__
            log_fp.close()


def append_raw_to_log(log_file: Path, text: str) -> None:
    """将多行原文追加到日志（命令输出等）。"""
    if not text:
        return
    with log_file.open("a", encoding="utf-8") as fp:
        fp.write(text)
        if not text.endswith("\n"):
            fp.write("\n")

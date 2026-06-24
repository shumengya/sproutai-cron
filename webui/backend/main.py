"""SproutClaw Cron 定时任务管理面板 API。"""

from __future__ import annotations

import subprocess
import sys
import threading
from pathlib import Path

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

CRON_ROOT = Path(__file__).resolve().parents[2]
FRONTEND_DIR = Path(__file__).resolve().parents[1] / "frontend"

sys.path.insert(0, str(CRON_ROOT / "lib"))

from shumengya_cron.manager import (  # noqa: E402
    build_task_info,
    list_tasks,
    read_log_tail,
    set_task_state,
    sync_cron_d,
    toggle_task,
)
from shumengya_cron.manager import get_context  # noqa: E402
from shumengya_cron.runner import task_is_enabled  # noqa: E402

app = FastAPI(title="SproutClaw Cron", version="1.0.0")

_run_jobs: dict[str, subprocess.Popen[bytes]] = {}
_run_lock = threading.Lock()


class MessageResponse(BaseModel):
    ok: bool = True
    message: str = ""


class RunResponse(BaseModel):
    ok: bool = True
    message: str
    pid: int | None = None


def _task_not_found(exc: FileNotFoundError) -> HTTPException:
    return HTTPException(status_code=404, detail=str(exc))


def _cleanup_run_job(task_id: str, proc: subprocess.Popen[bytes]) -> None:
    proc.wait()
    with _run_lock:
        current = _run_jobs.get(task_id)
        if current is proc:
            _run_jobs.pop(task_id, None)


@app.get("/api/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/api/tasks")
def api_list_tasks() -> list[dict[str, object]]:
    return [task.to_dict() for task in list_tasks(CRON_ROOT)]


@app.get("/api/tasks/{task_id}")
def api_get_task(task_id: str) -> dict[str, object]:
    try:
        return build_task_info(task_id, CRON_ROOT).to_dict()
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.post("/api/tasks/{task_id}/enable")
def api_enable_task(task_id: str) -> dict[str, object]:
    try:
        info, _message = set_task_state(task_id, True, CRON_ROOT)
        return info.to_dict()
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.post("/api/tasks/{task_id}/disable")
def api_disable_task(task_id: str) -> dict[str, object]:
    try:
        info, _message = set_task_state(task_id, False, CRON_ROOT)
        return info.to_dict()
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.post("/api/tasks/{task_id}/toggle")
def api_toggle_task(task_id: str) -> dict[str, object]:
    try:
        info, _message = toggle_task(task_id, CRON_ROOT)
        return info.to_dict()
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.post("/api/tasks/{task_id}/sync-cron", response_model=MessageResponse)
def api_sync_cron(task_id: str) -> MessageResponse:
    try:
        ctx = get_context(task_id, CRON_ROOT)
        message = sync_cron_d(ctx, task_is_enabled(ctx)) or "已同步"
        return MessageResponse(message=message)
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.post("/api/tasks/{task_id}/run", response_model=RunResponse)
def api_run_task(task_id: str) -> RunResponse:
    try:
        get_context(task_id, CRON_ROOT)
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc

    with _run_lock:
        existing = _run_jobs.get(task_id)
        if existing is not None and existing.poll() is None:
            return RunResponse(
                ok=False,
                message="任务已在后台运行",
                pid=existing.pid,
            )

        proc = subprocess.Popen(
            [sys.executable, str(CRON_ROOT / "cronctl.py"), "run", task_id],
            cwd=str(CRON_ROOT),
        )
        _run_jobs[task_id] = proc
        threading.Thread(
            target=_cleanup_run_job,
            args=(task_id, proc),
            daemon=True,
        ).start()
        return RunResponse(message="已在后台启动", pid=proc.pid)


@app.get("/api/tasks/{task_id}/log")
def api_task_log(
    task_id: str,
    lines: int = Query(default=200, ge=1, le=2000),
) -> dict[str, str]:
    try:
        content = read_log_tail(task_id, lines, CRON_ROOT)
        return {"task_id": task_id, "content": content}
    except FileNotFoundError as exc:
        raise _task_not_found(exc) from exc


@app.get("/")
def index() -> FileResponse:
    return FileResponse(FRONTEND_DIR / "index.html")


app.mount("/static", StaticFiles(directory=str(FRONTEND_DIR)), name="static")

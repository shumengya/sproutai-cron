"""
Gitea 仓库同步：配置见本目录 repos.json。

同步逻辑：先 fetch 检查远端是否有新提交，有则 fast-forward merge，无则跳过，
不会无谓拉取；仓库依次串行处理，每条间隔 SYNC_DELAY_SECONDS 秒。

env 变量：
  BASE_DIR            （已废弃，local 字段现在必填）
  SYNC_DELAY_SECONDS  每个仓库处理间隔秒数（默认 2）
  GIT_SSH_COMMAND     git SSH 参数
"""

from __future__ import annotations

import sys as _sys, pathlib as _pl
_sys.path.insert(0, str(_pl.Path(__file__).resolve().parents[1] / "lib"))
del _sys, _pl

import json
import os
import subprocess
import time
from datetime import datetime
from pathlib import Path

from shumengya_cron.notify import TaskResult, send_task_summary
from shumengya_cron.runner import (
    TaskContext,
    acquire_cron_lock,
    append_raw_to_log,
    task_is_disabled,
    task_logging,
)


def _load_repos(path: Path) -> list[tuple[str, Path]]:
    """加载 repos.json，local 字段必填，缺失的条目直接跳过。"""
    if not path.is_file():
        return []
    data = json.loads(path.read_text(encoding="utf-8"))
    result: list[tuple[str, Path]] = []
    for item in data:
        owner_repo = item.get("repo", "").strip()
        local_s = item.get("local", "").strip()
        if not owner_repo or not local_s:
            continue
        if "/" not in owner_repo:
            owner_repo = f"shumengya/{owner_repo}"
        result.append((owner_repo, Path(local_s)))
    return result


def _git(args: list[str], *, cwd: Path | None = None) -> tuple[int, str]:
    r = subprocess.run(
        ["git", *args],
        cwd=str(cwd) if cwd else None,
        capture_output=True,
        text=True,
        env={**os.environ, "GIT_TERMINAL_PROMPT": "0"},
    )
    return r.returncode, ((r.stdout or "") + (r.stderr or "")).strip()


def run(ctx: TaskContext) -> int:
    if task_is_disabled(ctx):
        return 0

    os.environ.setdefault("HOME", "/root")
    os.environ.setdefault(
        "GIT_SSH_COMMAND",
        "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",
    )

    repos_file = ctx.task_dir / "repos.json"
    sync_delay = int(os.environ.get("SYNC_DELAY_SECONDS", "2"))
    start_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    result = TaskResult()

    def sync_one(owner_repo: str, dir_path: Path, log) -> None:
        parts = owner_repo.split("/")
        owner, repo = parts[0], parts[-1]
        remote_url = f"ssh://git@git.shumengya.top:8022/{owner}/{repo}.git"

        # ── 仓库不存在：直接 clone ───────────────────────────
        if not (dir_path / ".git").is_dir():
            if dir_path.exists():
                log(f"[{owner_repo}] 路径已存在但不是 git 仓库，跳过：{dir_path}")
                result.add_skip(f"{owner_repo}:not_git")
                return
            dir_path.parent.mkdir(parents=True, exist_ok=True)
            log(f"[{owner_repo}] clone -> {dir_path}")
            rc, out = _git(["clone", remote_url, str(dir_path)])
            if out:
                append_raw_to_log(ctx.log_file, out)
            if rc == 0:
                log(f"[{owner_repo}] clone OK")
                result.add_ok(f"{owner_repo}:clone")
            else:
                log(f"[{owner_repo}] clone 失败，请检查 SSH/权限/网络。")
                result.add_fail(f"{owner_repo}:clone_failed")
            return

        # ── 仓库已存在：fetch → 检查差异 → merge ─────────────

        # 确认当前分支
        rc, branch = _git(["symbolic-ref", "--quiet", "--short", "HEAD"], cwd=dir_path)
        branch = branch.splitlines()[0].strip() if branch else ""
        if rc != 0 or not branch:
            log(f"[{owner_repo}] detached HEAD，跳过。")
            result.add_skip(f"{owner_repo}:detached")
            return

        # 找远端名并更新 URL
        rc, remote_name = _git(["config", "--get", f"branch.{branch}.remote"], cwd=dir_path)
        remote_name = remote_name.splitlines()[0].strip() if remote_name else ""
        if not remote_name:
            rc2, rn = _git(["remote"], cwd=dir_path)
            remote_name = rn.splitlines()[0].strip() if rc2 == 0 and rn else ""
        if not remote_name:
            log(f"[{owner_repo}] 未找到远端名，跳过。")
            result.add_fail(f"{owner_repo}:no_remote")
            return

        _git(["remote", "set-url", remote_name, remote_url], cwd=dir_path)

        # 工作区必须干净
        rc, dirty = _git(["status", "--porcelain"], cwd=dir_path)
        if dirty:
            log(f"[{owner_repo}] 工作区有改动，跳过（避免覆盖本地改动）。")
            result.add_skip(f"{owner_repo}:dirty")
            return

        # fetch 远端最新
        rc, fetch_out = _git(["fetch", "--prune", remote_name, branch], cwd=dir_path)
        if fetch_out:
            append_raw_to_log(ctx.log_file, fetch_out)
        if rc != 0:
            log(f"[{owner_repo}] fetch 失败，跳过。")
            result.add_fail(f"{owner_repo}:fetch_failed")
            return

        # 检查是否有新提交
        rc, count = _git(["rev-list", "HEAD..FETCH_HEAD", "--count"], cwd=dir_path)
        if rc == 0 and count.strip() == "0":
            log(f"[{owner_repo}] 已是最新，跳过。")
            result.add_skip(f"{owner_repo}:already_latest")
            return

        # 有更新，fast-forward merge
        log(f"[{owner_repo}] 发现 {count.strip()} 个新提交，合并中…")
        rc, merge_out = _git(["merge", "--ff-only", "FETCH_HEAD"], cwd=dir_path)
        if merge_out:
            append_raw_to_log(ctx.log_file, merge_out)
        if rc == 0:
            log(f"[{owner_repo}] OK")
            result.add_ok(f"{owner_repo}:updated")
        else:
            log(f"[{owner_repo}] merge 失败，请手动检查：git -C {dir_path} merge --ff-only FETCH_HEAD")
            result.add_fail(f"{owner_repo}:merge_failed")

    with task_logging(ctx) as log:
        with acquire_cron_lock(
            ctx.lock_file,
            busy_message=os.environ.get("CRON_LOCK_BUSY_MSG", "已有同步任务在运行，跳过本次。"),
            log=log,
        ) as locked:
            if not locked:
                return 0

            log("start")
            log(f"repos_file={repos_file}")
            log(f"sync_delay_seconds={sync_delay}")

            if not repos_file.is_file():
                log(f"仓库列表文件不存在：{repos_file}")
                result.add_fail("repos_file:missing")
                send_task_summary(ctx, result, start_time, log=log)
                return 1

            repo_items = _load_repos(repos_file)
            log(f"共 {len(repo_items)} 个仓库，依次处理")

            for idx, (owner_repo, dir_path) in enumerate(repo_items):
                sync_one(owner_repo, dir_path, log)
                if sync_delay > 0 and idx < len(repo_items) - 1:
                    time.sleep(sync_delay)

            log("end")
            send_task_summary(ctx, result, start_time, log=log)

    return 0 if result.success else 1


def main() -> int:
    import pathlib
    task_id = pathlib.Path(__file__).parent.name
    ctx = TaskContext.from_task_id(task_id)
    return run(ctx)


if __name__ == "__main__":
    raise SystemExit(main())

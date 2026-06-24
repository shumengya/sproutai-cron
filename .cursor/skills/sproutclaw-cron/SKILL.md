---
name: sproutclaw-cron
description: >-
  Manage SproutClaw Cron scheduled tasks: list/enable/disable/run tasks, read logs,
  update schedules, create tasks from templates. Use when the user mentions cronctl,
  sproutclaw-cron, scheduled tasks, cron jobs, task enable/disable, or creating
  new cron tasks. Prefer MCP tools (cron_*) when available; fall back to cronctl.py CLI.
---

# SproutClaw Cron 定时任务技能

## 何时启用

用户提到：`cronctl`、定时任务、开关任务、试跑任务、新建 cron 任务、`sproutclaw-cron`、WebUI 管理面板，或需要查看任务日志/调度时。

## 优先顺序

1. **MCP 工具**（已配置 `.cursor/mcp.json`）：`cron_list_tasks`、`cron_get_task`、`cron_run_task` 等
2. **CLI 兜底**：`python cronctl.py <action> [task-id]`
3. **WebUI**：`start-webui.bat`（Windows）或 `webui/start.sh`（Linux）

## 项目路径

- 根目录：仓库根（`cronctl.py` 所在目录）
- 可通过环境变量 `SPROUTCLAW_CRON_ROOT` 覆盖
- Linux 生产路径示例：`/shumengya/project/agent/sproutclaw-cron`

## MCP 工具速查

| 场景 | 工具 |
|------|------|
| 看有哪些任务 | `cron_list_tasks` |
| 看单个任务详情 | `cron_get_task` |
| 开/关/切换 | `cron_enable_task` / `cron_disable_task` / `cron_toggle_task` |
| 试跑 | `cron_run_task` → `cron_get_log` |
| 改调度 | `cron_update_schedule` |
| 新建任务 | `cron_list_templates` → `cron_create_task` |
| 同步系统 cron | `cron_sync_cron` |

## 新建任务流程

1. 确认 `task_id` 命名：`<主机名>-<功能描述>`（如 `smallmengya-gitea-repo-sync`）
2. `cron_create_task(task_id=..., runtime="python")` — **默认禁用**
3. 编辑 `<task-id>/run.py`（或对应入口）与 `schedule.cron`
4. `cron_run_task` 试跑，`cron_get_log` 确认
5. 用户明确要求后再 `cron_enable_task`

模板与约定详见根目录 [AGENTS.md](../../AGENTS.md)。

## CLI 兜底命令

```bash
python cronctl.py status
python cronctl.py status <task-id>
python cronctl.py enable|disable|toggle <task-id>
python cronctl.py run <task-id>
python cronctl.py sync-cron <task-id>
python cronctl.py enable all
```

Windows 将 `python` 换为实际 Python 路径；Linux 可用 `python3`。

## Agent 原则

- **默认新建任务保持禁用**，除非用户明确要求启用
- **不要修改** `_template*` 示例目录（除非用户要求）
- **schedule.cron** 调度入口保持 `cronctl.py run <task-id>` 形式
- Python 任务保留 `task_is_disabled` / `task_logging` / `acquire_cron_lock` 结构
- 非 Python 任务只改入口脚本；锁/日志/disable 由 `cronctl run` 统一处理
- 改完业务逻辑后先 `run` 再 `enable`

## 任务目录结构

```
<task-id>/
├── task.json          # 可选：runtime + entry + tags
├── run.py|run.js|run.sh|run.ps1
├── schedule.cron
├── logs/<task-id>.log
└── *.json             # 可选配置
```

禁用任务位于 `.disabled/<task-id>/`。

## 附加资源

- CLI 与 API 详情：[reference.md](reference.md)
- 环境检查脚本：[scripts/check_cron.py](scripts/check_cron.py)

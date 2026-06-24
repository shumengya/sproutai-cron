# SproutClaw Cron 参考

## cronctl.py 子命令

| 子命令 | 说明 | 示例 |
|--------|------|------|
| `status` | 列出或查看任务状态 | `cronctl.py status` |
| `enable` | 启用并同步 cron | `cronctl.py enable my-task` |
| `disable` | 禁用（移入 `.disabled/`） | `cronctl.py disable my-task` |
| `toggle` | 切换状态 | `cronctl.py toggle my-task` |
| `run` | 同步执行一次 | `cronctl.py run my-task` |
| `sync-cron` | 仅同步 cron.d / schtasks | `cronctl.py sync-cron my-task` |

别名：`on` → `enable`，`off` → `disable`。批量操作传 `all`。

## task.json

```json
{
  "runtime": "python",
  "entry": "run.py",
  "tags": ["标签1", "标签2"]
}
```

`runtime`：`python` | `javascript` | `bash` | `powershell`

## schedule.cron 格式

- 首行注释为任务描述（WebUI / MCP 会读取）
- cron 五段 + 用户 + 命令
- 命令应调用：`python3 <CRON_ROOT>/cronctl.py run <task-id>`

示例：

```cron
# 每天 08:00 同步仓库
0 8 * * * root /usr/bin/python3 /path/to/sproutclaw-cron/cronctl.py run my-task >/dev/null 2>&1
```

## WebUI API（FastAPI，端口见 start 脚本）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/tasks` | 任务列表 |
| GET | `/api/tasks/{id}` | 任务详情 |
| POST | `/api/tasks/{id}/enable` | 启用 |
| POST | `/api/tasks/{id}/disable` | 禁用 |
| POST | `/api/tasks/{id}/run` | 后台试跑 |
| GET | `/api/tasks/{id}/log?lines=200` | 日志 |
| PATCH | `/api/tasks/{id}/schedule` | 更新调度 |

## 语言模板

| 模板目录 | runtime | 入口 |
|----------|---------|------|
| `_template` | python | `run.py` |
| `_template-javascript` | javascript | `run.js` |
| `_template-bash` | bash | `run.sh` |
| `_template-powershell` | powershell | `run.ps1` |

## 公共库（Python 任务）

从 `lib/runner.py` 导入：

- `TaskContext.from_task_id(task_id)`
- `task_is_disabled(ctx)` / `task_logging(ctx)` / `acquire_cron_lock(...)`

从 `lib/notify.py` 导入：

- `TaskResult`、`send_task_summary(...)`

## 开关机制

- 启用：`.disabled/<id>/` → `<id>/`
- 禁用：`<id>/` → `.disabled/<id>/`
- 禁用后 cron 仍可触发，但 `cronctl run` 会直接跳过

## MCP 服务器

- 入口：`mcp-server/server.py`
- 配置：`.cursor/mcp.json`
- 依赖：`pip install -r mcp-server/requirements.txt`

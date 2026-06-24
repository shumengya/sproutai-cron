# sproutclaw-cron 开发规则

## 新建定时任务

**默认从 `_template/`（Python）复制**；也可选用其他语言模板：

| 模板目录 | 语言 | 入口 |
|---|---|---|
| `_template` | Python（默认） | `run.py` |
| `_template-javascript` | JavaScript | `run.js` |
| `_template-bash` | Bash | `run.sh` |
| `_template-powershell` | PowerShell | `run.ps1` |

```bash
TASK_ID="<主机名>-<功能描述>"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"
TEMPLATE="_template"   # 或 _template-javascript / _template-bash / _template-powershell

cp -a "$CRON_ROOT/$TEMPLATE" "$CRON_ROOT/$TASK_ID"
sed -i "s/$TEMPLATE/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"
# Bash 模板额外：chmod +x "$CRON_ROOT/$TASK_ID/run.sh"
```

然后：

1. **Python**：修改 `run.py`，保留 `task_is_disabled` / `task_logging` / `acquire_cron_lock` 结构
2. **其他语言**：只改入口脚本（`run.js` / `run.sh` / `run.ps1`）；disable / 锁 / 日志由 `cronctl run` 统一处理
3. 修改 `schedule.cron`：调整 cron 表达式与注释说明
4. 可选：任务专属 JSON 配置（如 `targets.json`）
5. 用 `python3 cronctl.py run <task-id>` 试跑，确认日志正常
6. 默认保持关闭；用户要求启用时再 `cronctl enable <task-id>`

## task.json

可选清单，声明运行时与入口（无则按入口文件自动推断）：

```json
{
  "runtime": "python",
  "entry": "run.py"
}
```

`runtime` 取值：`python` | `javascript` | `bash` | `powershell`

## 不要修改

- `_template*` 是 hello world 示例任务，也是新任务复制源；除非用户要求，不要改其示例行为
- 不要删除或移动已有任务的 `schedule.cron` 里对 `cronctl.py run` 的调用方式

## 任务目录约定

```
<task-id>/
├── task.json          # 可选：runtime + entry
├── run.py|run.js|run.sh|run.ps1
├── schedule.cron
├── logs/<task-id>.log
└── *.json             # 可选配置
```

禁用任务位于 `.disabled/<task-id>/`，开关用 `cronctl enable|disable|toggle`。

## 管理命令

```bash
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py status
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py run <task-id>
python3 /shumengya/project/agent/sproutclaw-cron/cronctl.py enable <task-id>
```

WebUI：`/shumengya/project/agent/sproutclaw-cron/webui/start.sh` 或 Windows 下 `start-webui.bat`

## AI Agent 集成

- **Skill**：`.cursor/skills/sproutclaw-cron/SKILL.md` — 操作规范与 workflow
- **MCP**：`.cursor/mcp.json` 注册 `sproutclaw-cron` 服务器；工具前缀 `cron_*`
- MCP 依赖：`pip install -r mcp-server/requirements.txt`

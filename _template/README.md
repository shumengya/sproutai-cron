# 定时任务模板 / 示例任务（Python）

`_template` 是默认可管理的 Python hello world 任务（每天 08:00）。
新建 Python 任务时复制本目录，改任务名与业务逻辑即可。

## 多语言模板

| 目录 | 语言 | 入口 |
|---|---|---|
| `_template` | **Python（默认）** | `run.py` |
| `_template-javascript` | JavaScript | `run.js` |
| `_template-bash` | Bash | `run.sh` |
| `_template-powershell` | PowerShell | `run.ps1` |

非 Python 任务只需编写入口脚本；disable / 锁 / 日志由 `cronctl run` 负责。

## 新建步骤

```bash
TASK_ID="smallmengya-my-new-task"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"

cp -a "$CRON_ROOT/_template" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"

# 编辑 run.py，替换 hello world 为实际业务逻辑
# 编辑 schedule.cron，调整 cron 表达式与注释

python3 "$CRON_ROOT/cronctl.py" status "$TASK_ID"
python3 "$CRON_ROOT/cronctl.py" run "$TASK_ID"
python3 "$CRON_ROOT/cronctl.py" enable "$TASK_ID"
```

## 必须文件

| 文件 | 说明 |
|---|---|
| `task.json` | 可选；声明 `runtime` 与 `entry` |
| `run.py` | Python 任务入口，导出 `run(ctx) -> int` |
| `schedule.cron` | cron 配置，须调用 `cronctl.py run <task-id>` |

## run.py 约定

1. 入口第一行检查 `task_is_disabled(ctx)`，关闭时直接 `return 0`
2. 使用 `task_logging(ctx)` 写日志到 `logs/<task-id>.log`
3. 使用 `acquire_cron_lock(ctx.lock_file, log=log)` 防止并发重入
4. 业务逻辑写在 `log("start")` 与 `log("end")` 之间
5. 需要汇总通知时使用 `notify.send_task_summary`

## schedule.cron 约定

- 保留 `SHELL` / `PATH` / `HOME` / `CRON_MAIL_ENABLED` 头部
- 用 `#` 注释写一句任务说明（WebUI 会读取展示）
- cron 行格式：`分 时 日 月 周 root python3 .../cronctl.py run <task-id> >/dev/null 2>&1`
- 本模板默认 **每天 08:00**（`0 8 * * *`）

## 任务 ID 命名

`<主机名>-<功能描述>`，例如 `smallmengya-gitea-repo-sync`。

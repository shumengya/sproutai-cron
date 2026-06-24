# Bash 定时任务模板

Bash hello world 示例。业务逻辑写在 `run.sh`；disable / 锁 / 日志由 cronctl 负责。

## 依赖

- Python 3（cronctl）
- Bash（Linux 自带；Windows 需 Git Bash 或 WSL）

## 新建任务

```bash
TASK_ID="smallmengya-my-bash-task"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"
cp -a "$CRON_ROOT/_template-bash" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template-bash/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"
chmod +x "$CRON_ROOT/$TASK_ID/run.sh"
```

## 其他语言模板

见 `_template-javascript/README.md` 中的对照表。

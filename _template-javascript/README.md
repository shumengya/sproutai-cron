# JavaScript 定时任务模板

Node.js hello world 示例。调度仍由 `cronctl.py run` 统一管理（disable / 锁 / 日志）。

## 依赖

- Python 3（cronctl）
- Node.js（`node` 在 PATH 中）

## 新建任务

```bash
TASK_ID="smallmengya-my-js-task"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"
cp -a "$CRON_ROOT/_template-javascript" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template-javascript/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"
```

编辑 `run.js` 与 `schedule.cron` 后试跑：

```bash
python3 "$CRON_ROOT/cronctl.py" run "$TASK_ID"
```

## 其他语言模板

| 目录 | 语言 |
|---|---|
| `_template` | Python（默认） |
| `_template-javascript` | JavaScript |
| `_template-bash` | Bash |
| `_template-powershell` | PowerShell |

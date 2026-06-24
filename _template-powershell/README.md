# PowerShell 定时任务模板

PowerShell hello world 示例。业务逻辑写在 `run.ps1`；disable / 锁 / 日志由 cronctl 负责。

## 依赖

- Python 3（cronctl）
- PowerShell（`pwsh` 或 Windows 自带 `powershell`）

## 新建任务

```bash
TASK_ID="smallmengya-my-ps-task"
CRON_ROOT="/shumengya/project/agent/sproutclaw-cron"
cp -a "$CRON_ROOT/_template-powershell" "$CRON_ROOT/$TASK_ID"
sed -i "s/_template-powershell/$TASK_ID/g" "$CRON_ROOT/$TASK_ID/schedule.cron"
```

Windows 上可直接：

```powershell
python D:\SmyProjects\AI\sproutclaw-cron\cronctl.py run _template-powershell
```

## 其他语言模板

见 `_template-javascript/README.md` 中的对照表。

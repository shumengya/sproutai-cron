# AI 早报

每天下午 **14:00** 从 [daily.juya.uk](https://daily.juya.uk) 拉取当日 Markdown 早报，排版后发飞书。

## 数据源

```text
https://daily.juya.uk/markdown/YYYY-MM-DD.md
```

- 默认**只发本地当天**期号（今天 7 月 22 日 → 只拉 `2026-07-22.md`）
- **发送前三重核对**，不一致则**拒发正文**（只告警失败）：
  1. 目标日期 = 本地今天（未设 `AI_DAILY_DATE` 时）
  2. URL 路径日期 = 目标日期
  3. 正文标题中的日期（如 `# AI 早报 2026-07-22`）= 目标日期
- **默认不会**回退昨天，避免把 7/21 当成 7/22 发出去
- 调试才用：`AI_DAILY_DATE=2026-07-21`（仍会核对正文标题是否为该日）

## 排版

- 去掉图片链接（机器人侧不便展示）
- 压缩多余空行
- 文首附原文链接
- 超长按 `AI_DAILY_MAX_CHARS`（默认 12000）截断并提示看原文

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `AI_DAILY_BASE_URL` | `https://daily.juya.uk/markdown` | 目录前缀 |
| `AI_DAILY_DATE` | （空=今天） | 强制日期；正式调度勿设 |
| `AI_DAILY_ALLOW_YESTERDAY` | `0` | 仅当天 404 时是否改试昨天（仍会核对正文日期） |
| `AI_DAILY_MODE` | `full` | `full` 全文 / `overview` 仅概览 |
| `AI_DAILY_MAX_CHARS` | `12000` | 消息最大字符数 |

## 试跑

```bash
# 发「今天」：不要设 AI_DAILY_DATE
cronctl run ai-daily-briefing

# 调试历史期号（正文标题日期必须一致）
set AI_DAILY_DATE=2026-07-22
cronctl run ai-daily-briefing
cronctl log ai-daily-briefing --lines 50
```

## 调度

```cron
0 14 * * *
```

需 `cronctl serve`（或 WebUI 已自动拉起调度）保持运行。

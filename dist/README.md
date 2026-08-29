# dist — 预编译二进制

由 sproutai monorepo 根目录 `node build.mjs` 生成，或在本仓执行 `go build ./cmd/cronctl`。

| 路径 | 平台 |
|------|------|
| `windows-amd64/cronctl.exe` | Windows x86_64 |
| `linux-amd64/cronctl` | Linux x86_64 |

特性：

- `CGO_ENABLED=0` 静态链接（无外部 C 运行时依赖）
- **WebUI 前端已 `go:embed` 打进二进制**，运行 `cronctl web` 无需 `webui/frontend` 目录
- 仍需项目根（或 `SPROUTAI_CRON_ROOT`）下有 `cron-tasks/` 以管理任务

```bash
# 重新构建（sproutai 根目录）
node build.mjs

# 或仅本仓
go build -o dist/linux-amd64/cronctl ./cmd/cronctl

# 使用
export SPROUTAI_CRON_ROOT=/path/to/sproutai-cron
./dist/linux-amd64/cronctl status
./dist/linux-amd64/cronctl web
```

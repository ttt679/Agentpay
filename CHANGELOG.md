# Changelog

## [0.2.0] — 2026-06-10

### Added
- 代理模式：`cmd/agentpay-proxy` 独立二进制，商户零代码接入
- `UPSTREAM_URL` / `LISTEN_PORT` 配置项支持
- 凭证去重（`sync.Map`，trade_no 30 分钟窗口）
- 履约回执 `context.WithTimeout` 30 秒超时保护
- 优雅关闭（SIGINT/SIGTERM）
- `/health` 健康检查端点
- `.env.example` 配置模板
- GitHub Actions CI（`go build` + `go vet`）

### Changed
- 项目定位从「库模式 middleware library」升级为「代理 + 库双模式」
- `README.md` 快速开始重写为代理模式优先
- 目录整理：文档移入 `docs/`，脚本移入 `scripts/`
- `mock_server.py` 协议头更新为当前 402 协议

### Fixed
- `verify.go` 遗留注释清理
- `run_server.sh` 修正为代理模式

## [0.1.0] — 2024

### Added
- 初始版本：Gin 框架 AI 付 402 协议 SDK
- RSA2 签名（`billing.go`）
- 支付宝支付验证与履约回执（`verify.go`）
- Gin 中间件（`middleware.go`）
- 库模式示例（`examples/gin_example.go`）

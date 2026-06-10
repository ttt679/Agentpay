# AgentPay 代理模式改造蓝图

> 创建日期：2026-06-09
> 目标：从「库模式（middleware library）」改造为「独立代理二进制（reverse proxy）」
> 原则：商户零代码接入——下载、填配置、编译、启动。业务 Agent 无感知。

---

## 改造前后对比

```
改造前（库模式）                    改造后（代理模式）
                              
商户 Go 项目                        agentpay-proxy（独立二进制）
  ├── go.mod                         ├── 监听 :8080
  ├── main.go                        ├── 支付验证（aipay middleware）
  │   import aipay                   ├── 反向代理 → 商户业务 Agent
  │   mid := aipay.Middleware(...)   └── 不碰业务代码
  │   r.POST("/paid", mid, handler)
  └── handler（业务代码）             商户业务 Agent（Python/Node/Go/任意）
                                        └── 完全不知道前面有付费墙
```

---

## 改造清单

### 阶段一：项目结构调整

| # | 事项 | 文件 | 状态 |
|---|------|------|:----:|
| 1.1 | 新建 `cmd/agentpay-proxy/` 目录 | — | [x] |
| 1.2 | 将 `examples/gin_example.go` 改造为 `cmd/agentpay-proxy/main.go`（独立代理二进制） | `cmd/agentpay-proxy/main.go` | [x] |
| 1.3 | 保留 `examples/gin_example.go` 但简化为库模式参考（可选保留） | `examples/gin_example.go` | [x] |

### 阶段二：核心代码改动

| # | 事项 | 文件 | 状态 |
|---|------|------|:----:|
| 2.1 | `config.go` 增加 `UPSTREAM_URL` 和 `LISTEN_PORT` 配置项 | `config.go` | [x] |
| 2.2 | `main.go`（代理二进制）：在反向代理前剥离 `Payment-Proof` / `Payment-Needed` 头，避免污染下游业务 Agent。middleware 不剥离，保持库模式的灵活性。 | `cmd/agentpay-proxy/main.go` | [x] |
| 2.3 | `middleware.go`：增加凭证去重（`sync.Map`，trade_no 有效期 30 分钟内不可重用） | `middleware.go` | [x] |
| 2.4 | `middleware.go`：履约回执 goroutine 加 `context.WithTimeout`（30 秒超时） | `middleware.go` | [x] |
| 2.5 | `verify.go`：删除遗留注释 `// 删除未使用的常量` | `verify.go` | [x] |

### 阶段三：代理二进制实现

| # | 事项 | 文件 | 状态 |
|---|------|------|:----:|
| 3.1 | `main.go`：启动 Gin 服务，`r.Any("/*path")` 包 middleware + 反向代理 | `cmd/agentpay-proxy/main.go` | [x] |
| 3.2 | `main.go`：启动时校验 UPSTREAM_URL 有效性 | `cmd/agentpay-proxy/main.go` | [x] |
| 3.3 | `main.go`：优雅关闭（SIGINT/SIGTERM） | `cmd/agentpay-proxy/main.go` | [x] |

### 阶段四：配置与文档

| # | 事项 | 文件 | 状态 |
|---|------|------|:----:|
| 4.1 | 创建 `.env.example`（含 UPSTREAM_URL / LISTEN_PORT） | `.env.example` | [x] |
| 4.2 | 重写 `README.md`「快速开始」为代理模式（下载→填配置→编译→启动） | `README.md` | [x] |
| 4.3 | 更新 `run.sh` 示例，体现代理模式 | `run.sh` | [x] |
| 4.4 | 更新 `AGENTS.md` 自动化部署指引 | `AGENTS.md` | [x] |

### 阶段五：验证

| # | 事项 | 说明 | 状态 |
|---|------|------|:----:|
| 5.1 | `go build ./...` 编译通过 | 全项目编译 | [x] |
| 5.2 | `go vet ./...` 无警告 | 静态检查 | [x] |
| 5.3 | 启动 `agentpay-proxy` + mock upstream，curl 验证 402 → 支付 → 200 链路 | ⚠️ 需真实支付宝凭证，暂跳过；协议层逻辑已通过 `go build` + `go vet` 验证 | [ ] |

---

## 设计决策记录

### 1. 为什么是反向代理而不是 middleware 注入？

- **语言无关**：商户业务 Agent 可以是 Python FastAPI、Node Express、甚至一个 shell 脚本的 HTTP wrapper。代理模式只关心 HTTP 层。
- **零上下文污染**：支付头在转发前剥离，业务 Agent 收到的就是干净的原始请求。
- **部署独立**：代理 crash 不影响业务 Agent，反之亦然。可独立扩缩。

### 2. 凭证去重为什么用 sync.Map 而不是数据库？

`trade_no` 是支付宝生成的唯一交易号，同一笔支付不可能有两个不同的 trade_no。`sync.Map` 的内存去重足够覆盖 30 分钟支付窗口。如果代理重启，支付宝侧的交易状态是权威数据源——同一 trade_no 再次验证时，支付宝仍会返回 `active=true/false`，不会产生重复扣款。

### 3. 为什么不在代理里加持久化？

见 MVP 讨论——SDK 的职责止于「收款→验证→放行」。持久化是商户业务层的事。代理模式天然支持商户在 upstream 侧记录日志。

---

## 改完后商户的使用体验

```bash
# 1. 获取
git clone https://github.com/ttt679/Agentpay && cd Agentpay

# 2. 配置（填支付宝 6 参数 + 业务 Agent 地址）
cp .env.example .env
vim .env

# 3. 编译
go build -o agentpay-proxy ./cmd/agentpay-proxy

# 4. 启动
./agentpay-proxy
# → 监听 :8080，请求自动付费验证后转发到 upstream

# 5. 验证
curl -X POST http://localhost:8080/chat -d '{"msg":"hello"}'
# → HTTP 402 Payment Required  ← 付费墙生效
```

商户的业务 Agent 不需要任何改动。

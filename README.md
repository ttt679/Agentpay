# AgentPay — AI 智能体付费 SDK

为 Gin 框架提供的支付宝 **AI 付** 402 协议 SDK。一行代码让你的 AI API 端点具备付费能力——用户智能体调用前自动完成支付宝支付，你的智能体代码不需要感知支付逻辑。

## 工作原理

```
用户Agent             SDK 支付层           商户业务层(你的代码)      支付宝
  │                      │                      │                    │
  │  POST /paid/api      │                      │                    │
  │  (无 Payment-Proof)  │                      │                    │
  │ ────────────────────→│                      │                    │
  │                      │ RSA2 签名账单         │                    │
  │  402 ←───────────────│                      │                    │
  │  Payment-Needed      │                      │                    │
  │                      │                      │                    │
  │  [AI付 MCP 自动支付]  │                      │                    │
  │                      │                      │                    │
  │  GET /paid/api       │                      │                    │
  │  Payment-Proof       │                      │                    │
  │ ────────────────────→│                      │                    │
  │                      │ 验证凭证 ──────────────────────────────→│
  │                      │      ←── 通过 ─────────────────────────│
  │                      │                      │                    │
  │                      │ ──→ 放行(c.Next) ──→│                    │
  │                      │                      │ 执行业务 translate()│
  │  200 ←──────────────────────────────────────│                    │
  │  {"result":"你好"}   │                      │                    │
  │                      │ 履约回执 ──────────────────────────────→│
```

## 安装

```bash
go get github.com/ttt679/Agentpay@latest
```

## 快速开始（代理模式）

代理模式让你**不写一行代码**就给现有智能体加上付费墙。

```bash
# 1. 克隆
git clone https://github.com/ttt679/Agentpay && cd Agentpay

# 2. 配置
cp .env.example .env
# 编辑 .env，填入支付宝凭证 + 你业务 Agent 的地址
vim .env

# 3. 编译
go build -o agentpay-proxy ./cmd/agentpay-proxy

# 4. 启动（你的业务 Agent 不需要任何改动）
./agentpay-proxy
# → 监听 :8080，所有请求自动付费验证后转发到 AIPAY_UPSTREAM_URL

# 5. 验证
curl -X POST http://localhost:8080/chat -d '{"msg":"hello"}'
# → HTTP 402 Payment Required ← 付费墙生效
```

**代理启动后，你的业务 Agent（Python/Node/Go/任意语言）收到的就是干净的 HTTP 请求——它不知道前面有付费墙。**

### 如果你仍想用库模式（在 Go 代码中嵌入 middleware）

参考 `examples/gin_example.go`，在自己 Go 项目的路由上包一层 `aipay.Middleware(...)`。代理模式已覆盖绝大多数场景，库模式仅供需要深度定制的开发者使用。

## 环境变量

| 变量 | 必填 | 说明 |
|------|:--:|------|
| `AIPAY_APP_ID` | 是 | 支付宝应用 ID |
| `AIPAY_PRIVATE_KEY` 或 `_FILE` | 是 | 应用私钥 (PEM, PKCS#8) |
| `AIPAY_PUBLIC_KEY` 或 `_FILE` | 是 | 支付宝公钥 (PEM) |
| `AIPAY_SELLER_ID` | 是 | 商户 2088 ID |
| `AIPAY_SERVICE_ID` | 是 | AI 收 service_id |
| `AIPAY_UPSTREAM_URL` | 是* | 商户业务 Agent 地址（代理模式必填，库模式忽略） |
| `AIPAY_SELLER_NAME` | 否 | 商户名称 (显示在账单中) |
| `AIPAY_LISTEN_PORT` | 否 | 代理监听端口 (默认 8080) |
| `AIPAY_GATEWAY` | 否 | 支付宝网关 (默认生产环境) |
| `AIPAY_SKIP_RESPONSE_VERIFY` | 否 | 跳过响应验签 (仅调试，默认 false) |

> 支持 `_FILE` 后缀从文件读取密钥，优先级高于直接设置环境变量。

## 部署到云服务器

```bash
# 1. 克隆
git clone https://github.com/ttt679/Agentpay.git && cd Agentpay

# 2. 配置（推荐用 .env 文件）
cp .env.example .env
vim .env

# 3. 编译
go build -o agentpay-proxy ./cmd/agentpay-proxy

# 4. 启动（systemd 或 nohup）
nohup ./agentpay-proxy > /dev/null 2>&1 &

# 5. 验证
curl http://你的服务器IP:8080/health
# → {"status":"ok","upstream":"http://localhost:9000","port":"8080"}

curl -X POST http://你的服务器IP:8080/chat -d '{"msg":"test"}'
# → HTTP 402 Payment Required ✓
```

## 接入现有智能体

代理模式**不需要改智能体代码**。启动代理时指定 `AIPAY_UPSTREAM_URL` 指向你的业务 Agent 地址即可。

代理负责支付验证，通过后请求原样转发给业务 Agent，Agent 完全不知道前面有付费。支持任何语言的 HTTP 服务。

## 下一步：让你的服务被更多用户发现

SDK 解决了收款问题，但谁来调用你的服务？

→ 注册 [AgentPay 社区](https://api.looom.top)，你的服务会进入智能体可搜索的付费服务目录，被更多用户智能体发现和调用。提供调用量统计、收入仪表盘、服务排行。

## 项目结构

```
├── billing.go               # 402 账单 + RSA2 签名
├── middleware.go             # Gin 中间件 (Payment-Proof 解析 + 验证 + 去重)
├── verify.go                 # 支付宝 API (验证 + 履约回执)
├── config.go                 # 环境变量配置
├── go.mod / go.sum
├── .env.example              # 配置模板
├── cmd/agentpay-proxy/
│   └── main.go               # 独立代理二进制入口
├── examples/
│   ├── gin_example.go        # 库模式参考
│   └── static/               # 前端 demo
├── docs/
│   ├── BLUEPRINT.md          # 代理模式改造蓝图
│   ├── ANALYSIS.md           # 开源就绪分析
│   └── AGENTS.md             # AI 智能体配置指引
├── scripts/
│   ├── run.sh                # 本地启动脚本
│   ├── run_server.sh         # 云服务器启动脚本
│   └── mock_server.py        # Mock 服务器（无凭证本地联调）
├── .github/workflows/
│   └── test.yml              # CI（build + vet + test）
├── CHANGELOG.md
└── LICENSE
```

## 许可证

MIT

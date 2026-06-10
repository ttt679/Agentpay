# AGENTS.md — AgentPay 配置指引

> 给 AI 智能体读的：这个项目是什么，需要人做什么，机器做什么，最终得到什么。

## 项目是什么

**AgentPay** 是一个支付宝 AI 付 SDK。把它放在商户现有智能体前面，就能让智能体的 API 变成付费的——用户调用前自动完成支付宝 0.05 元支付，商户的智能体不需要改代码。

## 人需要做的事（智能体做不了）

```
□ 1. 去支付宝开放平台 (https://open.alipay.com) 创建 AI 智能体支付应用
     拿到 6 个值：APP_ID / 私钥 / 公钥 / SELLER_ID / SERVICE_ID / 商户名

□ 2. 告诉智能体这 6 个值
```

## 智能体需要做的事（全自动）

```
□ 1. 检查 Go 是否安装 (go version)，未安装则引导安装
□ 2. 在这台机器上找到或创建密钥文件
     - 私钥 → private_key.pem
     - 公钥 → alipay_public_key.pem
□ 3. 生成 .env 文件或 systemd 配置，填入用户提供的 6 个值
□ 4. 识别用户现有智能体的端口（询问或扫描 3000/5000/8081/9000 等常见端口）
□ 5. 编译: go build -o agentpay-proxy ./cmd/agentpay-proxy
□ 6. 生成 systemd 或 docker-compose 配置
□ 7. 启动: systemctl start agentpay-proxy
□ 8. curl 测试: 确认 /health 返回 200，其他路径返回 HTTP 402
□ 9. 告诉用户: "完成。对外暴露 http://你的IP:8080/ 即可收费"
```

## 结果

- 用户访问 `http://你的IP:8080/任意路径` → 返回 402 账单
- 用户智能体调 AI 付完成支付 → 0.05 元到你支付宝账户
- 支付后代理自动转发请求到商户业务 Agent → 拿到结果
- 商户业务 Agent 代码没有动

## 升级订阅（可选）

如果用户想要调用量统计、服务排名、智能体搜索引擎收录：

```
□ 从 .env 中移除 reportConfig = nil 这行（或添加 reportConfig 配置）
□ 填入 NewAPI 的 API Key 和网关地址
□ 重启代理

之后每次成功交易会上报 NewAPI，数据进入 AgentPay 社区广场。
```

## 已知注意事项

- 使用 `gin.New()` + Logger + Recovery（避免 gzip 兼容问题）
- 使用 `r.Any` 自动注册 GET/POST 路由
- SDK 价格必须与支付宝后台 AI 商品定价一致
- 代理模式下业务 Agent 地址通过 `AIPAY_UPSTREAM_URL` 指定

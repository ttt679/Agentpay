#!/bin/bash
# AgentPay 代理模式 — 本地启动脚本
# ⚠️ 请替换下方占位符为你在支付宝开放平台申请的正式凭证

# 切换到项目根目录（脚本位于 scripts/ 子目录）
cd "$(dirname "$0")/.."

export AIPAY_APP_ID=请替换为你的APP_ID
export AIPAY_SELLER_ID=请替换为你的2088商户ID
export AIPAY_SERVICE_ID=请替换为你的SERVICE_ID
export AIPAY_SELLER_NAME=请替换为你的商户名称

# 公钥（从支付宝开放平台下载）
export AIPAY_PUBLIC_KEY='-----BEGIN PUBLIC KEY-----
请替换为你的支付宝公钥
-----END PUBLIC KEY-----'

# 私钥（从支付宝开放平台下载，PKCS#8 格式）
export AIPAY_PRIVATE_KEY='-----BEGIN PRIVATE KEY-----
请替换为你的应用私钥
-----END PRIVATE KEY-----'

echo "=== AgentPay Go Proxy ==="
echo "APP_ID: $AIPAY_APP_ID"
echo "SELLER: $AIPAY_SELLER_ID"
echo "SERVICE: $AIPAY_SERVICE_ID"
echo "UPSTREAM: $AIPAY_UPSTREAM_URL"
echo "========================="

go run ./cmd/agentpay-proxy

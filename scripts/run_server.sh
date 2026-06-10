#!/bin/bash
# AgentPay 代理模式 — 云服务器启动脚本（使用密钥文件）
# ⚠️ 请替换下方占位符为你在支付宝开放平台申请的正式凭证

# 切换到项目根目录（脚本位于 scripts/ 子目录）
cd "$(dirname "$0")/.."

export AIPAY_APP_ID=请替换为你的APP_ID
export AIPAY_SELLER_ID=请替换为你的2088商户ID
export AIPAY_SERVICE_ID=请替换为你的SERVICE_ID
export AIPAY_SELLER_NAME=请替换为你的商户名称
export AIPAY_PRIVATE_KEY_FILE=请替换为你的私钥文件路径
export AIPAY_PUBLIC_KEY_FILE=请替换为你的公钥文件路径

# 商户业务 Agent 地址（代理模式必填）
export AIPAY_UPSTREAM_URL=http://localhost:9000

echo "=== AgentPay Proxy (server mode) ==="
echo "APP_ID:    $AIPAY_APP_ID"
echo "SELLER:    $AIPAY_SELLER_ID"
echo "SERVICE:   $AIPAY_SERVICE_ID"
echo "UPSTREAM:  $AIPAY_UPSTREAM_URL"
echo "====================================="

go run ./cmd/agentpay-proxy

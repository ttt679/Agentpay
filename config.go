package aipay

import (
	"os"
)

// Config holds the merchant credentials and proxy settings for AI Pay (402 protocol).
type Config struct {
	AppID           string // 支付宝应用 ID
	PrivateKey      string // 应用私钥 (PEM, PKCS#8)
	AlipayPublicKey string // 支付宝公钥 (PEM)
	SellerID        string // 商户 2088 ID
	ServiceID       string // AI收 service_id
	SellerName      string // 商户名称
	Gateway         string // 支付宝网关地址（空则默认生产环境）

	// Proxy mode (agentpay-proxy binary)
	UpstreamURL       string // 商户业务 Agent 地址（代理模式必填）
	UpstreamAuthHeader string // 转发时注入的鉴权头（可选，如 "Bearer sk-xxx"）
	ListenPort        string // 代理监听端口（默认 8080）
}

// LoadConfig reads configuration from environment variables.
// If AIPAY_PRIVATE_KEY_FILE is set, it reads the key from that file.
// Otherwise it reads AIPAY_PRIVATE_KEY directly.
func LoadConfig() *Config {
	cfg := &Config{
		AppID:              os.Getenv("AIPAY_APP_ID"),
		SellerID:           os.Getenv("AIPAY_SELLER_ID"),
		ServiceID:          os.Getenv("AIPAY_SERVICE_ID"),
		SellerName:         os.Getenv("AIPAY_SELLER_NAME"),
		Gateway:            os.Getenv("AIPAY_GATEWAY"),
		UpstreamURL:        os.Getenv("AIPAY_UPSTREAM_URL"),
		UpstreamAuthHeader: os.Getenv("AIPAY_UPSTREAM_AUTH_HEADER"),
		ListenPort:         os.Getenv("AIPAY_LISTEN_PORT"),
	}

	// Provide production gateway as default (AI Pay methods are only available on production gateway)
	// Sandbox auto-detection removed because aipay methods are not supported on sandbox

	// Read private key from file or env
	if keyFile := os.Getenv("AIPAY_PRIVATE_KEY_FILE"); keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err == nil {
			cfg.PrivateKey = string(data)
		}
	}
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = os.Getenv("AIPAY_PRIVATE_KEY")
	}

	// Read public key from file or env
	if keyFile := os.Getenv("AIPAY_PUBLIC_KEY_FILE"); keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err == nil {
			cfg.AlipayPublicKey = string(data)
		}
	}
	if cfg.AlipayPublicKey == "" {
		cfg.AlipayPublicKey = os.Getenv("AIPAY_PUBLIC_KEY")
	}

	return cfg
}

func (c *Config) IsValid() bool {
	return c.AppID != "" && c.PrivateKey != "" && c.AlipayPublicKey != "" &&
		c.SellerID != "" && c.ServiceID != ""
}

// GetGateway returns the Alipay gateway URL, defaulting to production.
func (c *Config) GetGateway() string {
	if c.Gateway != "" {
		return c.Gateway
	}
	return "https://openapi.alipay.com/gateway.do"
}

// GetListenPort returns the proxy listen port, defaulting to 8080.
func (c *Config) GetListenPort() string {
	if c.ListenPort != "" {
		return c.ListenPort
	}
	return "8080"
}



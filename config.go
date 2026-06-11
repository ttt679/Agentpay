package aipay

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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

// DefaultConfigFile is the default path for the persistent config file.
const DefaultConfigFile = "agentpay-config.json"

// LoadConfig reads configuration from config file first, falling back to environment variables.
//
// Priority:
//  1. agentpay-config.json in the working directory (created by admin web UI)
//  2. Environment variables (AIPAY_APP_ID, AIPAY_PRIVATE_KEY_FILE, etc.)
func LoadConfig() *Config {
	// ── Try config file first ──
	if cfg, err := LoadFromFile(DefaultConfigFile); err == nil && cfg.IsValid() {
		log.Printf("[aipay] loaded config from %s", DefaultConfigFile)
		return cfg
	}

	// ── Fallback to environment variables ──
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

	// Read private key from file or env
	if keyFile := os.Getenv("AIPAY_PRIVATE_KEY_FILE"); keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err == nil {
			cfg.PrivateKey = string(data)
		} else {
			log.Printf("[aipay] WARNING: AIPAY_PRIVATE_KEY_FILE set but cannot read %s: %v", keyFile, err)
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
		} else {
			log.Printf("[aipay] WARNING: AIPAY_PUBLIC_KEY_FILE set but cannot read %s: %v", keyFile, err)
		}
	}
	if cfg.AlipayPublicKey == "" {
		cfg.AlipayPublicKey = os.Getenv("AIPAY_PUBLIC_KEY")
	}

	return cfg
}

// LoadFromFile reads a Config from a JSON file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveToFile writes the Config to a JSON file with restricted permissions.
func (c *Config) SaveToFile(path string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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



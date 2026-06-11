// agentpay-verifier — AI 付支付验证 + 管理后台
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	aipay "github.com/ttt679/Agentpay"
	"github.com/gin-gonic/gin"
)

type runtimeConfig struct {
	mu                  sync.RWMutex
	UpstreamURL         string
	UpstreamAuthHeader  string
	UpstreamFormat      string // "dify" or "openai"
	UpstreamModel       string // model name for openai format
	UpstreamExtraHeader string // extra header "Key: Value"
	StartTime           time.Time
	RequestCount        int64
	PaidCount           int64

	// Alipay credentials (persisted to config.json, restart required to take effect)
	AppID           string
	SellerID        string
	ServiceID       string
	SellerName      string
	PrivateKey      string
	AlipayPublicKey string
	Gateway         string
}

func (rc *runtimeConfig) snapshot() gin.H {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return gin.H{
		"upstream_url":          rc.UpstreamURL,
		"has_auth":              rc.UpstreamAuthHeader != "",
		"upstream_format":       rc.UpstreamFormat,
		"upstream_model":        rc.UpstreamModel,
		"upstream_extra_header": rc.UpstreamExtraHeader,
		"uptime":                time.Since(rc.StartTime).Round(time.Second).String(),
		"request_count":         rc.RequestCount,
		"paid_count":            rc.PaidCount,
		// Alipay credentials (masked for security)
		"app_id":          rc.AppID,
		"seller_id":       rc.SellerID,
		"service_id":      rc.ServiceID,
		"seller_name":     rc.SellerName,
		"has_private_key": rc.PrivateKey != "",
		"has_public_key":  rc.AlipayPublicKey != "",
		"gateway":         rc.Gateway,
		"creds_valid":     rc.AppID != "" && rc.PrivateKey != "" && rc.AlipayPublicKey != "" && rc.SellerID != "" && rc.ServiceID != "",
	}
}

func (rc *runtimeConfig) update(upstreamURL, authHeader, format, model, extraHeader string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.UpstreamURL = upstreamURL
	rc.UpstreamAuthHeader = authHeader
	rc.UpstreamFormat = format
	rc.UpstreamModel = model
	rc.UpstreamExtraHeader = extraHeader
}

// updateCredentials saves Alipay credentials (requires restart to take effect).
func (rc *runtimeConfig) updateCredentials(appID, sellerID, serviceID, sellerName, privateKey, publicKey, gateway string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.AppID = appID
	rc.SellerID = sellerID
	rc.ServiceID = serviceID
	rc.SellerName = sellerName
	rc.PrivateKey = privateKey
	rc.AlipayPublicKey = publicKey
	rc.Gateway = gateway
}

// toAipayConfig converts runtimeConfig to aipay.Config for persistence.
func (rc *runtimeConfig) toAipayConfig() *aipay.Config {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return &aipay.Config{
		AppID:             rc.AppID,
		SellerID:          rc.SellerID,
		ServiceID:         rc.ServiceID,
		SellerName:        rc.SellerName,
		PrivateKey:        rc.PrivateKey,
		AlipayPublicKey:   rc.AlipayPublicKey,
		Gateway:           rc.Gateway,
		UpstreamURL:       rc.UpstreamURL,
		UpstreamAuthHeader: rc.UpstreamAuthHeader,
	}
}

// loadCredentialsFromConfig populates credential fields from an aipay.Config.
func (rc *runtimeConfig) loadCredentialsFromConfig(cfg *aipay.Config) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.AppID = cfg.AppID
	rc.SellerID = cfg.SellerID
	rc.ServiceID = cfg.ServiceID
	rc.SellerName = cfg.SellerName
	rc.PrivateKey = cfg.PrivateKey
	rc.AlipayPublicKey = cfg.AlipayPublicKey
	rc.Gateway = cfg.Gateway
	if rc.UpstreamURL == "" {
		rc.UpstreamURL = cfg.UpstreamURL
	}
	if rc.UpstreamAuthHeader == "" {
		rc.UpstreamAuthHeader = cfg.UpstreamAuthHeader
	}
}

func (rc *runtimeConfig) getExtraHeader() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.UpstreamExtraHeader
}

func (rc *runtimeConfig) getModel() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.UpstreamModel == "" {
		return "gpt-3.5-turbo"
	}
	return rc.UpstreamModel
}

func (rc *runtimeConfig) getUpstreamURL() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.UpstreamURL
}

func (rc *runtimeConfig) getAuthHeader() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.UpstreamAuthHeader
}

func (rc *runtimeConfig) getFormat() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.UpstreamFormat == "" {
		return "dify"
	}
	return rc.UpstreamFormat
}

func (rc *runtimeConfig) incRequest() {
	rc.mu.Lock()
	rc.RequestCount++
	rc.mu.Unlock()
}

func (rc *runtimeConfig) incPaid() {
	rc.mu.Lock()
	rc.PaidCount++
	rc.mu.Unlock()
}

func main() {
	cfg := aipay.LoadConfig()
	if !cfg.IsValid() {
		log.Println("[agentpay-verifier] ⚠ Alipay credentials missing — admin panel will allow web-based setup")
	}

	rc := &runtimeConfig{
		UpstreamURL:        cfg.UpstreamURL,
		UpstreamAuthHeader: cfg.UpstreamAuthHeader,
		StartTime:          time.Now(),
	}
	rc.loadCredentialsFromConfig(cfg)

	// Auto-create config.json on first run so all config is in one place
	if cfg.IsValid() {
		if _, err := os.Stat(aipay.DefaultConfigFile); os.IsNotExist(err) {
			if err := rc.toAipayConfig().SaveToFile(aipay.DefaultConfigFile); err != nil {
				log.Printf("[agentpay-verifier] WARNING: cannot auto-create %s: %v", aipay.DefaultConfigFile, err)
			} else {
				log.Printf("[agentpay-verifier] ✓ created %s (all config in one place)", aipay.DefaultConfigFile)
			}
		}
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// ── 管理后台 ──
	r.GET("/admin", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, adminHTML)
	})

	// ── 配置 API ──
	r.GET("/api/config", func(c *gin.Context) {
		c.JSON(200, rc.snapshot())
	})

	r.POST("/api/config", func(c *gin.Context) {
		var body struct {
			UpstreamURL         string `json:"upstream_url"`
			UpstreamAuthHeader  string `json:"upstream_auth_header"`
			UpstreamFormat      string `json:"upstream_format"`
			UpstreamModel       string `json:"upstream_model"`
			UpstreamExtraHeader string `json:"upstream_extra_header"`
			// Alipay credentials (saved to config.json, requires restart)
			AppID           string `json:"app_id"`
			SellerID        string `json:"seller_id"`
			ServiceID       string `json:"service_id"`
			SellerName      string `json:"seller_name"`
			PrivateKey      string `json:"private_key"`
			AlipayPublicKey string `json:"alipay_public_key"`
			Gateway         string `json:"gateway"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		// Update upstream settings (instant)
		rc.update(body.UpstreamURL, body.UpstreamAuthHeader, body.UpstreamFormat, body.UpstreamModel, body.UpstreamExtraHeader)
		log.Printf("[admin] upstream config updated: url=%s format=%s model=%s", body.UpstreamURL, body.UpstreamFormat, body.UpstreamModel)

		// Update credentials (requires restart)
		credsUpdated := false
		hasNewCreds := body.AppID != "" || body.SellerID != "" || body.ServiceID != "" || body.PrivateKey != "" || body.AlipayPublicKey != ""
		if hasNewCreds {
			rc.updateCredentials(body.AppID, body.SellerID, body.ServiceID, body.SellerName, body.PrivateKey, body.AlipayPublicKey, body.Gateway)
			credsUpdated = true
		}

		// Persist to config.json (preserves existing credentials if not being updated)
		saveCfg := rc.toAipayConfig()
		if err := saveCfg.SaveToFile(aipay.DefaultConfigFile); err != nil {
			log.Printf("[admin] ERROR: failed to save config: %v", err)
			c.JSON(500, gin.H{"saved": false, "error": err.Error()})
			return
		}

		resp := gin.H{"saved": true}
		if credsUpdated {
			resp["restart_needed"] = true
			resp["message"] = "凭证已保存到 agentpay-config.json，重启服务后生效"
		}
		c.JSON(200, resp)
	})

	// ── 测试 API ──
	r.POST("/api/test", func(c *gin.Context) {
		var body struct {
			Message string `json:"message"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.Message == "" {
			body.Message = "hello"
		}

		respBody, err := forwardUpstream(rc.getUpstreamURL(), rc.getAuthHeader(), rc.getFormat(), rc.getModel(), rc.getExtraHeader(),
			[]byte(body.Message))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Data(200, "application/json", respBody)
	})

	// ── 代理 upstream 的 parameters ──
	r.GET("/api/upstream-info", func(c *gin.Context) {
		upstreamBase := rc.getUpstreamURL()
		if upstreamBase == "" {
			c.JSON(200, gin.H{"error": "no upstream configured"})
			return
		}
		infoURL := upstreamBase
		if len(infoURL) > len("/chat-messages") && infoURL[len(infoURL)-len("/chat-messages"):] == "/chat-messages" {
			infoURL = infoURL[:len(infoURL)-len("/chat-messages")] + "/parameters"
		}
		req, _ := http.NewRequest("GET", infoURL, nil)
		if ah := rc.getAuthHeader(); ah != "" {
			req.Header.Set("Authorization", ah)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.JSON(200, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		c.Data(200, "application/json", body)
	})

	// ── 健康检查 ──
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"mode":    "verifier",
			"port":    cfg.GetListenPort(),
			"upstream": rc.getUpstreamURL(),
		})
	})

	// ── 支付验证 ──
	mid := aipay.Middleware(cfg, 0.05, "AI智能体服务")
	r.NoRoute(mid, func(c *gin.Context) {
		rc.incRequest()
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		tradeNo, _ := c.Get("aipay_trade_no")

		if rc.getUpstreamURL() != "" {
			rc.incPaid()
			respBody, err := forwardUpstream(rc.getUpstreamURL(), rc.getAuthHeader(), rc.getFormat(), rc.getModel(), rc.getExtraHeader(), bodyBytes)
			if err != nil {
				log.Printf("[agentpay-verifier] upstream forward failed: %v", err)
				c.JSON(200, gin.H{
					"status":        "verified",
					"user_message":  string(bodyBytes),
					"trade_no":      tradeNo,
					"forward_error": err.Error(),
				})
				return
			}
			c.Data(200, "application/json", respBody)
			return
		}

		c.JSON(200, gin.H{
			"status":       "verified",
			"user_message": string(bodyBytes),
			"trade_no":     tradeNo,
		})
	})

	// ── 启动 ──
	srv := &http.Server{Addr: ":" + cfg.GetListenPort(), Handler: r}
	go func() {
		log.Printf("[agentpay-verifier] listening on :%s", cfg.GetListenPort())
		log.Printf("[agentpay-verifier] admin: http://localhost:%s/admin", cfg.GetListenPort())
		if rc.getUpstreamURL() != "" {
			log.Printf("[agentpay-verifier] upstream: %s", rc.getUpstreamURL())
		}
		if !cfg.IsValid() {
			log.Printf("[agentpay-verifier] ⚠ 支付宝凭证未配置，请访问 /admin 进行配置")
		} else {
			log.Printf("[agentpay-verifier] ✓ 支付宝凭证已配置")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[agentpay-verifier] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("[agentpay-verifier] stopped")
}

func forwardUpstream(upstreamURL, authHeader, format, model, extraHeader string, body []byte) ([]byte, error) {
	query := string(body)
	var in map[string]interface{}
	if json.Unmarshal(body, &in) == nil {
		if msg, ok := in["message"].(string); ok && msg != "" {
			query = msg
		}
	}

	if model == "" {
		model = "gpt-3.5-turbo"
	}

	var outBody []byte
	switch format {
	case "openai":
		outBody, _ = json.Marshal(map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": query},
			},
		})
	default:
		outBody, _ = json.Marshal(map[string]interface{}{
			"query":         query,
			"inputs":        map[string]interface{}{},
			"response_mode": "blocking",
			"user":          "agentpay-user",
		})
	}

	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(outBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if extraHeader != "" {
		parts := splitHeader(extraHeader)
		if len(parts) == 2 {
			req.Header.Set(parts[0], parts[1])
		}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[agentpay-verifier] upstream %d (%d bytes)", resp.StatusCode, len(respBody))

	var upResp map[string]interface{}
	if json.Unmarshal(respBody, &upResp) == nil {
		switch format {
		case "openai":
			if choices, ok := upResp["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if msg, ok := choice["message"].(map[string]interface{}); ok {
						if content, ok := msg["content"].(string); ok {
							a, _ := json.Marshal(map[string]interface{}{"response": content})
							return a, nil
						}
					}
				}
			}
		default:
			if answer, ok := upResp["answer"]; ok {
				a, _ := json.Marshal(map[string]interface{}{"response": answer})
				return a, nil
			}
		}
	}
	return respBody, nil
}

func splitHeader(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])}
		}
	}
	return nil
}

const adminHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>AgentPay Admin</title>
<style>
:root{--bg:#f5f5f5;--card:#fff;--text:#333;--muted:#888;--accent:#534AB7;--green:#1D9E75;--red:#D85A30;--border:#e0e0e0;--radius:12px}
*{margin:0;padding:0;box-sizing:border-box}
body{font:14px/1.6 -apple-system,sans-serif;background:var(--bg);color:var(--text)}
.header{background:var(--accent);color:#fff;padding:16px 24px;font-size:18px;font-weight:500;display:flex;justify-content:space-between;align-items:center}
.header span{font-size:13px;opacity:.85}
.tabs{display:flex;gap:0;background:#fff;border-bottom:1px solid var(--border);padding:0 24px}
.tab{padding:12px 20px;cursor:pointer;border:none;background:none;font-size:14px;color:var(--muted);border-bottom:2px solid transparent}
.tab.active{color:var(--accent);border-bottom-color:var(--accent);font-weight:500}
.main{max-width:900px;margin:24px auto;padding:0 16px}
.card{background:var(--card);border-radius:var(--radius);padding:24px;margin-bottom:16px;border:1px solid var(--border)}
.card h3{font-size:15px;font-weight:500;margin-bottom:12px;color:var(--accent)}
.row{display:flex;gap:12px;flex-wrap:wrap}
.stat{flex:1;min-width:140px;background:var(--bg);border-radius:8px;padding:16px;text-align:center}
.stat .val{font-size:24px;font-weight:500;color:var(--accent)}
.stat .lab{font-size:12px;color:var(--muted);margin-top:4px}
.stat.green .val{color:var(--green)}
label{display:block;font-size:13px;font-weight:500;margin:12px 0 4px;color:var(--muted)}
input,textarea{width:100%;padding:10px;border:1px solid var(--border);border-radius:8px;font:14px monospace}
input:focus,textarea:focus{outline:none;border-color:var(--accent)}
button{padding:10px 24px;background:var(--accent);color:#fff;border:none;border-radius:8px;cursor:pointer;font-size:14px}
button:hover{opacity:.9}
button.secondary{background:var(--bg);color:var(--text);border:1px solid var(--border)}
.response-box{background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:8px;font:13px monospace;white-space:pre-wrap;max-height:300px;overflow:auto;margin-top:12px}
.endpoint{display:flex;align-items:center;gap:8px;padding:10px;border-bottom:1px solid var(--border);font:13px monospace}
.endpoint:last-child{border:none}
.method{font-weight:500;padding:2px 8px;border-radius:4px;font-size:11px;min-width:40px;text-align:center}
.method.get{background:#e3f2fd;color:#1565c0}
.method.post{background:#e8f5e9;color:#2e7d32}
.url{flex:1;color:var(--text)}
.desc{color:var(--muted);font-size:12px;font-family:inherit}
.hidden{display:none}
.toast{position:fixed;bottom:24px;right:24px;padding:12px 24px;border-radius:8px;color:#fff;font-size:14px;z-index:100;animation:in .3s}
.toast.ok{background:var(--green)}
.toast.err{background:var(--red)}
@keyframes in{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}
</style>
</head>
<body>

<div class="header">
  AgentPay Admin
  <span id="status-dot">● 运行中</span>
</div>

<div class="tabs">
  <button class="tab active" onclick="switchTab('dashboard')">仪表盘</button>
  <button class="tab" onclick="switchTab('settings')">设置</button>
  <button class="tab" onclick="switchTab('test')">测试</button>
  <button class="tab" onclick="switchTab('docs')">API 文档</button>
</div>

<div class="main">

  <!-- 仪表盘 -->
  <div id="tab-dashboard">
    <div class="row" id="stats"></div>
    <div class="card">
      <h3>服务端点</h3>
      <div id="dashboard-endpoints"></div>
    </div>
  </div>

  <!-- 设置 -->
  <div id="tab-settings" class="hidden">
    <div class="card">
      <h3>🔑 支付宝凭证配置</h3>
      <p style="color:var(--muted);font-size:13px;margin-bottom:12px">
        首次使用请在此填写支付宝商户凭证。保存后需<b>重启服务</b>生效。<br>
        <span style="color:var(--accent)">凭证状态：<b id="creds-badge">检测中...</b></span>
      </p>
      <label>应用 ID (APP_ID)</label>
      <input id="cfg-app-id" placeholder="2021xxxxxxxxxxxx">
      <label>商户 ID (SELLER_ID)</label>
      <input id="cfg-seller-id" placeholder="2088xxxxxxxxxxxx">
      <label>AI收 Service ID</label>
      <input id="cfg-service-id" placeholder="API_xxxxxxxxxxxx">
      <label>商户名称（显示在账单中）</label>
      <input id="cfg-seller-name" placeholder="AI智能体服务">
      <label>应用私钥 (PEM 文本)</label>
      <textarea id="cfg-private-key" rows="5" placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----" style="font-size:12px"></textarea>
      <label>支付宝公钥 (PEM 文本)</label>
      <textarea id="cfg-public-key" rows="5" placeholder="-----BEGIN PUBLIC KEY-----&#10;...&#10;-----END PUBLIC KEY-----" style="font-size:12px"></textarea>
      <label>支付宝网关（留空用生产环境）</label>
      <input id="cfg-gateway" placeholder="https://openapi.alipay.com/gateway.do">
      <div style="margin-top:16px;display:flex;gap:8px">
        <button onclick="saveCredentials()">💾 保存凭证</button>
        <button class="secondary" onclick="loadCredentials()">刷新</button>
      </div>
      <div id="creds-result" style="margin-top:8px;font-size:13px"></div>
    </div>
    <div class="card">
      <h3>上游配置</h3>
      <p style="color:var(--muted);font-size:13px;margin-bottom:12px">
        修改后即时生效，无需重启
      </p>
      <label>上游 URL</label>
      <input id="cfg-url" placeholder="http://localhost/v1/chat-messages">
      <label>Authorization Header（Bearer xxx）</label>
      <input id="cfg-auth" placeholder="Bearer app-xxx">
      <label>上游格式</label>
      <select id="cfg-format" style="width:100%;padding:10px;border:1px solid var(--border);border-radius:8px;font:14px sans-serif">
        <option value="dify">Dify / Chatflow</option>
        <option value="openai">OpenAI Compatible</option>
      </select>
      <label>模型名称（OpenAI 格式时使用）</label>
      <input id="cfg-model" placeholder="gpt-3.5-turbo">
      <label>额外请求头（格式: Key: Value）</label>
      <input id="cfg-extra" placeholder="X-LangBot-Group: aipay">
      <div style="margin-top:16px;display:flex;gap:8px">
        <button onclick="saveConfig()">保存配置</button>
        <button class="secondary" onclick="loadConfig()">刷新</button>
      </div>
      <div id="config-result" style="margin-top:8px;font-size:13px"></div>
    </div>
    <div class="card" id="upstream-card">
      <h3>上游应用信息</h3>
      <div id="upstream-info">点击下方按钮获取</div>
      <button class="secondary" onclick="fetchUpstreamInfo()" style="margin-top:12px">获取上游参数</button>
      <div class="response-box" id="upstream-raw" style="margin-top:12px;display:none"></div>
    </div>
  </div>

  <!-- 测试 -->
  <div id="tab-test" class="hidden">
    <div class="card">
      <h3>快速测试</h3>
      <label>测试消息</label>
      <textarea id="test-msg" rows="3" placeholder="输入测试内容...">hello</textarea>
      <div style="margin-top:12px;display:flex;gap:8px">
        <button onclick="testUpstream()">发送测试（直连上游）</button>
        <button class="secondary" onclick="test402()">测试 402 响应</button>
      </div>
      <div class="response-box" id="test-result" style="display:none"></div>
    </div>
  </div>

  <!-- API 文档 -->
  <div id="tab-docs" class="hidden">
    <div class="card" id="full-docs"></div>
  </div>

</div>

<script>
const BASE = '';

function switchTab(name) {
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  document.querySelector('[onclick="switchTab(\'' + name + '\')"]').classList.add('active');
  document.querySelectorAll('[id^="tab-"]').forEach(d => d.classList.add('hidden'));
  document.getElementById('tab-' + name).classList.remove('hidden');
  if (name === 'dashboard') loadDashboard();
  if (name === 'settings') loadConfig();
}

async function loadDashboard() {
  try {
    const r = await fetch(BASE + '/api/config');
    const d = await r.json();
    document.getElementById('stats').innerHTML =
      '<div class="stat"><div class="val">' + d.uptime + '</div><div class="lab">运行时间</div></div>' +
      '<div class="stat"><div class="val">' + d.request_count + '</div><div class="lab">请求总数</div></div>' +
      '<div class="stat green"><div class="val">' + d.paid_count + '</div><div class="lab">付费通过</div></div>' +
      '<div class="stat"><div class="val">' + (d.upstream_url ? '已配置' : '未配置') + '</div><div class="lab">上游状态</div></div>' +
      '<div class="stat"><div class="val" style="color:' + (d.creds_valid ? 'var(--green)' : 'var(--red)') + '">' + (d.creds_valid ? '已配置' : '未配置') + '</div><div class="lab">支付宝凭证</div></div>';
    document.getElementById('dashboard-endpoints').innerHTML = renderEndpoints();
  } catch(e) { document.getElementById('stats').innerHTML = '<p>加载失败</p>'; }
}

function renderEndpoints() {
  const eps = [
    {method:'POST', path:'/chat', desc:'支付验证入口，402 需支付 / 200 返回 AI 回复'},
    {method:'GET', path:'/health', desc:'健康检查'},
    {method:'GET', path:'/admin', desc:'管理后台'},
    {method:'GET', path:'/api/config', desc:'获取运行时配置'},
    {method:'POST', path:'/api/config', desc:'修改上游配置'},
    {method:'POST', path:'/api/test', desc:'直连上游测试'},
    {method:'GET', path:'/api/upstream-info', desc:'获取上游应用参数'},
  ];
  return eps.map(e =>
    '<div class="endpoint">' +
    '<span class="method ' + e.method.toLowerCase() + '">' + e.method + '</span>' +
    '<span class="url">' + e.path + '</span>' +
    '<span class="desc">' + e.desc + '</span>' +
    '</div>'
  ).join('');
}

async function loadConfig() {
  const r = await fetch(BASE + '/api/config');
  const d = await r.json();
  document.getElementById('cfg-url').value = d.upstream_url || '';
  document.getElementById('cfg-auth').value = '';
  document.getElementById('cfg-format').value = d.upstream_format || 'dify';
  document.getElementById('cfg-model').value = d.upstream_model || '';
  document.getElementById('cfg-extra').value = d.upstream_extra_header || '';
  loadCredentialsData(d);
}

async function loadCredentials() {
  const r = await fetch(BASE + '/api/config');
  const d = await r.json();
  loadCredentialsData(d);
}

function loadCredentialsData(d) {
  document.getElementById('cfg-app-id').value = d.app_id || '';
  document.getElementById('cfg-seller-id').value = d.seller_id || '';
  document.getElementById('cfg-service-id').value = d.service_id || '';
  document.getElementById('cfg-seller-name').value = d.seller_name || '';
  document.getElementById('cfg-gateway').value = d.gateway || '';
  // Mask key textareas with placeholder text to avoid exposing full key in DOM
  if (d.has_private_key) {
    document.getElementById('cfg-private-key').placeholder = '已配置 (点击保存可覆盖)';
    document.getElementById('cfg-private-key').value = '';
  } else {
    document.getElementById('cfg-private-key').placeholder = '-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----';
  }
  if (d.has_public_key) {
    document.getElementById('cfg-public-key').placeholder = '已配置 (点击保存可覆盖)';
    document.getElementById('cfg-public-key').value = '';
  } else {
    document.getElementById('cfg-public-key').placeholder = '-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----';
  }
  updateCredsBadge(d.creds_valid);
}

function updateCredsBadge(valid) {
  const badge = document.getElementById('creds-badge');
  if (badge) {
    badge.textContent = valid ? '✅ 已配置' : '⚠ 未配置';
    badge.style.color = valid ? 'var(--green)' : 'var(--red)';
  }
}

async function saveCredentials() {
  const body = {
    app_id: document.getElementById('cfg-app-id').value,
    seller_id: document.getElementById('cfg-seller-id').value,
    service_id: document.getElementById('cfg-service-id').value,
    seller_name: document.getElementById('cfg-seller-name').value,
    private_key: document.getElementById('cfg-private-key').value,
    alipay_public_key: document.getElementById('cfg-public-key').value,
    gateway: document.getElementById('cfg-gateway').value,
    // Preserve existing upstream config
    upstream_url: document.getElementById('cfg-url').value,
    upstream_auth_header: document.getElementById('cfg-auth').value,
    upstream_format: document.getElementById('cfg-format').value,
    upstream_model: document.getElementById('cfg-model').value,
    upstream_extra_header: document.getElementById('cfg-extra').value
  };
  const r = await fetch(BASE + '/api/config', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const d = await r.json();
  const el = document.getElementById('creds-result');
  if (d.saved) {
    el.innerHTML = '<span style="color:var(--green)">✅ 凭证已保存到 agentpay-config.json</span>'
      + (d.restart_needed ? ' <span style="color:var(--red)">⚠ 请重启服务使凭证生效</span>' : '');
    document.getElementById('cfg-private-key').value = '';
    document.getElementById('cfg-public-key').value = '';
    loadCredentials();
  } else {
    el.innerHTML = '<span style="color:var(--red)">❌ 保存失败: ' + JSON.stringify(d) + '</span>';
  }
}

async function saveConfig() {
  const body = {
    upstream_url: document.getElementById('cfg-url').value,
    upstream_auth_header: document.getElementById('cfg-auth').value,
    upstream_format: document.getElementById('cfg-format').value,
    upstream_model: document.getElementById('cfg-model').value,
    upstream_extra_header: document.getElementById('cfg-extra').value
  };
  const r = await fetch(BASE + '/api/config', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const d = await r.json();
  if (d.saved) {
    toast('配置已保存', 'ok');
    document.getElementById('cfg-auth').value = '';
  } else {
    toast('保存失败: ' + JSON.stringify(d), 'err');
  }
}

async function testUpstream() {
  const msg = document.getElementById('test-msg').value;
  const box = document.getElementById('test-result');
  box.style.display = 'block';
  box.textContent = '请求中...';
  try {
    const r = await fetch(BASE + '/api/test', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:msg})});
    const t = await r.text();
    try { box.textContent = JSON.stringify(JSON.parse(t), null, 2); }
    catch { box.textContent = t; }
  } catch(e) { box.textContent = '错误: ' + e.message; }
}

async function test402() {
  const msg = document.getElementById('test-msg').value;
  const box = document.getElementById('test-result');
  box.style.display = 'block';
  box.textContent = '请求中...';
  try {
    const r = await fetch(BASE + '/chat', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:msg})});
    const t = await r.text();
    const hdr = r.headers.get('Payment-Needed');
    box.textContent = 'HTTP ' + r.status + '\n' + (hdr ? 'Payment-Needed: ' + hdr.substring(0,80) + '...\n\n' : '\n') + (t.length > 500 ? t.substring(0,500) + '...' : t);
  } catch(e) { box.textContent = '错误: ' + e.message; }
}

async function fetchUpstreamInfo() {
  const box = document.getElementById('upstream-raw');
  box.style.display = 'block';
  box.textContent = '获取中...';
  try {
    const r = await fetch(BASE + '/api/upstream-info');
    const t = await r.text();
    try { box.textContent = JSON.stringify(JSON.parse(t), null, 2); }
    catch { box.textContent = t; }
  } catch(e) { box.textContent = '错误: ' + e.message; }
}

function toast(msg, type) {
  const el = document.createElement('div');
  el.className = 'toast ' + type;
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 2500);
}

// init
loadDashboard();
document.getElementById('tab-docs').querySelector('.card').innerHTML =
  '<h3>API 端点</h3>' + renderEndpoints() +
  '<h3 style="margin-top:24px">请求格式</h3>' +
  '<div class="response-box">POST /chat\nContent-Type: application/json\n\n{"message": "你的问题"}\n\n第一次请求返回:\nHTTP 402 Payment Required\nPayment-Needed: &lt;base64url 账单&gt;\n\n支付后重试返回:\nHTTP 200\n{"response": "AI 的回复"}</div>' +
  '<h3 style="margin-top:24px">其他端点说明</h3>' +
  '<p style="color:var(--muted);font-size:13px">' +
  'GET /health → 服务状态<br>' +
  'GET /admin → 当前管理后台<br>' +
  'GET /api/config → 运行时配置（上游 URL 等）<br>' +
  'POST /api/config → 修改上游配置<br>' +
  'POST /api/test → 绕过支付，直连上游测试<br>' +
  'GET /api/upstream-info → 代理获取上游应用参数</p>';
</script>
</body>
</html>`

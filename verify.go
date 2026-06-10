package aipay

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type VerifyResult struct {
	TradeNo    string `json:"trade_no"`
	Amount     string `json:"amount"`
	ResourceID string `json:"resource_id"`
	Active     bool   `json:"active"`
	OutTradeNo string `json:"out_trade_no"`
}

// VerifyPaymentProof calls Alipay to verify a payment proof from the buyer.
func VerifyPaymentProof(cfg *Config, proof PaymentProof) (*VerifyResult, error) {
	bizContent := map[string]string{
		"trade_no":      proof.TradeNo,
		"payment_proof": proof.PaymentProof,
	}
	if proof.ClientSession != "" {
		bizContent["client_session"] = proof.ClientSession
	}

	result, err := callAlipayAPI(cfg, "alipay.aipay.agent.payment.verify", bizContent)
	if err != nil {
		return nil, err
	}

	var verifyResp struct {
		Response VerifyResult `json:"alipay_aipay_agent_payment_verify_response"`
		Sign     string       `json:"sign"`
	}
	if err := json.Unmarshal(result, &verifyResp); err != nil {
		return nil, fmt.Errorf("parse verify response: %w", err)
	}

	if !verifyResp.Response.Active {
		return &verifyResp.Response, fmt.Errorf("payment proof inactive, trade_no=%s", proof.TradeNo)
	}

	return &verifyResp.Response, nil
}

// ConfirmFulfillment sends a fulfillment confirmation to Alipay after delivering the resource.
func ConfirmFulfillment(cfg *Config, tradeNo string) error {
	bizContent := map[string]string{
		"trade_no": tradeNo,
	}

	result, err := callAlipayAPI(cfg, "alipay.aipay.agent.fulfillment.confirm", bizContent)
	if err != nil {
		return err
	}

	var fulfillResp struct {
		Response struct {
			Code    string `json:"code"`
			Msg     string `json:"msg"`
			TradeNo string `json:"trade_no"`
		} `json:"alipay_aipay_agent_fulfillment_confirm_response"`
	}
	if err := json.Unmarshal(result, &fulfillResp); err != nil {
		return fmt.Errorf("parse fulfill response: %w", err)
	}

	if fulfillResp.Response.Code != "10000" {
		return fmt.Errorf("fulfillment failed: code=%s msg=%s", fulfillResp.Response.Code, fulfillResp.Response.Msg)
	}

	return nil
}

// PaymentProof is the decoded Payment-Proof header from the buyer.
type PaymentProof struct {
	TradeNo       string `json:"trade_no"`
	PaymentProof  string `json:"payment_proof"`
	ClientSession string `json:"client_session"`
}

// callAlipayAPI makes a signed request to the Alipay OpenAPI gateway.
func callAlipayAPI(cfg *Config, method string, bizContent map[string]string) ([]byte, error) {
	bizJSON, _ := json.Marshal(bizContent)

	params := url.Values{}
	params.Set("app_id", cfg.AppID)
	params.Set("method", method)
	params.Set("format", "JSON")
	params.Set("charset", "utf-8")
	params.Set("sign_type", "RSA2")
	params.Set("timestamp", time.Now().Format("2006-01-02 15:04:05"))
	params.Set("version", "1.0")
	params.Set("biz_content", string(bizJSON))

	// Build sign string (dictionary order of all params)
	signStr := buildAlipaySignString(params)
	sign, err := rsaSign(cfg.PrivateKey, signStr)
	if err != nil {
		return nil, fmt.Errorf("alipay sign: %w", err)
	}
	params.Set("sign", sign)

	// POST form-urlencoded
	resp, err := http.PostForm(cfg.GetGateway(), params)
	if err != nil {
		return nil, fmt.Errorf("alipay request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read alipay response: %w", err)
	}

	// Debug: log raw Alipay response to file when AIPAY_DEBUG_LOG is set
	if debugLog := os.Getenv("AIPAY_DEBUG_LOG"); debugLog != "" {
		if f, err := os.OpenFile(debugLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "[AIPAY-DEBUG] HTTP %d, method=%s, body: %s\n\n",
				resp.StatusCode, method, string(body))
			f.Close()
		}
	}

	// Verify Alipay's response signature (skip if AIPAY_SKIP_RESPONSE_VERIFY=true)
	if os.Getenv("AIPAY_SKIP_RESPONSE_VERIFY") == "true" {
		fmt.Fprintf(os.Stderr, "[AIPAY] WARNING: skipping Alipay response signature verification\n")
	} else {
		if err := verifyAlipayResponse(cfg, body); err != nil {
			fmt.Fprintf(os.Stderr, "[AIPAY] response signature verification failed (set AIPAY_SKIP_RESPONSE_VERIFY=true to skip): %v\nRAW: %s\n", err, safeTruncate2(string(body), 1000))
			return nil, fmt.Errorf("verify alipay response: %w", err)
		}
	}

	return body, nil
}

func buildAlipaySignString(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params.Get(k))
	}
	return b.String()
}

func verifyAlipayResponse(cfg *Config, body []byte) error {
	// Extract sign (base64 ASCII, safe to unmarshal)
	var wrapper struct {
		Sign string `json:"sign"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return err
	}
	if wrapper.Sign == "" {
		return nil // some error responses have no sign, skip verification
	}

	// IMPORTANT: Use raw bytes for sign content, NOT json.Unmarshal.
	// Alipay may return GBK-encoded Chinese characters in fields like sub_msg,
	// and Go's json.Unmarshal corrupts these by converting Latin-1 bytes to UTF-8.
	// We must use the exact raw bytes that Alipay signed.
	respKey, respRawBytes := extractResponseRaw(body)
	if respKey == "" {
		return nil
	}

	// AI Pay API signs only the response JSON value itself, without "key=" prefix
	_ = respKey
	signContentBytes := respRawBytes

	// Verify with RSA2
	block, _ := pem.Decode([]byte(cfg.AlipayPublicKey))
	if block == nil {
		return fmt.Errorf("parse alipay public key")
	}
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse alipay public key: %w", err)
	}
	rsaPub, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("not RSA public key")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(wrapper.Sign)
	if err != nil {
		return fmt.Errorf("decode sign: %w", err)
	}

	hashed := sha256.Sum256(signContentBytes)
	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], sigBytes)
}

// extractResponseRaw finds the *_response key and its raw JSON value bytes
// directly from the HTTP response body, preserving all bytes including non-UTF8.
func extractResponseRaw(body []byte) (key string, rawValue []byte) {
	// Find a key ending with "_response"
	idx := bytes.Index(body, []byte("_response"))
	if idx < 0 {
		return "", nil
	}

	// Find the start of the key (the preceding '"')
	keyStart := bytes.LastIndexByte(body[:idx], '"')
	if keyStart < 0 {
		return "", nil
	}
	key = string(body[keyStart+1 : idx+len("_response")])

	// Find the value: skip '":' after the key
	valStart := idx + len("_response") + 2 // skip _response":
	if valStart >= len(body) {
		return key, nil
	}

	// Count braces to find the matching closing brace
	depth := 0
	valEnd := valStart
	for i := valStart; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				valEnd = i + 1
				goto done
			}
		}
	}
done:
	return key, body[valStart:valEnd]
}

func safeTruncate2(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

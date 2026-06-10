package aipay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"testing"
	"time"
)

// generateTestKeyPair creates an RSA key pair for testing and returns PEM strings.
func generateTestKeyPair(t *testing.T) (privateKeyPEM, publicKeyPEM string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	// PKCS#8 private key
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	}))

	// PKIX public key
	pkix, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal PKIX: %v", err)
	}
	publicKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pkix,
	}))

	return
}

func TestBuildSignString(t *testing.T) {
	params := map[string]string{
		"zebra":  "1",
		"apple":  "2",
		"mango":  "3",
		"amount": "0.05",
	}
	got := buildSignString(params)

	// Must be dictionary-ordered
	expected := "amount=0.05&apple=2&mango=3&zebra=1"
	if got != expected {
		t.Errorf("buildSignString = %q, want %q", got, expected)
	}
}

func TestBuildSignString_SingleEntry(t *testing.T) {
	params := map[string]string{"foo": "bar"}
	got := buildSignString(params)
	if got != "foo=bar" {
		t.Errorf("buildSignString = %q, want %q", got, "foo=bar")
	}
}

func TestNewBill(t *testing.T) {
	bill := NewBill("ORDER_123", 0.05, "测试服务", "/api/chat")

	if bill.OutTradeNo != "ORDER_123" {
		t.Errorf("OutTradeNo = %q, want ORDER_123", bill.OutTradeNo)
	}
	if bill.Amount != 0.05 {
		t.Errorf("Amount = %f, want 0.05", bill.Amount)
	}
	if bill.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", bill.Currency)
	}
	if bill.ResourceID != "/api/chat" {
		t.Errorf("ResourceID = %q, want /api/chat", bill.ResourceID)
	}
	if bill.GoodsName != "测试服务" {
		t.Errorf("GoodsName = %q, want 测试服务", bill.GoodsName)
	}

	// PayBefore should be ~30 minutes in the future
	payBefore, err := time.Parse(time.RFC3339, bill.PayBefore)
	if err != nil {
		t.Fatalf("PayBefore not valid RFC3339: %v", err)
	}
	diff := time.Until(payBefore)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Errorf("PayBefore is %v from now, want ~30m", diff)
	}
}

func TestRsaSign_Valid(t *testing.T) {
	privPEM, pubPEM := generateTestKeyPair(t)

	content := "amount=0.05&currency=CNY"
	sig, err := rsaSign(privPEM, content)
	if err != nil {
		t.Fatalf("rsaSign: %v", err)
	}
	if sig == "" {
		t.Fatal("signature is empty")
	}

	// Verify with public key
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}

	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		t.Fatal("failed to parse public key PEM")
	}
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	rsaPub := pubKey.(*rsa.PublicKey)

	hashed := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], sigBytes); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
}

func TestRsaSign_InvalidKey(t *testing.T) {
	_, err := rsaSign("not a valid PEM", "content")
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestBuildPaymentNeeded(t *testing.T) {
	privPEM, pubPEM := generateTestKeyPair(t)

	cfg := &Config{
		AppID:           "test_app_id",
		PrivateKey:      privPEM,
		AlipayPublicKey: pubPEM,
		SellerID:        "2088000000000000",
		ServiceID:       "test_service_id",
		SellerName:      "测试商户",
	}

	bill := NewBill("ORDER_001", 1.50, "AI翻译服务", "/api/translate")

	encoded, err := BuildPaymentNeeded(cfg, bill)
	if err != nil {
		t.Fatalf("BuildPaymentNeeded: %v", err)
	}
	if encoded == "" {
		t.Fatal("BuildPaymentNeeded returned empty string")
	}

	// Decode the base64 URL-encoded payload
	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(encoded)
	if err != nil {
		// Try with padding
		decoded, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("decode Payment-Needed: %v", err)
		}
	}

	var payload paymentNeeded
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatalf("unmarshal Payment-Needed: %v", err)
	}

	// Verify protocol fields
	if payload.Protocol.OutTradeNo != "ORDER_001" {
		t.Errorf("OutTradeNo = %q", payload.Protocol.OutTradeNo)
	}
	if payload.Protocol.Amount != "1.50" {
		t.Errorf("Amount = %q", payload.Protocol.Amount)
	}
	if payload.Protocol.Currency != "CNY" {
		t.Errorf("Currency = %q", payload.Protocol.Currency)
	}
	if payload.Protocol.SellerSignType != "RSA2" {
		t.Errorf("SellerSignType = %q", payload.Protocol.SellerSignType)
	}
	if payload.Protocol.SellerUniqueID != "2088000000000000" {
		t.Errorf("SellerUniqueID = %q", payload.Protocol.SellerUniqueID)
	}

	// Verify seller signature
	sigBytes, err := base64.StdEncoding.DecodeString(payload.Protocol.SellerSignature)
	if err != nil {
		t.Fatalf("decode seller signature: %v", err)
	}
	block, _ := pem.Decode([]byte(pubPEM))
	pubKey, _ := x509.ParsePKIXPublicKey(block.Bytes)
	rsaPub := pubKey.(*rsa.PublicKey)

	// Reconstruct the sign string
	signStr := buildSignString(map[string]string{
		"amount":       "1.50",
		"currency":     "CNY",
		"goods_name":   "AI翻译服务",
		"out_trade_no": "ORDER_001",
		"pay_before":   bill.PayBefore,
		"resource_id":  "/api/translate",
		"seller_id":    "2088000000000000",
		"service_id":   "test_service_id",
	})

	hashed := sha256.Sum256([]byte(signStr))
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], sigBytes); err != nil {
		t.Fatalf("seller signature verification failed: %v", err)
	}
}

func TestBuildPaymentNeeded_MissingKey(t *testing.T) {
	cfg := &Config{
		AppID:    "test",
		SellerID: "2088",
	}
	bill := NewBill("ORDER_001", 0.05, "test", "/test")

	_, err := BuildPaymentNeeded(cfg, bill)
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

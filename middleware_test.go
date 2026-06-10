package aipay

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParsePaymentProof_StandardBase64(t *testing.T) {
	proof := PaymentProof{
		TradeNo:       "2024060900000001",
		PaymentProof:  "alipay_proof_string_here",
		ClientSession: "session_abc",
	}
	jsonBytes, _ := json.Marshal(proof)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	result, err := parsePaymentProof(encoded)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "2024060900000001" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
	if result.PaymentProof != "alipay_proof_string_here" {
		t.Errorf("PaymentProof = %q", result.PaymentProof)
	}
	if result.ClientSession != "session_abc" {
		t.Errorf("ClientSession = %q", result.ClientSession)
	}
}

func TestParsePaymentProof_RawStdEncoding(t *testing.T) {
	proof := PaymentProof{
		TradeNo:      "TRADE_002",
		PaymentProof: "proof_xyz",
	}
	jsonBytes, _ := json.Marshal(proof)
	// RawStdEncoding (no padding) — alipay-bot uses this
	encoded := base64.RawStdEncoding.EncodeToString(jsonBytes)

	result, err := parsePaymentProof(encoded)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "TRADE_002" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
}

func TestParsePaymentProof_URLEncoding(t *testing.T) {
	proof := PaymentProof{
		TradeNo:      "TRADE_003",
		PaymentProof: "proof_url",
	}
	jsonBytes, _ := json.Marshal(proof)
	encoded := base64.URLEncoding.EncodeToString(jsonBytes)

	result, err := parsePaymentProof(encoded)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "TRADE_003" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
}

func TestParsePaymentProof_RawURLEncoding(t *testing.T) {
	proof := PaymentProof{
		TradeNo:      "TRADE_004",
		PaymentProof: "proof_raw_url",
	}
	jsonBytes, _ := json.Marshal(proof)
	encoded := base64.RawURLEncoding.EncodeToString(jsonBytes)

	result, err := parsePaymentProof(encoded)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "TRADE_004" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
}

func TestParsePaymentProof_PlainJSON(t *testing.T) {
	// Raw JSON string (not base64-encoded)
	proof := PaymentProof{
		TradeNo:      "TRADE_005",
		PaymentProof: "plain_json_proof",
	}
	jsonBytes, _ := json.Marshal(proof)

	result, err := parsePaymentProof(string(jsonBytes))
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "TRADE_005" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
}

func TestParsePaymentProof_WrappedFormat(t *testing.T) {
	// The "protocol" wrapper format used by alipay-bot
	wrapper := struct {
		Protocol PaymentProof `json:"protocol"`
		Method   struct {
			ClientSession string `json:"client_session"`
		} `json:"method"`
	}{
		Protocol: PaymentProof{
			TradeNo:      "TRADE_006",
			PaymentProof: "wrapped_proof",
		},
		Method: struct {
			ClientSession string `json:"client_session"`
		}{
			ClientSession: "wrapped_session",
		},
	}
	jsonBytes, _ := json.Marshal(wrapper)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	result, err := parsePaymentProof(encoded)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "TRADE_006" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
	if result.PaymentProof != "wrapped_proof" {
		t.Errorf("PaymentProof = %q", result.PaymentProof)
	}
	if result.ClientSession != "wrapped_session" {
		t.Errorf("ClientSession = %q (should inherit from method)", result.ClientSession)
	}
}

func TestParsePaymentProof_SimpleFormat(t *testing.T) {
	// "trade_no:payment_proof" colon-separated format
	header := "ORDER_2024:alipay_signature_value"

	result, err := parsePaymentProof(header)
	if err != nil {
		t.Fatalf("parsePaymentProof: %v", err)
	}
	if result.TradeNo != "ORDER_2024" {
		t.Errorf("TradeNo = %q", result.TradeNo)
	}
	if result.PaymentProof != "alipay_signature_value" {
		t.Errorf("PaymentProof = %q", result.PaymentProof)
	}
}

func TestParsePaymentProof_InvalidBase64(t *testing.T) {
	// Completely invalid input
	_, err := parsePaymentProof("!!!not-valid-base64-or-json!!!")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestParsePaymentProof_EmptyString(t *testing.T) {
	_, err := parsePaymentProof("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParsePaymentProof_InvalidJSON_InBase64(t *testing.T) {
	// Valid base64 but invalid JSON inside
	encoded := base64.StdEncoding.EncodeToString([]byte("{not valid json}"))
	_, err := parsePaymentProof(encoded)
	if err == nil {
		t.Fatal("expected error for invalid JSON in base64")
	}
}

func TestSafeTruncate_Short(t *testing.T) {
	got := safeTruncate("hello", 10)
	if got != "hello" {
		t.Errorf("safeTruncate = %q, want %q", got, "hello")
	}
}

func TestSafeTruncate_Long(t *testing.T) {
	got := safeTruncate("hello world this is a long string", 10)
	if got != "hello worl..." {
		t.Errorf("safeTruncate = %q, want %q", got, "hello worl...")
	}
}

func TestSafeTruncate_Exact(t *testing.T) {
	got := safeTruncate("hello", 5)
	if got != "hello" {
		t.Errorf("safeTruncate = %q, want %q", got, "hello")
	}
}

func TestSafeTruncate_Empty(t *testing.T) {
	got := safeTruncate("", 5)
	if got != "" {
		t.Errorf("safeTruncate = %q, want empty string", got)
	}
}

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
	"fmt"
	"sort"
	"time"
)

// Bill contains the payment information to be included in the 402 Payment-Needed header.
type Bill struct {
	OutTradeNo  string  // 商户订单号
	Amount      float64 // 支付金额 (元)
	Currency    string  // 货币类型, default CNY
	ResourceID  string  // 资源标识
	GoodsName   string  // 商品名称
	PayBefore   string  // 支付截止时间 (ISO8601)
}

// PaymentNeededProtocol is the protocol layer of the Payment-Needed payload.
type paymentNeededProtocol struct {
	OutTradeNo      string `json:"out_trade_no"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	ResourceID      string `json:"resource_id"`
	PayBefore       string `json:"pay_before"`
	SellerSignature string `json:"seller_signature"`
	SellerSignType  string `json:"seller_sign_type"`
	SellerUniqueID  string `json:"seller_unique_id"`
}

type paymentNeededMethod struct {
	SellerName         string `json:"seller_name"`
	SellerID           string `json:"seller_id"`
	SellerAppID        string `json:"seller_app_id"`
	GoodsName          string `json:"goods_name"`
	SellerUniqueIDKey  string `json:"seller_unique_id_key"`
	ServiceID          string `json:"service_id"`
}

type paymentNeeded struct {
	Protocol paymentNeededProtocol `json:"protocol"`
	Method   paymentNeededMethod   `json:"method"`
}

// BuildPaymentNeeded constructs the Base64URL-encoded Payment-Needed header value.
func BuildPaymentNeeded(cfg *Config, bill *Bill) (string, error) {
	// RSA2 sign the key parameters (dictionary order)
	signParams := map[string]string{
		"amount":       fmt.Sprintf("%.2f", bill.Amount),
		"currency":     bill.Currency,
		"goods_name":   bill.GoodsName,
		"out_trade_no": bill.OutTradeNo,
		"pay_before":   bill.PayBefore,
		"resource_id":  bill.ResourceID,
		"seller_id":    cfg.SellerID,
		"service_id":   cfg.ServiceID,
	}
	signStr := buildSignString(signParams)

	signature, err := rsaSign(cfg.PrivateKey, signStr)
	if err != nil {
		return "", fmt.Errorf("rsa sign: %w", err)
	}

	payload := paymentNeeded{
		Protocol: paymentNeededProtocol{
			OutTradeNo:      bill.OutTradeNo,
			Amount:          fmt.Sprintf("%.2f", bill.Amount),
			Currency:        bill.Currency,
			ResourceID:      bill.ResourceID,
			PayBefore:       bill.PayBefore,
			SellerSignature: signature,
			SellerSignType:  "RSA2",
			SellerUniqueID:  cfg.SellerID,
		},
		Method: paymentNeededMethod{
			SellerName:         cfg.SellerName,
			SellerID:           cfg.SellerID,
			SellerAppID:        cfg.AppID,
			GoodsName:          bill.GoodsName,
			SellerUniqueIDKey:  "seller_id",
			ServiceID:          cfg.ServiceID,
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(jsonBytes), nil
}

// NewBill creates a bill with default values filled.
func NewBill(outTradeNo string, amount float64, goodsName string, resourceID string) *Bill {
	now := time.Now()
	return &Bill{
		OutTradeNo: outTradeNo,
		Amount:     amount,
		Currency:   "CNY",
		ResourceID: resourceID,
		GoodsName:  goodsName,
		PayBefore:  now.Add(30 * time.Minute).Format(time.RFC3339),
	}
}

func buildSignString(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := ""
	for i, k := range keys {
		if i > 0 {
			result += "&"
		}
		result += k + "=" + params[k]
	}
	return result
}

func rsaSign(privateKeyPEM, content string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("not an RSA private key")
	}

	hashed := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

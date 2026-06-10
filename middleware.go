package aipay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// proofCache prevents replay of the same payment proof within the validity window.
// Key: trade_no, Value: expiry time (30 minutes after first use).
var proofCache sync.Map

// fulfillmentSem bounds concurrent async fulfillment goroutines to prevent unbounded goroutine growth under extreme load.
var fulfillmentSem = make(chan struct{}, 100)

// Middleware returns a Gin handler that enforces AI Pay (402 protocol).
//
// Every request to the protected route is checked:
//   - No Payment-Proof header → 402 + Payment-Needed bill
//   - Has Payment-Proof header → verify with Alipay → allow or deny
//
// After successfully returning data, fulfillment is confirmed asynchronously.
func Middleware(cfg *Config, pricePerCall float64, goodsName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for Payment-Proof header
		proofHeader := c.GetHeader("Payment-Proof")
		if proofHeader == "" {
			// No proof → return 402
			send402(c, cfg, pricePerCall, goodsName)
			return
		}

		// Parse Payment-Proof (base64 encoded JSON)
		log.Printf("[aipay] header_len=%d raw(first 200): %s", len(proofHeader), safeTruncate(proofHeader, 200))
		proof, err := parsePaymentProof(proofHeader)
		if err != nil {
			log.Printf("[aipay] WARN: invalid Payment-Proof header: %v (raw: %s)", err, safeTruncate(proofHeader, 500))
			send402(c, cfg, pricePerCall, goodsName)
			return
		}

		// Verify with Alipay
		verifyResult, err := VerifyPaymentProof(cfg, *proof)
		if err != nil {
			log.Printf("[aipay] WARN: payment verify failed: %v", err)
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":   "Payment Required",
				"message": fmt.Sprintf("Payment verification failed: %v", err),
			})
			c.Abort()
			return
		}

		// Store trade_no for fulfillment after response
		c.Set("aipay_trade_no", verifyResult.TradeNo)

		// Dedup: same trade_no cannot be replayed within 30-minute window
		expiry := time.Now().Add(30 * time.Minute)
		if _, loaded := proofCache.LoadOrStore(verifyResult.TradeNo, expiry); loaded {
			log.Printf("[aipay] WARN: duplicate trade_no=%s rejected", verifyResult.TradeNo)
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":   "Payment Required",
				"message": "Payment proof already used",
			})
			c.Abort()
			return
		}

		log.Printf("[aipay] payment verified trade_no=%s amount=%s", verifyResult.TradeNo, verifyResult.Amount)

		c.Next()

		// After handler returns, send fulfillment confirmation (async) with 30s timeout
		// Semaphore bounds concurrent goroutines to prevent unbounded growth under extreme load.
		fulfillmentSem <- struct{}{}
		go func(tradeNo string) {
			defer func() { <-fulfillmentSem }()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- ConfirmFulfillment(cfg, tradeNo)
			}()

			select {
			case err := <-done:
				if err != nil {
					log.Printf("[aipay] ERROR: fulfillment failed trade_no=%s: %v", tradeNo, err)
				} else {
					log.Printf("[aipay] fulfillment confirmed trade_no=%s", tradeNo)
				}
			case <-ctx.Done():
				log.Printf("[aipay] ERROR: fulfillment timeout trade_no=%s", tradeNo)
			}
		}(verifyResult.TradeNo)
	}
}

// send402 constructs and returns a 402 Payment Required response with Payment-Needed header.
func send402(c *gin.Context, cfg *Config, pricePerCall float64, goodsName string) {
	tradeNo := fmt.Sprintf("ORDER_%d_%s", time.Now().UnixMilli(), uuid.New().String()[:6])
	resourceID := c.Request.URL.Path

	bill := NewBill(tradeNo, pricePerCall, goodsName, resourceID)

	paymentNeeded, err := BuildPaymentNeeded(cfg, bill)
	if err != nil {
		log.Printf("[aipay] ERROR: failed to build Payment-Needed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate payment bill"})
		c.Abort()
		return
	}

	c.Header("Payment-Needed", paymentNeeded)
	c.JSON(http.StatusPaymentRequired, gin.H{
		"error":       "Payment Needed",
		"message":     fmt.Sprintf("This resource requires payment of %.2f CNY", pricePerCall),
		"resource_id": resourceID,
		"out_trade_no": tradeNo,
	})
	c.Abort()
}

// parsePaymentProof decodes the base64 Payment-Proof header into a PaymentProof struct.
func parsePaymentProof(header string) (*PaymentProof, error) {
	decoded, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		// alipay-bot sends unpadded base64; try RawStdEncoding
		decoded, err = base64.RawStdEncoding.DecodeString(header)
		if err != nil {
			// Try URL-safe base64 (with and without padding)
			decoded, err = base64.URLEncoding.DecodeString(header)
			if err != nil {
				decoded, err = base64.RawURLEncoding.DecodeString(header)
				if err != nil {
					// Try raw string (might be plain JSON)
					decoded = []byte(header)
				}
			}
		}
	}
	// The Payment-Proof might be wrapped
	var wrapper struct {
		Protocol PaymentProof `json:"protocol"`
		Method   struct {
			ClientSession string `json:"client_session"`
		} `json:"method"`
	}
	if err := json.Unmarshal(decoded, &wrapper); err == nil && wrapper.Protocol.TradeNo != "" {
		result := &wrapper.Protocol
		if result.ClientSession == "" {
			result.ClientSession = wrapper.Method.ClientSession
		}
		return result, nil
	}

	// Try direct unmarshal
	var proof PaymentProof
	if err := json.Unmarshal(decoded, &proof); err != nil {
		// Not valid JSON — try simple "trade_no:proof" colon-separated format
		parts := strings.SplitN(strings.TrimSpace(header), ":", 3)
		if len(parts) >= 2 {
			return &PaymentProof{
				TradeNo:      parts[0],
				PaymentProof: parts[1],
			}, nil
		}
		return nil, fmt.Errorf("unmarshal Payment-Proof: %w", err)
	}

	if proof.TradeNo == "" || proof.PaymentProof == "" {
		// Valid JSON but missing fields — try simple format as fallback
		parts := strings.SplitN(strings.TrimSpace(header), ":", 3)
		if len(parts) >= 2 {
			return &PaymentProof{
				TradeNo:      parts[0],
				PaymentProof: parts[1],
			}, nil
		}
		return nil, fmt.Errorf("Payment-Proof missing required fields")
	}

	return &proof, nil
}

func safeTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

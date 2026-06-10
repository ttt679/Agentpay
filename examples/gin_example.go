// example: minimal Gin server with AI Pay 402 protection.
//
// Usage:
//
//	export AIPAY_APP_ID="..."
//	export AIPAY_PRIVATE_KEY_FILE="/path/to/private_key.pem"
//	export AIPAY_PUBLIC_KEY_FILE="/path/to/alipay_public_key.pem"
//	export AIPAY_SELLER_ID="2088..."
//	export AIPAY_SERVICE_ID="..."
//	export AIPAY_SELLER_NAME="my-store"
//
//	go run examples/gin_example.go
//
// Then:
//
//	# GET or POST without Payment-Proof → 402
//	curl -s -D - http://localhost:8080/paid/echo
//
//	# After paying via alipay-bot, retry with Payment-Proof header
//	curl http://localhost:8080/paid/echo \
//	  -H 'Payment-Proof: ...' \
//	  -d '{"msg":"hello"}'
package main

import (
	"io"
	"log"
	"net/http"

	aipay "github.com/ttt679/Agentpay"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := aipay.LoadConfig()
	if !cfg.IsValid() {
		log.Fatal("AI Pay config not set. Please configure AIPAY_APP_ID, AIPAY_PRIVATE_KEY, AIPAY_PUBLIC_KEY, AIPAY_SELLER_ID, AIPAY_SERVICE_ID")
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Serve static files (HTML demo)
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// Protected route: 0.05 CNY per call, accepts any HTTP method
	aipayMid := aipay.Middleware(cfg, 0.05, "Echo Service")
	r.GET("/paid/echo", aipayMid, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Payment verified, resource delivered.", "price": "0.05 CNY"})
	})
	r.POST("/paid/echo", aipayMid, func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"echo": string(body), "message": "Payment verified, resource delivered."})
	})

	// Free route for comparison
	r.POST("/free/echo", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"echo": string(body)})
	})

	log.Println("listening on :8080")
	r.Run(":8080")
}
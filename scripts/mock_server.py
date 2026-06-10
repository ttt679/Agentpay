#!/usr/bin/env python3
"""
AgentPay Mock Server — 模拟代理行为，用于无支付宝凭证时的本地联调

模拟 agentpay-proxy 的 402 协议：
  - 无 Payment-Proof → 返回 402 + Payment-Needed 头
  - 有 Payment-Proof → 模拟验证 → 返回 200

用法:
    python3 scripts/mock_server.py
    # 监听 http://localhost:8080
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import base64
import time
import sys
import io

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')


class MockHandler(BaseHTTPRequestHandler):
    """模拟 agentpay-proxy 的 402 协议行为"""

    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8') if content_length > 0 else '{}'

        payment_proof = self.headers.get('Payment-Proof')

        if payment_proof:
            try:
                decoded = json.loads(base64.b64decode(payment_proof).decode('utf-8'))
                trade_no = decoded.get('trade_no', 'unknown')

                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()

                response = {
                    "echo": json.loads(body) if body else {},
                    "message": "Payment verified, resource delivered.",
                    "trade_no": trade_no,
                    "timestamp": int(time.time()),
                }
                self.wfile.write(json.dumps(response, indent=2, ensure_ascii=False).encode('utf-8'))
                print(f"[200] Payment verified, trade_no={trade_no}")

            except Exception as e:
                self.send_response(400)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"error": "Invalid payment proof"}).encode('utf-8'))
                print(f"[400] Invalid payment proof: {e}")
        else:
            # 返回 402 Payment Required（与 agentpay-proxy 协议一致）
            self.send_response(402)
            self.send_header('Content-Type', 'application/json')
            self.send_header('X-Mock-Payment-Needed', 'true')
            self.end_headers()

            response = {
                "error": "Payment Required",
                "message": "This resource requires payment. Add a Payment-Proof header to proceed.",
            }
            self.wfile.write(json.dumps(response, indent=2, ensure_ascii=False).encode('utf-8'))
            print(f"[402] Payment required for {self.path}")

        print(f"   Body: {body[:100]}...")
        print()

    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok", "mode": "mock"}).encode('utf-8'))
            return

        if self.path == '/' or self.path == '/index.html':
            try:
                with open('examples/static/index.html', 'rb') as f:
                    content = f.read()
                    self.send_response(200)
                    self.send_header('Content-Type', 'text/html; charset=utf-8')
                    self.send_header('Content-Length', len(content))
                    self.end_headers()
                    self.wfile.write(content)
                    print("[200] Served index.html")
            except FileNotFoundError:
                self.send_response(404)
                self.end_headers()
        elif self.path.startswith('/static/'):
            try:
                filepath = self.path.lstrip('/')
                with open(filepath, 'rb') as f:
                    content = f.read()
                    self.send_response(200)
                    if filepath.endswith('.html'):
                        self.send_header('Content-Type', 'text/html; charset=utf-8')
                    elif filepath.endswith('.css'):
                        self.send_header('Content-Type', 'text/css')
                    elif filepath.endswith('.js'):
                        self.send_header('Content-Type', 'application/javascript')
                    self.end_headers()
                    self.wfile.write(content)
                    print(f"[200] Served {filepath}")
            except FileNotFoundError:
                self.send_response(404)
                self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass  # suppress default logging


def run_server(port=8080):
    server_address = ('', port)
    httpd = HTTPServer(server_address, MockHandler)

    print("=" * 60)
    print("AgentPay Mock Server Started")
    print(f"URL: http://localhost:{port}")
    print("=" * 60)
    print()
    print("Test Flow:")
    print("  1. curl -X POST http://localhost:8080/chat -d '{}'")
    print("     → HTTP 402 Payment Required")
    print("  2. curl -X POST http://localhost:8080/chat -H 'Payment-Proof: ...'")
    print("     → HTTP 200 (mock)")
    print()
    print("Press Ctrl+C to stop")
    print("=" * 60)
    print()

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nServer stopped.")
        httpd.shutdown()


if __name__ == '__main__':
    run_server(8080)

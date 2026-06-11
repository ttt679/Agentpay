#!/usr/bin/env python3
"""
AgentPay Mock Server — 模拟 agentpay-verifier 行为，402 协议与 alipay-bot 对齐

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
import uuid
import hashlib
from datetime import datetime, timedelta, timezone

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

TZ_CST = timezone(timedelta(hours=8))


def build_payment_needed(trade_no, resource_id, price=0.05):
    """构造与 billing.go BuildPaymentNeeded() 完全一致的 Payment-Needed 头"""
    now = datetime.now(TZ_CST)
    pay_before = now + timedelta(minutes=30)

    mock_sign = base64.b64encode(
        hashlib.sha256(f"MOCK_SIGN_{trade_no}".encode()).digest()
    ).decode()

    payload = {
        "protocol": {
            "out_trade_no": trade_no,
            "amount": f"{price:.2f}",
            "currency": "CNY",
            "resource_id": resource_id,
            "pay_before": pay_before.isoformat(),
            "seller_signature": mock_sign,
            "seller_sign_type": "RSA2",
            "seller_unique_id": "2088123456789000",
        },
        "method": {
            "seller_name": "MockSeller",
            "seller_id": "2088123456789000",
            "seller_app_id": "2021000000000000",
            "goods_name": "AI智能体服务",
            "seller_unique_id_key": "seller_id",
            "service_id": "MOCK_SERVICE",
        },
    }

    # base64url without padding — matches billing.go line 99
    return base64.urlsafe_b64encode(
        json.dumps(payload, ensure_ascii=False).encode()
    ).rstrip(b"=").decode()


def parse_payment_proof(header):
    """解析 Payment-Proof 头，兼容 alipay-bot 的 protocol/method 双层 + 多种编码"""
    for decoder in [
        base64.b64decode,                # standard
        lambda s: base64.b64decode(s + "=="),  # unpadded standard
        base64.urlsafe_b64decode,        # url-safe
        lambda s: base64.urlsafe_b64decode(s + "=="),  # unpadded url-safe
    ]:
        try:
            decoded = decoder(header).decode('utf-8')
            data = json.loads(decoded)

            # protocol/method 双层包装
            proto = data.get("protocol", {})
            if isinstance(proto, dict) and proto.get("trade_no"):
                return proto.get("trade_no"), proto.get("payment_proof", "")

            # 单层
            if isinstance(data, dict) and data.get("trade_no"):
                return data.get("trade_no"), data.get("payment_proof", "")

        except Exception:
            continue

    # 兜底：简单 colon 分隔
    parts = header.split(":", 2)
    if len(parts) >= 2:
        return parts[0], parts[1]

    return None, None


class MockHandler(BaseHTTPRequestHandler):
    """模拟 agentpay-verifier 的 402 协议行为，与 alipay-bot 对齐"""

    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8') if content_length > 0 else '{}'

        payment_proof = self.headers.get('Payment-Proof')

        if payment_proof:
            trade_no, proof = parse_payment_proof(payment_proof)
            if trade_no:
                self.send_response(200)
                self.send_header('Content-Type', 'application/json; charset=utf-8')
                self.end_headers()

                response = {
                    "status": "verified",
                    "user_message": body,
                    "trade_no": trade_no,
                }
                self.wfile.write(json.dumps(response, ensure_ascii=False).encode('utf-8'))
                print(f"[200] Payment verified, trade_no={trade_no}")
            else:
                self.send_response(400)
                self.send_header('Content-Type', 'application/json; charset=utf-8')
                self.end_headers()
                self.wfile.write(json.dumps({"error": "Invalid payment proof format"}, ensure_ascii=False).encode('utf-8'))
                print(f"[400] Invalid payment proof format")
        else:
            # 返回 402，Payment-Needed 与 billing.go 完全对齐
            trade_no = f"MOCK_{int(time.time() * 1000)}_{uuid.uuid4().hex[:6]}"
            resource_id = self.path
            payment_needed = build_payment_needed(trade_no, resource_id)

            self.send_response(402)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Payment-Needed', payment_needed)
            self.end_headers()

            response = {
                "error": "Payment Needed",
                "message": f"This resource requires payment of 0.05 CNY (Mock)",
                "resource_id": resource_id,
                "out_trade_no": trade_no,
            }
            self.wfile.write(json.dumps(response, ensure_ascii=False).encode('utf-8'))
            print(f"[402] Payment required for {self.path}, trade_no={trade_no}")

        print(f"   Body: {body[:100]}")
        print()

    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok", "mode": "mock"}).encode())
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

    # 让 verifier 的 r.Any("/*path") 也能被 mock 处理
    do_PUT = do_POST

    def log_message(self, format, *args):
        pass


def run_server(port=8080):
    server_address = ('', port)
    httpd = HTTPServer(server_address, MockHandler)

    print("=" * 60)
    print("AgentPay Mock Server Started")
    print(f"URL: http://localhost:{port}")
    print("Format: protocol/method double-layer (alipay-bot compatible)")
    print("=" * 60)
    print()
    print("Test Flow:")
    print('  1. curl -X POST http://localhost:8080/chat -H "Content-Type: application/json" -d \'{"message":"hello"}\'')
    print("     -> HTTP 402 Payment Required (protocol+method Payment-Needed)")
    print('  2. curl -X POST http://localhost:8080/chat -H "Content-Type: application/json" -H "Payment-Proof: ..." -d \'{"message":"hello"}\'')
    print("     -> HTTP 200 (verified)")
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

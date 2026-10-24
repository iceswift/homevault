"""Synthetic recovery fixture, not a production application or authentication system."""
import hmac
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import sqlite3


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        if self.path == '/health':
            with sqlite3.connect('file:/data/db/main.sqlite?mode=ro', uri=True) as db:
                row = db.execute('SELECT value FROM proof').fetchone()
            status, payload = (200, b'{"status":"ok"}') if row == ('verified',) else (503, b'{}')
        elif self.path == '/media':
            status, payload = 200, Path('/data/app/proof.txt').read_bytes()
        else:
            status, payload = 404, b'{}'
        self.send_response(status)
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self):
        data = self.rfile.read(min(int(self.headers.get('Content-Length', '0')), 1024))
        valid = self.path == '/login' and hmac.compare_digest(data, b'fixture-only')
        self.send_response(200 if valid else 401)
        self.end_headers()
        self.wfile.write(json.dumps({'authenticated': valid}).encode())


HTTPServer(('127.0.0.1', 8000), Handler).serve_forever()

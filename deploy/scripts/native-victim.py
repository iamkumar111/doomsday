#!/usr/bin/env python3
"""Minimal HTTP/1.1 lab victim for native bench (no Docker)."""
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
import os

PORT = int(os.environ.get("VICTIM_PORT", "8443"))

class Handler(SimpleHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

if __name__ == "__main__":
    srv = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    print(f"victim listening on http://127.0.0.1:{PORT}")
    srv.serve_forever()
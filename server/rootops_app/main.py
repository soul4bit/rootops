from __future__ import annotations

from http.server import ThreadingHTTPServer

from .config import DB_PATH, HOST, PORT
from .database import init_db
from .handlers import RootOPSHandler


def main() -> None:
    init_db()
    httpd = ThreadingHTTPServer((HOST, PORT), RootOPSHandler)
    print(f"RootOPS auth server: http://{HOST}:{PORT}")
    print(f"SQLite database: {DB_PATH}")
    httpd.serve_forever()


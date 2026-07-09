from __future__ import annotations

import os
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parents[2]
DATA_DIR = Path(os.getenv("ROOTOPS_DATA_DIR", PROJECT_ROOT / "data"))
DB_PATH = Path(os.getenv("ROOTOPS_DB", DATA_DIR / "rootops.sqlite3"))

HOST = os.getenv("ROOTOPS_HOST", "127.0.0.1")
PORT = int(os.getenv("ROOTOPS_PORT", "8080"))
COOKIE_SECURE = os.getenv("ROOTOPS_COOKIE_SECURE", "0") == "1"
SESSION_COOKIE = "rootops_session"
SESSION_TTL_SECONDS = 7 * 24 * 60 * 60
PBKDF2_ITERATIONS = int(os.getenv("ROOTOPS_PBKDF2_ITERATIONS", "600000"))
MAX_BODY_BYTES = 16 * 1024

CSP = (
    "default-src 'self'; "
    "img-src 'self' data:; "
    "style-src 'self' 'unsafe-inline'; "
    "script-src 'self'; "
    "connect-src 'self'; "
    "form-action 'self'; "
    "base-uri 'self'; "
    "frame-ancestors 'none'"
)

BLOCKED_STATIC_PARTS = {".git", ".github", "server", "data", "tmp", "__pycache__"}


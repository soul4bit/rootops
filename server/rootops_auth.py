#!/usr/bin/env python3
# -*- coding: utf-8 -*-
from __future__ import annotations

import base64
import hashlib
import hmac
import html
import json
import mimetypes
import os
import re
import secrets
import sqlite3
import time
from http import cookies
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

PROJECT_ROOT = Path(__file__).resolve().parents[1]
DATA_DIR = Path(os.getenv("ROOTOPS_DATA_DIR", PROJECT_ROOT / "data"))
DB_PATH = Path(os.getenv("ROOTOPS_DB", DATA_DIR / "rootops.sqlite3"))

HOST = os.getenv("ROOTOPS_HOST", "127.0.0.1")
PORT = int(os.getenv("ROOTOPS_PORT", "8080"))
COOKIE_SECURE = os.getenv("ROOTOPS_COOKIE_SECURE", "0") == "1"
SESSION_COOKIE = "rootops_session"
SESSION_TTL_SECONDS = 7 * 24 * 60 * 60
PBKDF2_ITERATIONS = int(os.getenv("ROOTOPS_PBKDF2_ITERATIONS", "600000"))
MAX_BODY_BYTES = 16 * 1024

EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")
RATE_BUCKETS: dict[str, list[float]] = {}

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


def now_ts() -> int:
    return int(time.time())


def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode("ascii")


def unb64(value: str) -> bytes:
    return base64.urlsafe_b64decode(value.encode("ascii"))


def hash_password(password: str) -> str:
    salt = secrets.token_bytes(16)
    digest = hashlib.pbkdf2_hmac(
        "sha256",
        password.encode("utf-8"),
        salt,
        PBKDF2_ITERATIONS,
        dklen=32,
    )
    return f"pbkdf2_sha256${PBKDF2_ITERATIONS}${b64(salt)}${b64(digest)}"


def verify_password(password: str, stored_hash: str) -> bool:
    try:
      algorithm, iterations, salt, expected = stored_hash.split("$", 3)
      if algorithm != "pbkdf2_sha256":
          return False

      digest = hashlib.pbkdf2_hmac(
          "sha256",
          password.encode("utf-8"),
          unb64(salt),
          int(iterations),
          dklen=32,
      )
      return hmac.compare_digest(b64(digest), expected)
    except (ValueError, TypeError):
      return False


def session_hash(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def cookie_header(value: str, max_age: int = SESSION_TTL_SECONDS) -> str:
    jar = cookies.SimpleCookie()
    jar[SESSION_COOKIE] = value
    jar[SESSION_COOKIE]["path"] = "/"
    jar[SESSION_COOKIE]["max-age"] = str(max_age)
    jar[SESSION_COOKIE]["httponly"] = True
    jar[SESSION_COOKIE]["samesite"] = "Lax"
    if COOKIE_SECURE:
        jar[SESSION_COOKIE]["secure"] = True
    return jar.output(header="").strip()


def expired_cookie_header() -> str:
    return cookie_header("", max_age=0)


def db() -> sqlite3.Connection:
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    connection = sqlite3.connect(DB_PATH)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA foreign_keys = ON")
    return connection


def init_db() -> None:
    with db() as connection:
        connection.executescript(
            """
            CREATE TABLE IF NOT EXISTS users (
              id INTEGER PRIMARY KEY AUTOINCREMENT,
              email TEXT NOT NULL UNIQUE COLLATE NOCASE,
              name TEXT NOT NULL,
              password_hash TEXT NOT NULL,
              created_at INTEGER NOT NULL
            );

            CREATE TABLE IF NOT EXISTS sessions (
              id_hash TEXT PRIMARY KEY,
              user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
              csrf_token TEXT NOT NULL,
              created_at INTEGER NOT NULL,
              last_seen INTEGER NOT NULL,
              expires_at INTEGER NOT NULL
            );

            CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
            CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
            """
        )
        connection.execute("DELETE FROM sessions WHERE expires_at <= ?", (now_ts(),))


def is_rate_limited(key: str, limit: int = 8, window_seconds: int = 300) -> bool:
    current = time.time()
    bucket = [stamp for stamp in RATE_BUCKETS.get(key, []) if current - stamp < window_seconds]
    if len(bucket) >= limit:
        RATE_BUCKETS[key] = bucket
        return True

    bucket.append(current)
    RATE_BUCKETS[key] = bucket
    return False


def validate_registration(name: str, email: str, password: str) -> str | None:
    if len(name) < 2 or len(name) > 80:
        return "Имя должно быть от 2 до 80 символов."
    if not EMAIL_RE.match(email):
        return "Введите корректный email."
    if len(password) < 12:
        return "Пароль должен быть не короче 12 символов."
    if email.split("@", 1)[0].lower() in password.lower():
        return "Пароль не должен содержать часть email."
    return None


class RootOPSHandler(BaseHTTPRequestHandler):
    server_version = "RootOPSAuth/0.1"

    def log_message(self, fmt: str, *args: object) -> None:
        print(f"{self.address_string()} - {fmt % args}")

    def security_headers(self) -> None:
        self.send_header("Content-Security-Policy", CSP)
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Referrer-Policy", "same-origin")
        self.send_header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        self.send_header("Cross-Origin-Opener-Policy", "same-origin")

    def send_json(self, status: int, payload: dict, set_cookie: str | None = None) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.security_headers()
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        if set_cookie:
            self.send_header("Set-Cookie", set_cookie)
        self.end_headers()
        self.wfile.write(body)

    def redirect(self, location: str, set_cookie: str | None = None, status: int = 303) -> None:
        self.send_response(status)
        self.security_headers()
        self.send_header("Location", location)
        self.send_header("Cache-Control", "no-store")
        if set_cookie:
            self.send_header("Set-Cookie", set_cookie)
        self.end_headers()

    def not_found(self) -> None:
        self.send_response(404)
        self.security_headers()
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write("Not found".encode("utf-8"))

    def parse_cookie_token(self) -> str | None:
        raw_cookie = self.headers.get("Cookie", "")
        jar = cookies.SimpleCookie()
        jar.load(raw_cookie)
        morsel = jar.get(SESSION_COOKIE)
        if not morsel:
            return None
        return morsel.value

    def create_session(self, user_id: int | None = None, old_token: str | None = None) -> tuple[str, str]:
        token = secrets.token_urlsafe(48)
        csrf_token = secrets.token_urlsafe(32)
        current = now_ts()
        expires_at = current + SESSION_TTL_SECONDS

        with db() as connection:
            if old_token:
                connection.execute("DELETE FROM sessions WHERE id_hash = ?", (session_hash(old_token),))
            connection.execute(
                """
                INSERT INTO sessions (id_hash, user_id, csrf_token, created_at, last_seen, expires_at)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (session_hash(token), user_id, csrf_token, current, current, expires_at),
            )

        return token, csrf_token

    def current_session(self, create: bool = False) -> tuple[str, sqlite3.Row] | None:
        token = self.parse_cookie_token()
        current = now_ts()

        if token:
            with db() as connection:
                row = connection.execute(
                    "SELECT * FROM sessions WHERE id_hash = ? AND expires_at > ?",
                    (session_hash(token), current),
                ).fetchone()

                if row:
                    connection.execute(
                        "UPDATE sessions SET last_seen = ? WHERE id_hash = ?",
                        (current, row["id_hash"]),
                    )
                    return token, row

        if not create:
            return None

        token, _csrf = self.create_session()
        with db() as connection:
            row = connection.execute(
                "SELECT * FROM sessions WHERE id_hash = ?",
                (session_hash(token),),
            ).fetchone()
        return token, row

    def require_csrf(self, payload: dict | None = None) -> tuple[str, sqlite3.Row] | None:
        session = self.current_session(create=False)
        supplied = self.headers.get("X-CSRF-Token") or (payload or {}).get("csrfToken")

        if not session or not supplied:
            self.send_json(403, {"error": "CSRF-токен отсутствует."})
            return None

        _token, row = session
        if not hmac.compare_digest(str(supplied), row["csrf_token"]):
            self.send_json(403, {"error": "CSRF-токен недействителен."})
            return None

        return session

    def read_json(self) -> dict | None:
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.send_json(400, {"error": "Некорректный размер запроса."})
            return None

        if length > MAX_BODY_BYTES:
            self.send_json(413, {"error": "Слишком большой запрос."})
            return None

        try:
            raw = self.rfile.read(length) if length else b"{}"
            return json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            self.send_json(400, {"error": "Некорректный JSON."})
            return None

    def read_form(self) -> dict | None:
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.redirect("/?auth=login")
            return None

        if length > MAX_BODY_BYTES:
            self.redirect("/?auth=login")
            return None

        raw = self.rfile.read(length).decode("utf-8")
        parsed = parse_qs(raw, keep_blank_values=True)
        return {key: values[0] if values else "" for key, values in parsed.items()}

    def client_key(self, action: str) -> str:
        return f"{self.client_address[0]}:{action}"

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        path = parsed.path

        if path == "/api/auth/csrf":
            self.handle_csrf()
            return
        if path == "/api/auth/me":
            self.handle_me()
            return
        if path == "/dashboard":
            self.handle_dashboard()
            return
        if path == "/login":
            self.redirect("/?auth=login", status=302)
            return
        if path == "/register":
            self.redirect("/?auth=register", status=302)
            return
        if path == "/logout":
            self.redirect("/", set_cookie=expired_cookie_header())
            return

        self.serve_static(path)

    def do_POST(self) -> None:
        parsed = urlparse(self.path)
        path = parsed.path

        if path == "/api/auth/register":
            self.handle_register()
            return
        if path == "/api/auth/login":
            self.handle_login()
            return
        if path == "/api/auth/logout":
            self.handle_logout_json()
            return
        if path == "/logout":
            self.handle_logout_form()
            return

        self.not_found()

    def handle_csrf(self) -> None:
        token, row = self.current_session(create=True)
        self.send_json(
            200,
            {"csrfToken": row["csrf_token"]},
            set_cookie=cookie_header(token),
        )

    def handle_me(self) -> None:
        session = self.current_session(create=False)
        if not session:
            self.send_json(200, {"authenticated": False})
            return

        _token, row = session
        if not row["user_id"]:
            self.send_json(200, {"authenticated": False})
            return

        with db() as connection:
            user = connection.execute(
                "SELECT id, email, name, created_at FROM users WHERE id = ?",
                (row["user_id"],),
            ).fetchone()

        if not user:
            self.send_json(200, {"authenticated": False})
            return

        self.send_json(
            200,
            {
                "authenticated": True,
                "user": {
                    "id": user["id"],
                    "email": user["email"],
                    "name": user["name"],
                },
            },
        )

    def handle_register(self) -> None:
        payload = self.read_json()
        if payload is None:
            return

        session = self.require_csrf(payload)
        if not session:
            return

        if is_rate_limited(self.client_key("register"), limit=5):
            self.send_json(429, {"error": "Слишком много попыток. Попробуйте позже."})
            return

        old_token, _session_row = session
        name = str(payload.get("name", "")).strip()
        email = str(payload.get("email", "")).strip().lower()
        password = str(payload.get("password", ""))

        validation_error = validate_registration(name, email, password)
        if validation_error:
            self.send_json(400, {"error": validation_error})
            return

        try:
            with db() as connection:
                cursor = connection.execute(
                    """
                    INSERT INTO users (email, name, password_hash, created_at)
                    VALUES (?, ?, ?, ?)
                    """,
                    (email, name, hash_password(password), now_ts()),
                )
                user_id = int(cursor.lastrowid)
        except sqlite3.IntegrityError:
            self.send_json(409, {"error": "Аккаунт с таким email уже существует."})
            return

        token, csrf_token = self.create_session(user_id=user_id, old_token=old_token)
        self.send_json(
            201,
            {"ok": True, "csrfToken": csrf_token},
            set_cookie=cookie_header(token),
        )

    def handle_login(self) -> None:
        payload = self.read_json()
        if payload is None:
            return

        session = self.require_csrf(payload)
        if not session:
            return

        if is_rate_limited(self.client_key("login"), limit=8):
            self.send_json(429, {"error": "Слишком много попыток. Попробуйте позже."})
            return

        old_token, _session_row = session
        email = str(payload.get("email", "")).strip().lower()
        password = str(payload.get("password", ""))

        with db() as connection:
            user = connection.execute(
                "SELECT * FROM users WHERE email = ?",
                (email,),
            ).fetchone()

        if not user or not verify_password(password, user["password_hash"]):
            self.send_json(401, {"error": "Неверный email или пароль."})
            return

        token, csrf_token = self.create_session(user_id=user["id"], old_token=old_token)
        self.send_json(
            200,
            {"ok": True, "csrfToken": csrf_token},
            set_cookie=cookie_header(token),
        )

    def handle_logout_json(self) -> None:
        payload = self.read_json()
        if payload is None:
            return

        session = self.require_csrf(payload)
        if not session:
            return

        token, _row = session
        with db() as connection:
            connection.execute("DELETE FROM sessions WHERE id_hash = ?", (session_hash(token),))

        self.send_json(200, {"ok": True}, set_cookie=expired_cookie_header())

    def handle_logout_form(self) -> None:
        payload = self.read_form()
        if payload is None:
            return

        session = self.require_csrf(payload)
        if not session:
            return

        token, _row = session
        with db() as connection:
            connection.execute("DELETE FROM sessions WHERE id_hash = ?", (session_hash(token),))

        self.redirect("/", set_cookie=expired_cookie_header())

    def handle_dashboard(self) -> None:
        session = self.current_session(create=False)
        if not session:
            self.redirect("/?auth=login", status=302)
            return

        _token, session_row = session
        if not session_row["user_id"]:
            self.redirect("/?auth=login", status=302)
            return

        with db() as connection:
            user = connection.execute(
                "SELECT id, email, name FROM users WHERE id = ?",
                (session_row["user_id"],),
            ).fetchone()

        if not user:
            self.redirect("/?auth=login", status=302)
            return

        body = render_dashboard(user, session_row["csrf_token"]).encode("utf-8")
        self.send_response(200)
        self.security_headers()
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def serve_static(self, request_path: str) -> None:
        relative_path = "index.html" if request_path == "/" else request_path.lstrip("/")
        requested = (PROJECT_ROOT / relative_path).resolve()

        blocked_parts = {".git", ".github", "server", "data", "tmp", "__pycache__"}
        if blocked_parts.intersection(requested.parts):
            self.not_found()
            return

        try:
            requested.relative_to(PROJECT_ROOT)
        except ValueError:
            self.not_found()
            return

        if not requested.is_file():
            self.not_found()
            return

        content_type, _encoding = mimetypes.guess_type(str(requested))
        body = requested.read_bytes()

        self.send_response(200)
        self.security_headers()
        self.send_header("Content-Type", content_type or "application/octet-stream")
        self.send_header("Cache-Control", "public, max-age=3600" if request_path.startswith("/assets/") else "no-cache")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def render_dashboard(user: sqlite3.Row, csrf_token: str) -> str:
    safe_name = html.escape(user["name"])
    safe_email = html.escape(user["email"])
    safe_csrf = html.escape(csrf_token)
    initials = html.escape(user["name"][:2].upper())

    return f"""<!doctype html>
<html lang="ru">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>RootOPS - кабинет</title>
    <link rel="icon" href="/assets/rootops-logo.svg" type="image/svg+xml" />
    <link rel="stylesheet" href="/assets/styles.css" />
  </head>
  <body class="dashboard-body">
    <header class="site-header is-scrolled app-header">
      <a class="brand" href="/">
        <img class="brand-logo" src="/assets/rootops-logo.svg" alt="" />
        <span>RootOPS</span>
      </a>
      <nav class="site-nav" aria-label="Навигация кабинета">
        <a class="is-active" href="/dashboard">Главная</a>
        <a href="/dashboard">Мой сервер</a>
        <a href="/dashboard">Лаборатории</a>
        <a href="/dashboard">Roadmap</a>
      </nav>
      <form class="logout-form" method="post" action="/logout">
        <input type="hidden" name="csrfToken" value="{safe_csrf}" />
        <button class="login-action" type="submit">Выйти</button>
      </form>
    </header>

    <main class="protected-dashboard">
      <section class="section-inner dashboard-home">
        <div class="dashboard-welcome">
          <p class="section-kicker">Личный кабинет</p>
          <h1>Добро пожаловать, {safe_name}</h1>
          <p>{safe_email}</p>
        </div>

        <div class="dashboard-preview live-dashboard" aria-label="Кабинет RootOPS">
          <aside class="preview-sidebar">
            <div class="mini-brand">
              <img class="brand-logo" src="/assets/rootops-logo.svg" alt="" />
              <strong>RootOPS</strong>
            </div>
            <span class="nav-pill is-active">Главная</span>
            <span class="nav-pill">Мой сервер</span>
            <span class="nav-pill">Лаборатории</span>
            <span class="nav-pill">Roadmap</span>
            <span class="nav-pill">Задания</span>
            <div class="student-mini">
              <span>{initials}</span>
              <strong>{safe_name}</strong>
              <small>Student</small>
            </div>
          </aside>

          <div class="preview-main">
            <div class="preview-topbar">
              <strong>Главная</strong>
              <span class="search-field">Поиск</span>
            </div>
            <div class="preview-grid">
              <article class="preview-panel server-panel">
                <div>
                  <span>Мой учебный сервер</span>
                  <strong>Онлайн</strong>
                </div>
                <dl>
                  <div><dt>OS</dt><dd>Ubuntu 24.04</dd></div>
                  <div><dt>CPU</dt><dd>1 vCPU</dd></div>
                  <div><dt>RAM</dt><dd>512 MB</dd></div>
                </dl>
                <button type="button">Открыть терминал</button>
              </article>

              <article class="preview-panel progress-panel">
                <span>Текущий прогресс</span>
                <strong>Linux и терминал</strong>
                <div class="progress-row">
                  <span>35%</span>
                  <span class="progress-bar"><span style="width: 35%"></span></span>
                </div>
              </article>

              <article class="preview-panel learning-panel">
                <span>Продолжи обучение</span>
                <strong>Работа с файлами</strong>
                <p>Основы работы с системой, файлами, правами и процессами.</p>
                <button type="button">Продолжить лабораторию</button>
              </article>

              <article class="preview-panel roadmap-panel">
                <span>Твой путь в DevOps</span>
                <ol>
                  <li class="is-current"><span>01</span>Linux</li>
                  <li><span>02</span>Git</li>
                  <li><span>03</span>Docker</li>
                  <li><span>04</span>CI/CD</li>
                  <li><span>05</span>Kubernetes</li>
                </ol>
              </article>
            </div>
          </div>
        </div>
      </section>
    </main>
  </body>
</html>"""


def main() -> None:
    init_db()
    httpd = ThreadingHTTPServer((HOST, PORT), RootOPSHandler)
    print(f"RootOPS auth server: http://{HOST}:{PORT}")
    print(f"SQLite database: {DB_PATH}")
    httpd.serve_forever()


if __name__ == "__main__":
    main()

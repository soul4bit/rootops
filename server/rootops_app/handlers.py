from __future__ import annotations

import hmac
import json
import mimetypes
import sqlite3
from http import cookies
from http.server import BaseHTTPRequestHandler
from urllib.parse import parse_qs, urlparse

from .auth import (
    authenticate_user,
    create_guest_session,
    create_session,
    create_user,
    delete_session,
    find_session,
    find_user_by_id,
    is_rate_limited,
    validate_registration,
)
from .config import BLOCKED_STATIC_PARTS, CSP, MAX_BODY_BYTES, PROJECT_ROOT, SESSION_COOKIE
from .security import cookie_header, expired_cookie_header
from .templates import render_dashboard


class RootOPSHandler(BaseHTTPRequestHandler):
    server_version = "RootOPSAuth/0.2"

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

    def send_html(self, body: str) -> None:
        encoded = body.encode("utf-8")
        self.send_response(200)
        self.security_headers()
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

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
        return morsel.value if morsel else None

    def current_session(self, create: bool = False) -> tuple[str, sqlite3.Row] | None:
        token = self.parse_cookie_token()
        row = find_session(token)
        if row and token:
            return token, row

        if not create:
            return None

        return create_guest_session()

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
        path = urlparse(self.path).path

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
        path = urlparse(self.path).path

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

        user = find_user_by_id(row["user_id"])
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
            user_id = create_user(name, email, password)
        except sqlite3.IntegrityError:
            self.send_json(409, {"error": "Аккаунт с таким email уже существует."})
            return

        token, csrf_token = create_session(user_id=user_id, old_token=old_token)
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
        user = authenticate_user(email, password)

        if not user:
            self.send_json(401, {"error": "Неверный email или пароль."})
            return

        token, csrf_token = create_session(user_id=user["id"], old_token=old_token)
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
        delete_session(token)
        self.send_json(200, {"ok": True}, set_cookie=expired_cookie_header())

    def handle_logout_form(self) -> None:
        payload = self.read_form()
        if payload is None:
            return

        session = self.require_csrf(payload)
        if not session:
            return

        token, _row = session
        delete_session(token)
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

        user = find_user_by_id(session_row["user_id"])
        if not user:
            self.redirect("/?auth=login", status=302)
            return

        self.send_html(render_dashboard(user, session_row["csrf_token"]))

    def serve_static(self, request_path: str) -> None:
        relative_path = "index.html" if request_path == "/" else request_path.lstrip("/")
        requested = (PROJECT_ROOT / relative_path).resolve()

        if BLOCKED_STATIC_PARTS.intersection(requested.parts):
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


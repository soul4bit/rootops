from __future__ import annotations

import base64
import hashlib
import hmac
import time
from http import cookies

from .config import COOKIE_SECURE, PBKDF2_ITERATIONS, SESSION_COOKIE, SESSION_TTL_SECONDS


def now_ts() -> int:
    return int(time.time())


def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode("ascii")


def unb64(value: str) -> bytes:
    return base64.urlsafe_b64decode(value.encode("ascii"))


def hash_password(password: str, salt: bytes) -> str:
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


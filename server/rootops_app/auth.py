from __future__ import annotations

import re
import secrets
import sqlite3
import time

from .config import SESSION_TTL_SECONDS
from .database import db
from .security import hash_password, now_ts, session_hash, verify_password

EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")
RATE_BUCKETS: dict[str, list[float]] = {}


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


def create_session(user_id: int | None = None, old_token: str | None = None) -> tuple[str, str]:
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


def find_session(token: str | None) -> sqlite3.Row | None:
    if not token:
        return None

    current = now_ts()
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
        return row


def create_guest_session() -> tuple[str, sqlite3.Row]:
    token, _csrf = create_session()
    with db() as connection:
        row = connection.execute(
            "SELECT * FROM sessions WHERE id_hash = ?",
            (session_hash(token),),
        ).fetchone()
    return token, row


def create_user(name: str, email: str, password: str) -> int:
    with db() as connection:
        cursor = connection.execute(
            """
            INSERT INTO users (email, name, password_hash, created_at)
            VALUES (?, ?, ?, ?)
            """,
            (email, name, hash_password(password, secrets.token_bytes(16)), now_ts()),
        )
        return int(cursor.lastrowid)


def find_user_by_email(email: str) -> sqlite3.Row | None:
    with db() as connection:
        return connection.execute(
            "SELECT * FROM users WHERE email = ?",
            (email,),
        ).fetchone()


def find_user_by_id(user_id: int) -> sqlite3.Row | None:
    with db() as connection:
        return connection.execute(
            "SELECT id, email, name, created_at FROM users WHERE id = ?",
            (user_id,),
        ).fetchone()


def authenticate_user(email: str, password: str) -> sqlite3.Row | None:
    user = find_user_by_email(email)
    if not user or not verify_password(password, user["password_hash"]):
        return None
    return user


def delete_session(token: str) -> None:
    with db() as connection:
        connection.execute("DELETE FROM sessions WHERE id_hash = ?", (session_hash(token),))


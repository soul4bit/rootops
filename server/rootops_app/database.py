from __future__ import annotations

import sqlite3

from .config import DATA_DIR, DB_PATH
from .security import now_ts


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


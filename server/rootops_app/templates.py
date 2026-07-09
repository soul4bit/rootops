from __future__ import annotations

import html
import sqlite3
from pathlib import Path

TEMPLATE_DIR = Path(__file__).resolve().parent / "templates"


def render_template(name: str, context: dict[str, str]) -> str:
    content = (TEMPLATE_DIR / name).read_text(encoding="utf-8")
    for key, value in context.items():
        content = content.replace("{{ " + key + " }}", value)
    return content


def render_dashboard(user: sqlite3.Row, csrf_token: str) -> str:
    safe_name = html.escape(user["name"])
    return render_template(
        "dashboard.html",
        {
            "csrf_token": html.escape(csrf_token),
            "user_name": safe_name,
            "user_email": html.escape(user["email"]),
            "user_initials": html.escape(user["name"][:2].upper()),
        },
    )


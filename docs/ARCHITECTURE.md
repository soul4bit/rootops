# RootOPS Architecture

## Product Direction

RootOPS is a learning platform with a personal DevOps lab in the browser.
The product needs three layers:

- public marketing pages: explain the product and convert visitors to registration;
- protected learning app: dashboard, roadmap, labs, progress and profile;
- execution layer: isolated lab environments, terminal sessions and automated checks.

## Chosen Stack

For the first working version, RootOPS stays intentionally simple:

- Frontend: server-rendered HTML plus static CSS and small vanilla JavaScript.
- Backend: Python application with clear modules and no framework dependency yet.
- Database: SQLite for local development.
- Sessions: server-side sessions stored in the database.
- Auth: email/password registration and login, PBKDF2 password hashing, HttpOnly cookie and CSRF tokens.
- Reverse proxy: Caddy in production.

This lets us ship fast while keeping the code shaped like a real product.

## Future Production Stack

When RootOPS grows past the MVP, the same boundaries should move to:

- Backend API: FastAPI or another ASGI framework.
- Database: PostgreSQL.
- Session and rate-limit storage: Redis.
- Terminal transport: WebSocket endpoint.
- Lab execution: isolated Docker containers or microVM-backed runners.
- Background work: a queue for checks, lab provisioning and cleanup.
- Observability: structured logs, metrics and audit events.

## Current Repository Layout

```text
assets/                 Public static assets.
index.html              Public landing page.
server/rootops_auth.py  Backward-compatible app entrypoint.
server/rootops_app/     Backend application modules.
docs/                   Product and architecture notes.
data/                   Local SQLite database, ignored by git.
tmp/                    Local screenshots and test files, ignored by git.
```

## Backend Modules

```text
server/rootops_app/config.py      Runtime settings and security headers.
server/rootops_app/database.py    SQLite connection and schema setup.
server/rootops_app/security.py    Password hashing, cookies and CSRF helpers.
server/rootops_app/auth.py        User/session logic and validation.
server/rootops_app/templates.py   Minimal safe template rendering.
server/rootops_app/handlers.py    HTTP routes and request handling.
server/rootops_app/main.py        Server bootstrap.
```

## Security Baseline

The current MVP includes:

- password hashes with PBKDF2-HMAC-SHA256 and per-user salt;
- session identifiers stored as SHA-256 hashes in the database;
- HttpOnly session cookie with `SameSite=Lax`;
- optional `Secure` cookie flag through `ROOTOPS_COOKIE_SECURE=1`;
- CSRF token required for mutating auth requests;
- basic in-memory rate limiting for login and registration;
- security headers, including CSP and frame protection.

## Deployment Note

The current GitHub Actions workflow deploys only the static site.
The backend is intentionally excluded from static sync until we add a production service and Caddy reverse proxy rules for:

```text
/api/*
/login
/register
/logout
/dashboard
```

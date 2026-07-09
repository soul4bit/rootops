# RootOPS Architecture

## Product Direction

RootOPS is a learning platform with a personal DevOps lab in the browser.
The product needs three layers:

- public marketing pages: explain the product and convert visitors to registration;
- protected learning app: dashboard, roadmap, labs, progress and profile;
- execution layer: isolated lab environments, terminal sessions and automated checks.

## Chosen Stack

RootOPS now uses a compact Go product monolith:

- Frontend: server-rendered HTML plus static CSS and small vanilla JavaScript.
- Backend: Go standard `net/http` application split into small internal packages.
- Database: SQLite for local development and the first MVP.
- Sessions: server-side sessions stored in the database.
- Auth: email/password registration and login, bcrypt password hashing, HttpOnly cookie and CSRF tokens.
- Reverse proxy: Caddy in production.

This keeps the project simple while giving us a better base for terminal sessions, lab runners and long-lived backend work.

## Future Production Stack

When RootOPS grows past the MVP, the same boundaries should move to:

- Database: PostgreSQL.
- Session and rate-limit storage: Redis.
- Terminal transport: WebSocket endpoint.
- Lab execution: isolated Docker containers or microVM-backed runners.
- Background work: a queue for checks, lab provisioning and cleanup.
- Observability: structured logs, metrics and audit events.

## Current Repository Layout

```text
assets/                 Public static assets.
cmd/rootops/            Go application entrypoint.
internal/auth/          Passwords, tokens, validation and rate limiting.
internal/config/        Runtime settings.
internal/storage/       SQLite schema and queries.
internal/web/           HTTP routes, cookies, CSRF and rendering.
web/templates/          Protected HTML templates.
index.html              Public landing page.
docs/                   Product and architecture notes.
data/                   Local SQLite database, ignored by git.
tmp/                    Local screenshots and test files, ignored by git.
```

## Security Baseline

The current MVP includes:

- password hashes with bcrypt;
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

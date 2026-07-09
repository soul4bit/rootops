# RootOPS

RootOPS - практическая DevOps-платформа: лендинг, учебный roadmap, лаборатории и стартовый контур защищённой авторизации.

## Стек и структура

Проект переводится на Go product monolith:

- публичная главная: `index.html`, `assets/`;
- backend: `cmd/rootops/` и `internal/`;
- защищённые шаблоны: `web/templates/`;
- локальная база: SQLite в `data/`;
- авторизация: email/password, bcrypt, серверные сессии, HttpOnly cookie и CSRF.

Архитектура и production-путь описаны в `docs/ARCHITECTURE.md`.

## Локальный запуск

Статический просмотр главной по-прежнему возможен:

```bash
python -m http.server 8080
```

Для регистрации, входа и защищённого кабинета запускай Go-сервер:

```bash
go run ./cmd/rootops
```

По умолчанию он открывает:

```text
http://127.0.0.1:8080
```

Данные хранятся в SQLite:

```text
data/rootops.sqlite3
```

Файл базы не коммитится.

## Настройки

```text
ROOTOPS_ADDR=127.0.0.1:8080
ROOTOPS_HOST=127.0.0.1
ROOTOPS_PORT=8080
ROOTOPS_DATA_DIR=./data
ROOTOPS_DB=./data/rootops.sqlite3
ROOTOPS_COOKIE_SECURE=1
ROOTOPS_PUBLIC_URL=https://rootops.su
ROOTOPS_SMTP_HOST=smtp.beget.com
ROOTOPS_SMTP_PORT=2525
ROOTOPS_SMTP_USERNAME=verification@rootops.su
ROOTOPS_SMTP_PASSWORD=<mailbox-password>
ROOTOPS_SMTP_FROM=RootOPS <verification@rootops.su>
```

Для HTTPS production-окружения включи secure-cookie:

```bash
ROOTOPS_COOKIE_SECURE=1 go run ./cmd/rootops
```

Локально можно создать `.env` рядом с `go.mod`. Файл игнорируется git, поэтому пароль от почты не попадёт в репозиторий. Шаблон лежит в `.env.example`.

## Auth MVP

Сейчас реализовано:

- регистрация и вход через `/api/auth/register` и `/api/auth/login`;
- подтверждение email через одноразовую ссылку `/verify-email`;
- bcrypt для хранения паролей;
- серверные сессии в SQLite;
- cookie `rootops_session` с `HttpOnly` и `SameSite=Lax`;
- CSRF-токен для изменяющих auth-запросов;
- базовый rate limit на регистрацию и вход;
- защищённый `/dashboard`;
- logout через POST.

## Production

Текущий GitHub Actions workflow разворачивает только статическую часть сайта в `/var/www/rootops`.
Backend-код (`cmd/`, `internal/`, `web/`, `go.mod`, `go.sum`) намеренно исключён из статического deploy, чтобы Caddy не раздавал исходники как публичные файлы.

Канонический адрес:

```text
https://rootops.su/
```

Пример базового Caddyfile для статического сайта:

```caddyfile
www.rootops.su {
	redir https://rootops.su{uri} permanent
}

rootops.su {
	root * /var/www/rootops
	encode gzip zstd
	file_server
}
```

Для production backend нужно добавить отдельный systemd-сервис Go-приложения и reverse proxy для:

```text
/api/*
/login
/register
/logout
/dashboard
```

## Автодеплой из GitHub

Workflow находится в `.github/workflows/deploy.yml`. Он запускается при каждом `push` в `main` и синхронизирует статические файлы на сервер через SSH.

Необходимые GitHub Actions secrets:

```text
DEPLOY_HOST=<host>
DEPLOY_USER=<user>
DEPLOY_PORT=22
DEPLOY_PATH=/var/www/rootops
DEPLOY_SSH_KEY=<private deploy key>
```

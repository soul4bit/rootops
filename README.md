# RootOPS

RootOPS - практическая DevOps-платформа: лендинг, учебный roadmap, лаборатории и стартовый контур защищённой авторизации.

## Локальный запуск

Статический просмотр по-прежнему возможен:

```bash
python -m http.server 8080
```

Для регистрации, входа и защищённого кабинета запускай auth-сервер:

```bash
python server/rootops_auth.py
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

## Auth MVP

Сейчас реализовано:

- регистрация и вход через `/api/auth/register` и `/api/auth/login`;
- PBKDF2-HMAC-SHA256 для хранения паролей;
- серверные сессии в SQLite;
- cookie `rootops_session` с `HttpOnly` и `SameSite=Lax`;
- CSRF-токен для изменяющих запросов;
- базовый rate limit на регистрацию и вход;
- защищённый `/dashboard`;
- logout через POST.

Для HTTPS production-окружения включи secure-cookie:

```bash
ROOTOPS_COOKIE_SECURE=1 python server/rootops_auth.py
```

## Production

Текущий GitHub Actions workflow разворачивает только статическую часть сайта в `/var/www/rootops`.
Папка `server/` намеренно исключена из статического deploy, чтобы backend-код не раздавался Caddy как публичные файлы.

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

Когда auth-сервер будет готов к production, нужно будет добавить отдельный systemd-сервис и reverse proxy для `/api/*`, `/login`, `/register`, `/logout` и `/dashboard`.

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

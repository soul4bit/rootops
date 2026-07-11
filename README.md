# RootOPS

RootOPS is a practical DevOps knowledge and lab platform.

The first version is intentionally file-first:

- articles are Markdown files in `content/articles`;
- quizzes are JSON files in `content/quizzes`;
- console practices are JSON files in `content/labs`;
- the Go app validates and previews this content.

This keeps knowledge updates easy to review, commit and publish before adding
accounts, progress tracking and isolated lab runners.

## Commands

```bash
go run ./cmd/rootops serve
go run ./cmd/rootops content list
go run ./cmd/rootops content validate
go run ./cmd/rootops content new article linux-files --title "Linux files" --module linux
go run ./cmd/rootops content new quiz linux-files
go run ./cmd/rootops content new lab linux-files
```

By default the server opens on:

```text
http://127.0.0.1:8080
```

## Deploy

Autodeploy is handled by `.github/workflows/deploy.yml`.

It builds the Go app, validates content, uploads a release to the server and
restarts the `rootops` systemd service. Server setup notes are in
`docs/DEPLOY.md`.

## Structure

```text
cmd/rootops/              Application entrypoint.
internal/cli/             Console commands.
internal/config/          Runtime configuration.
internal/content/         Articles, quizzes, labs and validation.
internal/web/             HTTP preview app.
assets/                   Public CSS and images.
content/                  Knowledge base.
docs/                     Product and architecture notes.
.github/workflows/        CI and production deploy.
```

## Product Direction

RootOPS should feel less like a course library and more like an engineering
training system:

1. read a short article;
2. answer a quick test;
3. open a console task;
4. run commands in a lab environment;
5. let RootOPS check the result.

The MVP starts with content management and preview. The next layer is a lab
runner with isolated Docker environments and command/state checks.

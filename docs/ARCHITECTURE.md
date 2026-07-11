# RootOPS Architecture

## Core Idea

RootOPS is a DevOps learning system where knowledge is maintained by the owner
and every topic can have three attached learning objects:

- article: explanation and context;
- quiz: quick knowledge check;
- lab: console-based practice with automated checks.

## Initial Architecture

The first implementation is a compact Go monolith with a file-backed content
catalog.

```text
content/articles/*.md      Markdown articles with small front matter.
content/quizzes/*.json     Questions linked by article_id.
content/labs/*.json        Console practice linked by article_id.
```

This gives us simple ownership:

- content changes are visible in git diffs;
- the admin can add material through CLI commands;
- validation can run locally and in CI;
- a database is not required before user progress exists.

## Planned Layers

1. Content layer: articles, quizzes, labs, validation.
2. Preview layer: web interface for browsing the catalog.
3. Auth layer: accounts and protected student workspace.
4. Progress layer: quiz attempts, completed labs, next step.
5. Runner layer: isolated Docker containers, browser terminal, checks.
6. Admin layer: web editor if CLI editing becomes too slow.

## Lab Runner Direction

Labs should not trust user input. A lab definition should describe the scenario,
environment and checks, while the runner controls execution:

- one temporary environment per attempt;
- shell access through a controlled terminal session;
- checks executed server-side;
- cleanup after timeout;
- no real secrets in lab environments.

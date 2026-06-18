---
sidebar_position: 90
title: Contributing
---

# Contributing

Hand is still moving quickly, so the best contributions are small, tested, and easy to review.

## Before You Start

- Read [Developer Architecture](development/architecture.md) for package boundaries before large code changes.
- Open or join a discussion for larger changes.
- Keep changes scoped to one behavior or documentation area.
- Prefer existing package boundaries, naming conventions, and test patterns.
- Avoid live provider calls in normal tests.

## Local Checks

Run the project test target from the repository root:

```bash
make test
```

For focused Go package tests, keep the same SQLite FTS settings used by the Makefile:

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./internal/gateway/...
```

For docs changes, run checks from `website/docs`:

```bash
npm run typecheck
npm run build
```

## Pull Requests

- Explain the user-facing change first.
- Mention the checks you ran.
- Include screenshots for TUI or docs UI changes when useful.
- Keep secrets, provider tokens, raw payloads, and private user content out of issues, logs, and screenshots.

## Good First Areas

- Documentation gaps.
- Focused tests for existing behavior.
- Gateway setup guidance and examples.
- TUI polish with clear before/after screenshots.

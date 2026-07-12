---
title: Testing
description: Run and write tests for Morph.
displayed_sidebar: null
---

# Testing

This page should document test practices. For an example subsystem's full test layout (unit, store, RPC, gateway,
and e2e), see [Automation System: Component Ownership](./automation-system#component-ownership).

## Content Outline

- Use `make test` for the full suite.
- Focused Go tests with `CGO_ENABLED=1` and `-tags sqlite_fts5`.
- Avoid live OAuth tests during ordinary test runs.
- Gateway fake transports.
- TUI rendering tests.

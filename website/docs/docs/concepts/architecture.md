---
title: Architecture
description: The high-level shape of Hand.
---

# Architecture

Hand is organized around a daemon-owned runtime with multiple client and gateway surfaces.

## Content Outline

- Daemon as the long-lived owner.
- CLI and TUI as clients.
- RPC as the control and response boundary.
- Agent runtime, environment, tools, state, memory, and gateway packages.
- Where persistent state lives.

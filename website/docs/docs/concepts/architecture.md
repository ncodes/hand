---
title: Architecture
description: The high-level shape of Hand.
---

# Architecture

Hand is built around a long-lived **daemon** that owns the agent runtime, with thin **clients** that connect to it over
**RPC**. The daemon holds the model clients, the agent loop, tools, memory, and state for one profile; clients like the
TUI and CLI send requests and stream results back. Everything for a profile lives in that profile's home directory.

This page describes the high-level shape and how the parts fit. For the package-level map, see
[Development Architecture](../development/architecture).

```text
┌─────────────┐   ┌─────────────┐   ┌──────────────────┐
│  TUI        │   │  CLI        │   │  Gateway clients │
│  (hand)     │   │  (hand ...) │   │  Slack/Telegram  │
└──────┬──────┘   └──────┬──────┘   └────────┬─────────┘
       │  gRPC           │  gRPC             │  HTTP
       └────────┬────────┘                   │
                ▼                            ▼
        ┌───────────────────────────────────────────┐
        │  Daemon (one per profile)                 │
        │                                           │
        │   RPC server  ◄──────  Gateway manager    │
        │       │                                   │
        │       ▼                                   │
        │   Agent runtime                           │
        │     ├─ turn loop (model calls + tools)    │
        │     ├─ environment + tools                │
        │     ├─ model clients (main/summary/rerank)│
        │     ├─ memory provider                    │
        │     └─ guardrails (input/output/PII)      │
        │       │                                   │
        │       ▼                                   │
        │   State manager  ─►  SQLite + FTS5        │
        └───────────────────────────────────────────┘
                ▼
   ~/.hand/profiles/<name>/  (config, credentials, db, traces, runtime.json)
```

## The Daemon Owns the Runtime

The daemon is the long-lived process that owns everything stateful and expensive: the configured model clients, the
agent loop, the tool registry, the memory subsystem, and the connection to persistent storage. Running it once and
keeping it warm means clients stay thin and fast.

`hand daemon` boots the runtime for the active profile. On startup it loads and validates config, builds the model
clients, opens the state store, starts the agent, optionally starts the gateway, and binds the RPC listener. If model or
embedding credentials are missing, the daemon still starts but disables the parts that need them (for example the
gateway, vector search, or memory) so you can fix configuration and retry.

The daemon watches its profile `config.yaml` and restarts the runtime when the file changes, so a valid config edit
takes effect without a manual stop. On shutdown (`Ctrl+C` / SIGTERM) it stops accepting requests, drains the gateway,
flushes memory, and closes storage cleanly.

For lifecycle details, see [Daemon and RPC](./daemon-and-rpc) and [Daemon Operations](../operations/daemon).

## CLI and TUI Are Clients

Clients do not run the agent themselves. They resolve the active profile, find its daemon, and send requests over RPC:

- The **CLI** (`hand --chat`, `hand session ...`, `hand gateway ...`) connects to the daemon and prints results. A
  one-shot chat starts a temporary daemon when none is reachable, then stops it after the response completes.
- The **TUI** (`hand`) is the interactive surface. For convenience it can start a daemon for you: when no daemon is
  running, it launches one inside the same process, waits until RPC is ready, then connects. Exiting the TUI stops that
  embedded daemon.

Because clients select the profile first and then connect, every client shares the same runtime and state for that
profile. See [Profiles and Config](../getting-started/profiles-and-config) for how the active profile is resolved.

## RPC Is the Boundary

Clients and the daemon talk over a local gRPC listener, bound by default to `127.0.0.1:50051`. The daemon writes its
address and process id to the profile's `runtime.json`, and clients read that file to locate the daemon. This is how the
profile and its running daemon stay linked.

The RPC surface is organized by concern:

- **Chat** streams a response back as it is produced: incremental text, trace events, errors, and a final done signal.
- **Sessions** create, list, switch, rename, archive, compact, and inspect conversations.
- **Models** list providers and models and select or configure them.
- **Gateway** controls the messaging gateway at runtime: status, start, stop, restart, and pairing approvals.

For the full method list, see the [RPC Reference](../reference/rpc).

## Inside the Runtime

The agent runtime is what turns a message into an answer. A request becomes a **turn**, and a turn runs an iteration
loop: build the prompt context, call the model, and if the model requests tools, execute them and feed the results back
until the model produces a final answer.

Several subsystems support that loop:

- **Environment and tools** expose capabilities to the model — filesystem, command execution, web search and extraction,
  session search, memory, and time. Each tool is gated by the profile's capability flags, so you control what the agent
  can touch. See [Tools](./tools).
- **Model clients** abstract providers (OpenRouter, OpenAI, OpenAI Codex, Anthropic, GitHub Copilot) behind common API
  shapes. The daemon builds separate clients for the main model, the summary model, and the reranker. See
  [Provider Auth](../guides/provider-auth).
- **Memory** retrieves relevant context at the start of a turn and writes new memory during compaction and shutdown, so
  the agent carries useful context across sessions. See [Memory](./memory).
- **Guardrails** scan input, loaded content, and output, and redact sensitive data, recording safety events to traces.
  See [Safety and Guardrails](./safety-and-guardrails).

## Gateways Attach to the Same Runtime

When enabled, the gateway lets external clients — Slack, Telegram, or a generic HTTP caller — reach the same agent. The
daemon hosts the gateway as an HTTP server on its own address and port (default `50052`), separate from the gRPC
listener. Inbound messages are mapped to a Hand session and answered by the same agent runtime the TUI and CLI use.

The gateway is controlled at runtime through the RPC gateway service, so you can start, stop, and manage pairings
without restarting the daemon. See [Gateways](./gateways) and [Gateway Management](../operations/gateway-management).

## Where State Lives

All persistent state for a profile lives under its home directory, `~/.hand/profiles/<name>/`:

- **Config and credentials**: `config.yaml`, `.env`, and `auth.json`.
- **Conversations and memory**: a SQLite database with full-text search (FTS5) for session messages and memory items.
- **Traces**: recorded agent activity, on disk and in the database.
- **Runtime metadata**: `runtime.json`, which records the daemon's RPC endpoint, process id, and start time.

Because each profile is self-contained, you can run several profiles on one machine without sharing state, and you can
back up or move a profile by copying its directory. See [Profiles](./profiles), [Sessions](./sessions),
[Profiles and Config](../getting-started/profiles-and-config), and
[Backups and State](../operations/backups-and-state).

## Where To Go Next

- [Daemon and RPC](./daemon-and-rpc): why Hand runs a daemon and how clients connect.
- [Sessions](./sessions): the durable conversation model.
- [Tools](./tools): how capabilities are exposed and gated.
- [Memory](./memory): how Hand remembers across sessions.
- [Gateways](./gateways): reaching the agent from external clients.
- [Development Architecture](../development/architecture): the package-level implementation map.

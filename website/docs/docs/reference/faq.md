---
title: FAQ
description: Frequently asked questions and short answers.
---

# FAQ

Short answers to recurring Hand questions. For setup walkthroughs, start with [Quickstart](../getting-started/quickstart)
and the [Learning Path](../getting-started/learning-path).

## Profiles and config

### How do I switch profiles?

```bash
hand profile use work
hand --profile work
```

Or set `HAND_PROFILE=work` for a single command. The current profile is stored in `~/.hand/state.json`. Details:
[Profiles](../concepts/profiles), [Profiles and Config](../getting-started/profiles-and-config).

### Where does config live?

Each profile has its own directory under `~/.hand/profiles/<name>/` with `config.yaml`, `.env`, `data/state.db`, and
`traces/`. See [Backups and State](../operations/backups-and-state).

### Config vs environment variables — which wins?

Hand first loads the active profile's `config.yaml`. It also preloads the profile `.env` file into the process
environment, then applies any supported `HAND_*` variables over the YAML values before defaults and normalization run.

In practice: put durable settings in `config.yaml`, use `.env` or shell env vars for local overrides and secrets, and
expect `HAND_*` values to win for the current command. See [Environment Variables](./environment-variables) and
[Config Reference](./config).

## Daemon and CLI

### Does `hand gateway stop` stop the daemon?

**No.** It stops the **gateway runtime** (HTTP/Slack/Telegram ingress) inside the running daemon. The daemon and RPC
keep serving TUI and `hand session` clients. To stop the daemon process, terminate the `hand daemon` process (Ctrl+C or
your service manager). See [Gateway Management](../operations/gateway-management) and
[CLI Reference](./cli#gateway--gateway-runtime-and-pairing).

### How do I start the daemon?

```bash
hand daemon
```

Check status with `hand daemon status`. See [Daemon Operations](../operations/daemon).

### Why does `hand doctor` fail when chat still opens?

Doctor is stricter than simply opening the TUI. It validates config, credentials, daemon metadata, and optional
subsystems so you can see everything that is not ready yet. Fix reported `[FAIL]` items or start the daemon if the
daemon group warns. See [Doctor](../operations/doctor).

### When should I use one-shot chat instead of the TUI?

Use the TUI when you want an interactive session: streaming transcript, slash commands, model/provider panels, prompt
history, cancellation, and session navigation.

Use one-shot chat when you want a single prompt from a shell script, terminal pipeline, or quick command:

```bash
hand --chat "summarize this profile"
```

Both modes talk to the daemon RPC path and can start a quiet local daemon if none is reachable. One-shot chat prints the
answer and exits; the TUI stays open as your working surface. See [CLI Reference](./cli).

## Gateways

### Which Slack mode should I use locally?

**Socket Mode** (`gateway.slack.mode: socket`) — no public HTTP URL required; uses `appToken` + `botToken`. Good for
development machines behind NAT.

**HTTP** (`http`) — Slack posts to `/gateway/slack/webhook`; requires a reachable URL and `signingSecret`.

See [Slack Gateway](../guides/gateway/slack) and [Gateway Routes](./gateway-routes).

### Which Telegram mode should I use locally?

**Polling** (`gateway.telegram.mode: polling`) — daemon pulls updates; no webhook URL needed.

**Webhook** (`webhook`) — Telegram POSTs to `/gateway/telegram/webhook`; requires registration via
`hand gateway setwebhook telegram <url>` and `webhookSecret`.

See [Telegram Gateway](../guides/gateway/telegram).

### Generic HTTP clients?

POST JSON to `/v1/respond` when the gateway is enabled. Set `gateway.authToken` when binding beyond loopback. See
[Generic HTTP Gateway](../guides/gateway/generic-http) and [Gateway Routes](./gateway-routes).

## Sessions, memory, and tools

### Where is conversation history stored?

In the profile SQLite file `data/state.db` (default backend). Sessions, messages, summaries, memory, and traces share
this store. See [Sessions](../concepts/sessions) and [Backups and State](../operations/backups-and-state).

### Why did the agent stop after many tool calls?

The turn hit **`session.maxIterations`** (default **90**). Lower it for experiments or raise it in config. On
exhaustion, Hand runs a summary fallback model call. See [Sessions](../concepts/sessions) and
[Config Reference](./config#session).

### Why don't web or memory tools appear?

Tools are gated by **capabilities** (`cap.net`, `cap.mem`) and subsystem config (web provider credentials, memory
enabled). Run `hand doctor` and check [Tools](../concepts/tools).

## API and integration

### Where is the gRPC API defined?

The protobuf package is `hand.v1`. See [RPC Reference](./rpc) for the service and message summary.

## Security

### Is RPC authenticated?

Not yet at the application layer. RPC uses plaintext gRPC; keep `rpc.address` on loopback unless the network boundary
protects it. See [Security](../operations/security) and [Daemon and RPC](../concepts/daemon-and-rpc).

### Where should I put API keys?

Prefer `hand auth login` and profile `auth.json` over committing keys to YAML. Env vars are supported for CI and local
dev. See [Provider Auth](../guides/provider-auth) and [Environment Variables](./environment-variables#secret-handling).

## Documentation map

| Topic | Page |
| --- | --- |
| Commands and flags | [CLI Reference](./cli) |
| TUI `/` commands | [Slash Commands](./slash-commands) |
| Every config key | [Config Reference](./config) |
| `HAND_*` vars | [Environment Variables](./environment-variables) |
| HTTP ingress | [Gateway Routes](./gateway-routes) |
| gRPC API | [RPC Reference](./rpc) |
| Trace event names | [Trace Events](./trace-events) |
| Troubleshooting | [Troubleshooting Guide](../guides/troubleshooting) |

## Where To Go Next

- [Learning Path](../getting-started/learning-path): guided reading by role
- [Documentation home](/): full site map
- [Contributing](../contributing): report doc gaps or submit fixes

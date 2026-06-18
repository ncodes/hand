---
title: Gateway Management
description: Start, stop, restart, and inspect gateway runtime state.
---

# Gateway Management

The messaging **gateway** runs inside the [daemon](./daemon) as a separate HTTP listener plus optional Slack and Telegram
workers. Use `hand gateway` to inspect and control that runtime **without stopping the daemon or RPC**.

This page covers runtime commands: status, start, stop, restart, and Telegram webhook registration. For enabling
gateways in config and platform setup, see the [Gateway Overview](../guides/gateway/). For the conceptual model,
see [Gateways](../concepts/gateways). For connection problems, see
[Troubleshooting](../guides/troubleshooting#gateway-issues).

## Prerequisites

`hand gateway` commands talk to the daemon over **RPC** — same as `hand session`. They do **not** start a daemon.

1. Start the daemon for the profile: `hand daemon` (or keep `hand` open).
2. Enable the gateway in config: `hand config set gateway.enabled true` (plus platform tokens as the platform guides
   describe).
3. Confirm readiness: `hand doctor` — the **gateway** group should pass or show fixable warnings.

Use `--profile <name>` to target another profile.

## Runtime vs Config

Two layers matter:

| Layer | What changes it | What it affects |
| --- | --- | --- |
| **Profile config** (`config.yaml`, `.env`) | `hand config set`, editing files | What the daemon *should* run after reload |
| **Gateway runtime** | `hand gateway start/stop/restart` | Whether components are *running right now* |

Important rules:

- `hand gateway start/stop/restart` uses the daemon's **current in-memory** gateway config — it does **not** read
  `config.yaml` from disk.
- When you change config with `hand config set`, the daemon reloads automatically (when valid) and restarts its
  services, including the gateway. See [Daemon Operations — Config reload](./daemon#config-reload).
- `hand gateway restart` is for **operational recovery** (after `state=failed`, stuck components, or once the daemon has
  already reloaded new settings). To apply new tokens or modes from disk, prefer config reload first.

Pairing management (`hand gateway pairing …`) is also RPC-only — see
[Pairing and Allowlists](../guides/gateway/pairing-and-allowlists).

## Gateway Runtime State

The gateway manager tracks one state for the whole surface (HTTP listener plus Slack/Telegram workers):

| State | Meaning |
| --- | --- |
| `disabled` | `gateway.enabled` is false — nothing to run |
| `stopped` | Enabled in config but components are not running (clean stop or not started yet) |
| `starting` | Components are coming up |
| `running` | HTTP listener and enabled platform workers are active |
| `stopping` | Shutdown in progress |
| `failed` | A component exited with an error — see `last_error` |

There is no separate "degraded" state. A partial failure surfaces as `failed` with a component name in `last_error`
(for example `slack socket: …`). Secrets in errors are redacted.

When `gateway.enabled` is true, the daemon **starts the gateway automatically** at boot (if model config is sufficient).
`hand gateway stop` halts components while the daemon and RPC keep running.

## Inspect Status

```bash
hand gateway status
```

Example output:

```text
state=running address=127.0.0.1 port=50052 telegram=polling slack=socket
```

Fields:

- **state** — one of the values in the table above
- **address** / **port** — gateway HTTP listener (`gateway.address` / `gateway.port`, default `127.0.0.1:50052`)
- **telegram** — configured mode (`polling`, `webhook`, or empty when disabled)
- **slack** — configured mode (`socket`, `http`, or empty when disabled)
- **last_error** — present when `state=failed` (quoted string)

`hand doctor` complements this with **readiness** checks (missing tokens, webhook secrets, non-loopback bind without
`gateway.authToken`) before you expose the listener.

## Start, Stop, and Restart

All three commands print the same status line as `hand gateway status` when they succeed.

### Start

```bash
hand gateway start
```

Starts gateway components using the daemon's in-memory config. If the gateway is already running, this is a no-op.

Fails with `gateway is disabled` when `gateway.enabled` is false — enable it in config first.

### Stop

```bash
hand gateway stop
```

Stops the gateway HTTP server and any running Slack socket or Telegram polling workers. **Does not stop the daemon**,
RPC, agent, or session storage. Generic HTTP, Slack, and Telegram share one runtime — stop halts all of them together.

Use this when you want to pause inbound gateway traffic without shutting down the rest of Hand.

### Restart

```bash
hand gateway restart
```

Stops then starts gateway components with the daemon's current in-memory config. Use after fixing credentials in config
(and waiting for daemon reload), or when `state=failed` and `last_error` points to a recoverable platform error.

## What Runs When the Gateway Is Up

With `gateway.enabled` true and `state=running`, the daemon may run:

| Component | When |
| --- | --- |
| **Gateway HTTP** | Always — `/health`, `POST /v1/respond`, and webhook routes when platforms use HTTP mode |
| **Slack socket** | `gateway.slack.enabled` and `gateway.slack.mode socket` |
| **Telegram polling** | `gateway.telegram.enabled` and `gateway.telegram.mode polling` |

Slack **HTTP** mode and Telegram **webhook** mode are served by the shared HTTP listener (no separate poller or socket
worker). Route details are in [Gateway Routes](../reference/gateway-routes).

Platform setup (tokens, event subscriptions, public URLs) is in [Slack](../guides/gateway/slack),
[Telegram](../guides/gateway/telegram), and [Generic HTTP](../guides/gateway/generic-http).

## Register Telegram Webhooks

Hand does not register Telegram webhooks automatically when you enable webhook mode. After the gateway is reachable,
point Telegram at your public URL:

```bash
hand gateway setwebhook telegram https://<your-host>/gateway/telegram/webhook
```

Requirements:

- `gateway.telegram.enabled` true and `gateway.telegram.mode webhook`
- `gateway.telegram.botToken` and `gateway.telegram.webhookSecret` set
- The URL must reach `/gateway/telegram/webhook` on the gateway listener

To remove the webhook:

```bash
hand gateway setwebhook telegram
```

This calls Telegram's API from your machine (not through the daemon RPC path). Slack HTTP mode uses Slack's Event
Subscriptions **Request URL** in the Slack app settings instead — see [Slack — HTTP Events API](../guides/gateway/slack#http-events-api-mode).

## Gateway Stop vs Daemon Stop

| Action | Gateway | Daemon / RPC | Sessions / storage |
| --- | --- | --- | --- |
| `hand gateway stop` | Stopped | Keeps running | Unchanged |
| Exit TUI / **Ctrl+C** on `hand daemon` | Stopped | Stopped | Closed cleanly |
| `hand gateway restart` | Restarted | Keeps running | Unchanged |

Session bindings and pairing records live in the profile database — stopping the gateway does not delete them.

## Common Operator Tasks

### Pause inbound traffic temporarily

```bash
hand gateway stop
# … work …
hand gateway start
```

### Recover from `state=failed`

1. Read `hand gateway status` — note `last_error`.
2. Run `hand doctor` and fix missing tokens or mode settings in config.
3. Wait for config reload, or fix `.env` and restart the daemon if needed.
4. Run `hand gateway restart`.

Check daemon logs (`log.level debug`) for component errors — see [Troubleshooting](../guides/troubleshooting#logging-and-debug).

### Confirm generic HTTP is listening

With `gateway.enabled` true and `state=running`:

```bash
curl -sS http://127.0.0.1:50052/health
```

Adjust host/port if you changed `gateway.address` / `gateway.port`.

## Troubleshooting

### Gateway commands fail to connect

Start the daemon first — `hand daemon`, or keep `hand` open. Gateway commands do not bootstrap a daemon. See
[Troubleshooting — Daemon and RPC](../guides/troubleshooting#daemon-and-rpc-unreachable).

### `gateway is disabled` from start/restart

Run `hand config set gateway.enabled true` and confirm the daemon reloaded. `hand gateway status` should not show
`state=disabled`.

### Config change did not affect runtime

`hand gateway restart` does not reload `config.yaml`. Edit config (or `hand config set`) and wait for daemon reload, or
restart the daemon after `.env` changes. See [Daemon Operations](./daemon#config-reload).

### Messages arrive but no reply

Gateway runtime can be `running` while turns still fail — check model auth, sender pairing, and platform-specific
behavior. See [Gateway Overview — Troubleshooting](../guides/gateway/#troubleshooting) and platform guides.

## Where To Go Next

- [Gateway Overview](../guides/gateway/): enable gateways and choose a surface.
- [Gateways](../concepts/gateways): binding, authorization, and message flow.
- [Daemon Operations](./daemon): daemon lifecycle and config reload.
- [Gateway Routes](../reference/gateway-routes): HTTP paths and auth.
- [Pairing and Allowlists](../guides/gateway/pairing-and-allowlists): `hand gateway pairing` commands.
- [Doctor](./doctor): readiness checks before going live.
- [Troubleshooting](../guides/troubleshooting): gateway and webhook issues.
- [Daemon and RPC](../concepts/daemon-and-rpc): RPC boundary and `GatewayService`.

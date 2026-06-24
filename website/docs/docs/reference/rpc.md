---
title: RPC Reference
description: Daemon gRPC service reference.
---

# RPC Reference

Morph exposes a **gRPC** API on the daemon process (default `127.0.0.1:50051`). TUI, `morph session`, `morph gateway`, and
one-shot `--chat` are RPC clients. Conceptual overview: [Daemon and RPC](../concepts/daemon-and-rpc).

**Proto source:** `internal/rpc/proto/morph.proto`  
**Package:** `morph.v1`  

:::warning[Transport security]
Daemon RPC currently uses **plaintext gRPC** with no application-level authentication. Bind `rpc.address` to loopback
unless the host network boundary protects the port. See [Security](../operations/security).
:::

## Client usage

When RPC address/port are not explicitly configured, clients prefer the active profile's `runtime.json`; stale metadata
is removed and clients fall back to config.

:::note[`runtime.json` is connection metadata]
`runtime.json` is written by the running daemon so clients can find the actual port, including port `0` binds. It is
not part of durable user state and can be recreated by starting the daemon again.
:::

Typical flow for chat:

1. `MorphService.Respond` — server-streaming; receive `TEXT_DELTA`, selected display-safe `TRACE_EVENT` events, then
   `DONE` or `ERROR`.
2. Session commands use `SessionService` unary RPCs against the same connection.

For HTTP gateway clients that bypass gRPC, see [Gateway Routes](./gateway-routes).

## MorphService

| Method | Type | Description |
| --- | --- | --- |
| `Respond` | **server stream** | Run one agent turn; stream text deltas and optional trace events |

### RespondRequest

| Field | Type | Description |
| --- | --- | --- |
| `message` | string | User message (required) |
| `instruct` | string | One-turn instruction (`request.instruct`) |
| `id` | string | Session ID (empty → current session) |
| `stream` | optional bool | Override profile streaming default |

### RespondEvent

| Field | Description |
| --- | --- |
| `type` | `TEXT_DELTA`, `TRACE_EVENT`, `DONE`, or `ERROR` |
| `text` | Stream chunk (assistant or reasoning channel) |
| `channel` | `ASSISTANT` or `REASONING` for text deltas |
| `error` | Error message when `type == ERROR` |
| `trace_type` | Trace event name when `type == TRACE_EVENT` |
| `trace_payload_json` | JSON payload for trace events |
| `trace_session_id` | Trace session identifier |
| `timestamp` | Event time (trace, error, done) |

Non-streaming responses may emit a single assistant `TEXT_DELTA` before `DONE`. Not every persisted trace event is
streamed to clients; sensitive or noisy events, such as full model request payloads, are stored for trace inspection
instead. Event names: [Trace Events](./trace-events).

## SessionService

| Method | Description |
| --- | --- |
| `Create` | Create a session by ID |
| `List` | List sessions (optional archived filter) |
| `Use` | Set current session |
| `Archive` / `Unarchive` | Archive lifecycle |
| `Rename` | Update session title |
| `Current` | Return current session record |
| `Compact` | Force summary compaction; returns compaction metrics |
| `Repair` | Repair vector/search artifacts (`Vector` repair type; optional `full`) |
| `Status` | Context usage: offset, tokens, compaction status |
| `Timeline` | Paginated messages and trace events for inspection |

### Timeline pagination

`GetSessionTimelineRequest` fields:

- `id` — session ID
- `message_offset`, `message_limit` — message page
- `trace_offset`, `trace_limit` — trace event page

Messages include role, content, tool calls, and timestamps. Trace entries include `type` and `payload_json`.

CLI equivalents: [CLI Reference](./cli#session--sessions-over-rpc). User guide: [Sessions Guide](../guides/sessions).

## ModelService

| Method | Description |
| --- | --- |
| `ListProviders` | Providers with auth type hints |
| `ListModels` | Models for a provider |
| `SelectModel` | Persist main (and summary) model selection to profile config |
| `SetProviderAPIKey` | Store provider API key in profile config |

Used by TUI `/models` and `/providers` flows. Auth details: [Provider Auth](../guides/provider-auth).

## GatewayService

| Method | Description |
| --- | --- |
| `GatewayStatus` | Runtime state, bind address, channel modes, last error |
| `Start` / `Stop` / `Restart` | Control gateway without daemon restart |
| `ListPairings` | Pending and approved sender pairings |
| `ApprovePairing` | Approve by source + code |
| `RevokePairing` | Revoke approved sender |
| `ClearPendingPairings` | Clear pending requests for a source |

CLI: `morph gateway …`. Operations: [Gateway Management](../operations/gateway-management).

### Pairing messages

- **Pending:** `source`, `sender_id`, `display_name`, timestamps, expiry
- **Approved:** `source`, `sender_id`, `display_name`, created/updated times

## Health

When enabled, the gRPC server registers the standard gRPC health service for liveness checks.

## Where To Go Next

- [Daemon and RPC](../concepts/daemon-and-rpc): mental model
- [Gateway Routes](./gateway-routes): HTTP ingress (parallel to RPC)
- [Trace Events](./trace-events): `TRACE_EVENT` payload types
- [CLI Reference](./cli): commands that call these services
- [Slash Commands](./slash-commands): TUI commands that call Session/Model services

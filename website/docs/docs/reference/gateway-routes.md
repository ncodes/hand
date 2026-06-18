---
title: Gateway Routes
description: HTTP gateway route reference.
---

# Gateway Routes

When `gateway.enabled` is true, the daemon serves an **HTTP listener** alongside gRPC (default `127.0.0.1:50052`).
External clients — generic HTTP integrations, Slack Events API, Telegram webhooks — hit these routes. Slack **socket**
and Telegram **polling** modes do not add HTTP routes.

Conceptual overview: [Gateways](../concepts/gateways). Setup guides: [Gateway Overview](../guides/gateway/).
Runtime control: [Gateway Management](../operations/gateway-management).

:::note[One listener, route-specific auth]
The generic route, Slack webhook, Telegram webhook, and health check share the same gateway HTTP listener. Authentication
is enforced per route: bearer token for `/v1/respond`, Slack signing headers for Slack, and Telegram secret-token
headers for Telegram.
:::

## Route table

| Path | Method | Handler | Enabled when |
| --- | --- | --- | --- |
| `/health` | any | Health check | Gateway HTTP enabled |
| `/v1/respond` | `POST` | Generic chat ingress | Gateway HTTP enabled |
| `/gateway/slack/webhook` | `POST` | Slack Events API | `gateway.slack.enabled` + `slack.mode: http` |
| `/gateway/telegram/webhook` | `POST` | Telegram updates | `gateway.telegram.enabled` + `telegram.mode: webhook` |

## Authentication

### `/health`

No authentication. Returns `200` with body `ok`.

### `/v1/respond` — Bearer token

| `gateway.authToken` | Behavior |
| --- | --- |
| Empty | Auth **skipped** by this route (any or missing `Authorization` header accepted) |
| Set | Requires `Authorization: Bearer <token>`; else `401 unauthorized` |

When the gateway binds to a **non-loopback** address, config validation **requires** `gateway.authToken`. See
[Security](../operations/security).

### `/gateway/slack/webhook` — Slack signature

Verifies Slack signing headers:

- `X-Slack-Request-Timestamp`
- `X-Slack-Signature`

Secret: `gateway.slack.signingSecret` (required in HTTP mode). Timestamp tolerance: five minutes. Invalid signature →
`401`.

Slack `url_verification` challenges return `{"challenge":"<value>"}`.

### `/gateway/telegram/webhook` — Telegram secret token

Header: `X-Telegram-Bot-Api-Secret-Token`  
Secret: `gateway.telegram.webhookSecret`

| Secret | Behavior |
| --- | --- |
| Empty | Check skipped (normally only possible if validation was bypassed) |
| Set | Header must match; else `401` |

Disabled integrations return `404` with a short message (`slack events are disabled`, `telegram webhook is disabled`).

## `/v1/respond` — generic HTTP

**Request** (`Content-Type: application/json`, max **1 MB**):

```json
{
  "conversation_id": "required-stable-id",
  "message": "required user text",
  "user_id": "optional",
  "source": "optional, default generic_http",
  "instruct": "optional one-turn instruction"
}
```

**Success** (`200`, after the agent response completes):

```json
{
  "conversation_id": "...",
  "session_id": "...",
  "text": "..."
}
```

**Error codes:** `bad_request`, `unauthorized`, `internal_error`

Each `conversation_id` maps to a Hand session via gateway bindings in `data/state.db`. See
[Sessions](../concepts/sessions).

:::note[Generic HTTP is request/response today]
`/v1/respond` waits for the turn to finish and returns one JSON response. It does not stream partial output yet.
:::

Guide: [Generic HTTP Gateway](../guides/gateway/generic-http).

## Slack webhook

- **Body:** Slack event JSON, max 1 MB
- **Mode:** only when `gateway.slack.mode: http`
- **Credentials:** `gateway.slack.botToken`, `gateway.slack.signingSecret`
- **Response delivery:** `gateway.slack.responseMode` (`thread` or `message`)
- **HTTP response:** returns quickly after the event is verified and queued; the assistant reply is sent back to Slack
  asynchronously

Guide: [Slack Gateway](../guides/gateway/slack).

## Telegram webhook

- **Body:** Telegram `Update` JSON, max 1 MB
- **Mode:** only when `gateway.telegram.mode: webhook`
- **Credentials:** `gateway.telegram.botToken`, `gateway.telegram.webhookSecret`
- **Registration:** `hand gateway setwebhook telegram [url]`
- **HTTP response:** returns quickly after the update is verified and queued; the assistant reply is sent through the
  Telegram Bot API asynchronously

Guide: [Telegram Gateway](../guides/gateway/telegram).

## Non-HTTP transports

| Provider | Config mode | Transport |
| --- | --- | --- |
| Slack | `socket` | Socket Mode WebSocket (`gateway.slack.appToken`) |
| Telegram | `polling` | Long polling (no webhook route) |

Recommended for local development: Slack **socket**, Telegram **polling**. See [FAQ](./faq).

## Sender authorization (Slack / Telegram)

After HTTP or socket delivery, inbound messages pass **sender checks** (not HTTP auth):

1. **Allowlists** — `gateway.allowedUsers`, or per-channel `slack.allowedUsers` / `telegram.allowedUsers`
2. **Approved pairings** — stored in profile state; managed via RPC/CLI pairing commands
3. **Pairing challenge** — unpaired senders in private chats receive a code; non-private unpaired traffic is ignored

Pairing: [Pairing and Allowlists](../guides/gateway/pairing-and-allowlists). RPC: [GatewayService](./rpc#gatewayservice).

## Config keys (quick reference)

| Key | Purpose |
| --- | --- |
| `gateway.enabled`, `gateway.address`, `gateway.port` | Listener |
| `gateway.authToken` | Bearer auth for `/v1/respond` |
| `gateway.pairingSecret` | Pairing code signing |
| `gateway.allowedUsers` | Global sender allowlist |
| `gateway.slack.*` | Slack mode, tokens, signing secret, allowlist |
| `gateway.telegram.*` | Telegram mode, bot token, webhook secret, allowlist |

Full defaults: [Config Reference](./config#gateway).

## Where To Go Next

- [Gateways](../concepts/gateways): architecture and session binding
- [RPC Reference](./rpc): `GatewayService` for start/stop/pairing
- [Config Reference](./config): all gateway keys
- [Environment Variables](./environment-variables): `HAND_GATEWAY_*` overrides
- [Security](../operations/security): exposure and token handling
- [Troubleshooting](../guides/troubleshooting): gateway connection issues

---
title: Pairing and Allowlists
description: Authorize gateway senders safely.
---

# Pairing and Allowlists

Slack and Telegram bots can be reached by anyone who finds them. Hand uses allowlists and pairing so only trusted
senders trigger agent turns. Generic HTTP uses a shared bearer token instead — see
[Generic HTTP](./generic-http#authentication).

This guide covers the operator workflow. For enabling gateways, see the [Gateway Overview](./). For the underlying
model, see [Gateways](../../concepts/gateways). Platform-specific chat rules are in [Telegram](./telegram) and
[Slack](./slack).

## Two Authorization Models

| Surface | Who is authorized | Config |
| --- | --- | --- |
| **Generic HTTP** | Anyone with the bearer token | `gateway.authToken` on `/v1/respond` |
| **Slack and Telegram** | Allowlisted or paired senders | `gateway.allowedUsers`, platform allowlists, `gateway.pairingSecret` |

Slack and Telegram verify platform requests first (signing secret, webhook secret, or socket/app tokens). Sender checks
run after that.

## How Sender Authorization Works

For each inbound Slack or Telegram message, Hand:

1. Reads the **sender id** from the platform payload — Telegram's numeric user id, or Slack's member id (`U…`).
2. Checks **allowlists** in profile config: `gateway.allowedUsers` and the platform list (`gateway.telegram.allowedUsers`
   or `gateway.slack.allowedUsers`). If listed, the agent turn runs.
3. If not allowlisted, checks the profile database for an **approved pairing** for that source and sender id. If found,
   the turn runs.
4. If still unknown and the chat is **pairing-eligible** (see below), Hand posts a pairing code in the chat and stops —
   no agent turn yet.
5. If still unknown and the chat is **not** pairing-eligible (Telegram group, Slack channel), the message is dropped
   with no reply.

Pairing approves the **sender identity**, not a session. Once approved, that person can trigger Hand in every context
the platform allows. Session bindings are separate — keyed by conversation and thread. See
[Sessions](../../concepts/sessions).

## Allowlists

Allowlists let you skip pairing for senders you already know.

| Config key | Scope |
| --- | --- |
| `gateway.allowedUsers` | Slack and Telegram |
| `gateway.telegram.allowedUsers` | Telegram only |
| `gateway.slack.allowedUsers` | Slack only |

A sender needs to appear in the global list **or** the platform list. Hand matches the exact trimmed sender id.

### Set allowlists in config

From a terminal, on the active profile:

```bash
hand config set gateway.allowedUsers "123456789,U01234567"
hand config set gateway.telegram.allowedUsers "123456789,987654321"
hand config set gateway.slack.allowedUsers "U01234567,U98765432"
```

The daemon restarts when config is valid. You can also set comma-separated env vars:
`HAND_GATEWAY_ALLOWED_USERS`, `HAND_GATEWAY_TELEGRAM_ALLOWED_USERS`, `HAND_GATEWAY_SLACK_ALLOWED_USERS`.

### Find a sender id

**Telegram** — numeric user id, not `@username`:

- Message a bot like `@userinfobot`, or
- Have the user DM your bot once, then run `hand gateway pairing list telegram` and read the **sender id** column.

**Slack** — member id starting with `U`:

- Open the member's profile in Slack → **More** → **Copy member ID** (wording varies by client), or
- Same as Telegram: after they message the app, `hand gateway pairing list slack`.

## Pairing Secret

Pairing codes are derived from `gateway.pairingSecret` in profile config. Set it before anyone tries to pair:

```bash
hand config set gateway.pairingSecret "$(openssl rand -hex 32)"
```

Or export `HAND_GATEWAY_PAIRING_SECRET`. If this field is empty, Hand cannot issue codes — unlisted senders in
pairing-eligible chats see nothing useful, and logs show `gateway pairing secret is required`.

Store the secret like a password (profile config or a secret manager, not git). Hand redacts it from traces and logs —
see [Safety and Guardrails](../../concepts/safety-and-guardrails).

Rotating the secret invalidates active codes immediately. Already-approved senders stay approved.

## Pairing Flow

End-to-end for one new user:

1. **You:** Enable the gateway and platform ([Gateway Overview](./), then [Telegram](./telegram) or [Slack](./slack)).
   Set `gateway.pairingSecret` and confirm the daemon is running (`hand gateway status`).
2. **User:** Opens a DM with your bot or Slack app (Telegram private chat; Slack `im` or group `mpim`) and sends a
   message.
3. **Hand:** Replies in that same chat with an 8-digit code and a shell command, for example
   `hand gateway pairing approve telegram 12345678`.
4. **You:** In a terminal on your machine (same profile, daemon running), run that command with the **current** code from
   the chat. Success prints `approved <source> <sender-id>`.
5. **User:** Sends another message in the chat. Hand runs a normal agent turn.

Codes rotate every **30 seconds**. If approve fails, wait for a fresh code in the chat. Each pending request **expires
after one hour**; the user can message again to get a new one.

Pending and approved records live in the active [profile](../../concepts/profiles) database — not in `config.yaml` —
and survive daemon restarts.

### Where pairing is offered

| Platform | Hand sends a pairing code | Hand ignores silently |
| --- | --- | --- |
| **Telegram** | Private chat with the bot | Groups and supergroups |
| **Slack** | DM (`im`) or group DM (`mpim`) | Channels — need allowlist or prior DM pairing; use `@`-mention there |

For Telegram groups and Slack channels, allowlist senders or approve them in a private chat first. See
[Telegram — Groups](./telegram#groups-and-supergroups) and [Slack — Channels](./slack#channels).

## Manage Pairings from the CLI

`hand gateway pairing` talks to the daemon over RPC (like `hand gateway status`). Start a daemon first
(`hand daemon`, or keep `hand` open). Use `--profile` for another profile.

### List pending and approved senders

```bash
hand gateway pairing list
hand gateway pairing list telegram
hand gateway pairing list slack
```

Output has two sections: **pending** (source, sender id, display name, expiry) and **approved**. Optional source
filters to one platform.

Use this to look up sender ids or confirm a pending request before approving.

### Approve a pairing

```bash
hand gateway pairing approve telegram <code>
hand gateway pairing approve slack <code>
```

`<source>` is `telegram` or `slack`. `<code>` is the 8-digit value from the user's chat — not their sender id.

On success: `approved telegram 123456789`. If nothing matches: `no pending gateway pairing matched code` (wrong or
expired code, clock skew, or secret changed). Rarely, two pending senders share a code →
`gateway pairing code matches multiple pending requests` — run `list`, then `clear-pending`.

### Revoke an approved sender

```bash
hand gateway pairing revoke telegram <sender-id>
hand gateway pairing revoke slack <sender-id>
```

Removes pairing approval only. Existing Hand sessions are unchanged; the sender must pair again or hit an allowlist to
trigger new turns.

### Clear pending requests

```bash
hand gateway pairing clear-pending
hand gateway pairing clear-pending telegram
```

Deletes pending requests, not approved senders. Use after testing or when you hit the **100 pending per source** limit
(`gateway pairing pending request limit reached`).

## Pairing vs Sessions

| | What it controls | Where it lives |
| --- | --- | --- |
| **Pairing / allowlist** | Whether a sender may trigger turns | Profile DB (pairing tables) |
| **Session binding** | Which Hand session continues a thread | Profile DB (`gateway_bindings`) |

Approving a sender does not pick or create your TUI session. Gateway traffic never changes the **current session** in
the terminal. See [Session Guide](../sessions).

## Generic HTTP Has No Pairing

`POST /v1/respond` has no per-sender allowlist or pairing. Protect the route with `gateway.authToken` and network
controls — see [Generic HTTP](./generic-http#authentication).

## Troubleshooting

### Pairing code never arrives

- Confirm `gateway.pairingSecret` is set in profile config and the gateway is running (`hand gateway status`).
- Confirm the user is in a pairing-eligible chat (Telegram private; Slack `im`/`mpim`).
- Check daemon logs for dispatch or secret errors.

### `no pending gateway pairing matched code`

- Copy the **current** code from the user's chat (rotates every 30s).
- Run `hand gateway pairing list <source>` — look for a non-expired pending row.
- After changing `gateway.pairingSecret`, restart the daemon so approve uses the same secret as the running gateway.

### Sender approved but messages still ignored

- **Telegram groups / Slack channels:** no pairing prompts there — allowlist or approve in DM first.
- Confirm the correct **source** (`telegram` vs `slack`) and sender id from `hand gateway pairing list`.
- **Slack channels:** user must `@`-mention the app; Slack app needs `app_mention` subscribed.

### Too many pending requests

Run `hand gateway pairing clear-pending <source>` or wait one hour for expiry.

### Pairing CLI cannot connect

Start the daemon first. Pairing commands do not start it or reload config from disk. See
[Daemon and RPC](../../concepts/daemon-and-rpc).

## Where To Go Next

- [Gateway Overview](./): enable gateways and runtime commands.
- [Telegram](./telegram): bot setup and Telegram authorization context.
- [Slack](./slack): app setup and Slack authorization context.
- [Generic HTTP](./generic-http): bearer-token auth.
- [Gateways](../../concepts/gateways): binding model and message flow.
- [Gateway Management](../../operations/gateway-management): start, stop, restart.
- [Config Guide](../config): changing gateway settings safely.
- [Profiles](../../concepts/profiles): where pairing state lives.
- [Safety and Guardrails](../../concepts/safety-and-guardrails): secret handling in logs and traces.

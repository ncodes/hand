---
title: Gateways
description: Messaging surfaces and session bridging.
---

# Gateways

Gateways let you reach the same Morph agent from outside the terminal — from Slack, from a Telegram chat, or from your
own service over HTTP. A gateway receives an inbound message from an external platform, routes it to a Morph
[session](./sessions), runs a normal agent turn, and sends the reply back to that platform. The agent, tools, memory,
and history are exactly the same as in the TUI; only the surface differs.

This page explains the gateway model. For setting up a specific platform, see the [Gateway Guides](../guides/gateway/);
for running and controlling gateways, see [Gateway Management](../operations/gateway-management); for the exact HTTP
endpoints, see [Gateway Routes](../reference/gateway-routes).

## The Three Gateways

Morph ships three gateway types, all served by the same daemon:

- **Generic HTTP** — a plain JSON endpoint (`POST /v1/respond`) for your own integrations. You send text and a
  conversation id; Morph returns the final reply in the response body.
- **Slack** — connects a Slack app to Morph, either over Slack's Socket Mode or its Events API webhook.
- **Telegram** — connects a Telegram bot to Morph, either by long polling or by webhook.

All three are configured under a single `gateway` section of the profile config and share the same session binding,
authorization, and message-handling machinery; only the transport and platform details differ.

## Owned by the Daemon

Gateways are part of the [daemon](./daemon-and-rpc), not a separate process. When the daemons, it reads the
`gateway` config and, if gateways are enabled, opens a local HTTP listener (default `127.0.0.1:50052`) and starts any
configured Slack and Telegram components. When the daemon shuts down, the gateways stop with it, and if gateways are
disabled it simply skips this step.

Because settings are read at daemon, applying changed gateway settings means the daemon must restart — which it
does automatically when you edit `config.yaml` (see [Daemon and RPC](./daemon-and-rpc) for the config-reload behavior).

Separately, you can control the running gateways without touching config: `morph gateway start`, `morph gateway stop`, and
`morph gateway restart` operate them, and `morph gateway status` inspects them. These act on the daemon's current
configuration — they are for operational control and recovery, not for loading new file edits. See
[Gateway Management](../operations/gateway-management).

## Transports

Each platform offers a local-friendly mode and a webhook mode, selected in config:

- **Slack** runs in `socket` mode by default: the daemon makes an outbound WebSocket connection to Slack, so no inbound
  port needs to be exposed (this requires an app-level token). In `http` mode, Slack delivers events to an inbound
  webhook at `/gateway/slack/webhook` (this requires a signing secret to verify requests). Replies can be posted in a
  thread (default) or as a top-level message.
- **Telegram** runs in `polling` mode by default: the daemon polls Telegram for updates over an outbound connection. In
  `webhook` mode, Telegram delivers updates to an inbound POST at `/gateway/telegram/webhook`, verified with a secret
  header.
- **Generic HTTP** is always the inbound `POST /v1/respond` endpoint.

The socket and polling modes keep everything outbound, which is convenient for a local or private deployment; the
webhook modes are for when you can expose an HTTP endpoint to the platform.

All HTTP routes share one gateway listener: `/health`, generic HTTP at `/v1/respond`, and any Slack or Telegram webhook
routes you enable. Slack and Telegram verify their own webhook requests with platform secrets; `gateway.authToken`
protects the generic HTTP route. Because that route is available on the same listener, Morph requires
`gateway.authToken` when the gateway binds to a non-loopback address. The full route table and request/response shapes
are in [Gateway Routes](../reference/gateway-routes).

## Conversations Become Sessions

A gateway maps each external conversation to a Morph session so that an ongoing thread keeps a continuous history. The
mapping is keyed by the conversation and its thread — for Slack, the team, channel, and thread; for Telegram, the chat
and topic; for generic HTTP, the conversation id you supply — **not** by the individual sender. Everyone in a shared
thread therefore talks to the same session.

The binding is created the first time a conversation is seen and reused on every later message; it is stored in the
profile's database (`gateway_bindings`). If the bound session is later deleted, the next message creates a fresh session
for that conversation. Sessions created from a gateway also record their origin (which platform and conversation they
came from).

Gateway traffic never changes the **current session** that the TUI and CLI use: gateway turns always target their bound
session explicitly, so your interactive work and your messaging surfaces stay independent. See [Sessions](./sessions)
for the session model this builds on.

## Authorization and Pairing

The generic HTTP gateway is protected by an optional bearer token (`gateway.authToken`): when set, callers must present
it; there is no per-sender concept. Slack and Telegram, which face real users, add sender authorization on top of the
platform's own request verification (Slack signing secret / Telegram secret header):

- **Allowlists.** A sender is allowed immediately if they appear in the global `gateway.allowedUsers` list or the
  per-platform allowlist.
- **Pairing.** Otherwise, a sender messaging the bot in a private chat receives a pairing challenge — a short code,
  generated from `gateway.pairingSecret` — which an operator approves with `morph gateway pairing approve <source>
  <code>`. Once approved, the sender is remembered. Messages from unauthorized senders in non-private contexts (such as
  a public channel) are ignored rather than answered. Pending and approved pairings are managed with the
  `morph gateway pairing` commands and stored in the profile database.

For the operator workflow around allowlists and pairing, see [Pairing and Allowlists](../guides/gateway/pairing-and-allowlists).

## How a Message Is Handled

The inbound path is the same for every gateway:

1. The message arrives (webhook, socket event, or poll) and is normalized.
2. For Slack and Telegram, the sender is authorized (allowlist or pairing); the generic endpoint checks its bearer token.
3. The conversation is resolved to a session via its binding, creating one if needed.
4. Morph runs a normal agent turn against that session — same tools, memory, and history as any other surface.
5. The reply is delivered back to the platform.

Slack and Telegram **stream** the assistant's reply, updating the message as text arrives, while the generic HTTP
endpoint waits for the full turn and returns a single JSON body. Either way the conversation is persisted to the
session, so the next message continues seamlessly.

## Profiles and Isolation

Gateways belong to a [profile](./profiles): their configuration lives in the profile's `config.yaml`, and their state —
session bindings and pairing records — lives in the profile's database alongside sessions and memory. A daemon runs one
profile at a time, so switching profiles means a different gateway configuration and a different set of bindings.
Credentials such as bot tokens and signing secrets are part of profile config and are treated as secrets — they are
redacted from traces and logs, as described in [Safety and Guardrails](./safety-and-guardrails).

## Where To Go Next

- [Gateway Guides](../guides/gateway/): set up Slack, Telegram, or the generic HTTP gateway.
- [Pairing and Allowlists](../guides/gateway/pairing-and-allowlists): authorize senders.
- [Gateway Management](../operations/gateway-management): start, stop, restart, and inspect gateways.
- [Gateway Routes](../reference/gateway-routes): the HTTP endpoints and their auth.
- [Sessions](./sessions): the session model conversations bind to.
- [Daemon and RPC](./daemon-and-rpc): the process that owns gateways.
- [Profiles](./profiles): how gateway config and state are isolated.
- [Gateway Internals](../development/gateway-internals): the implementation-level design.

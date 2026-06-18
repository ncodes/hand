---
title: Troubleshooting
description: Fix common Hand setup and runtime issues.
---

# Troubleshooting

This guide collects recurring setup and runtime problems and points you to the right fix. Start with **`hand doctor`**
on the active profile — it validates config, resolves model credentials, and reports readiness for the daemon,
gateway, search, and memory subsystems. For what each check means, see [Doctor](../operations/doctor).

If you are new to Hand, the [Learning Path](../getting-started/learning-path) routes you here when something breaks
during setup.

## Start Here

Run diagnostics before changing random settings:

```bash
hand doctor
```

`hand doctor` prints two layers (see [Doctor — Output formats](../operations/doctor#output-formats) for text vs
`--json`):

1. **Config diagnostics** — env/config files load and config validates. In text output, **config load** and
   **config validation** appear under the **profile** group; credential checks appear under **models**.
2. **Readiness groups** — profile paths, daemon reachability, models, session, memory, search, safety, gateway, and
   web tools.

Each line is **PASS**, **WARN**, or **FAIL**. Only **FAIL** makes `hand doctor` exit with an error; **WARN** is
informational (for example vector search disabled, or daemon not running yet). When a check includes a suggested
command, run it and re-check.

For machine-readable output:

```bash
hand doctor --json
```

The JSON form uses the same groups as text output under `groups`; scripts should check `ok` and then inspect
`groups[].checks[]` for specific failures or actions.

Confirm you are on the intended [profile](../concepts/profiles):

```bash
hand profile list
hand --profile <name> doctor
```

## General Workflow

When something misbehaves:

1. Run `hand doctor` and fix any **FAIL** items (follow suggested commands when shown).
2. Confirm the **daemon** is running if the feature needs it — TUI with an existing daemon, `hand daemon`, or a
   long-lived `hand` session. See [Daemon and RPC](../concepts/daemon-and-rpc).
3. Confirm **model auth** for the role involved (`hand auth status`, then `hand doctor` **models** group).
4. Turn up logging if you need more detail (`log.level debug`) — see [Logging and Debug](#logging-and-debug).
5. For gateway, search, memory, or session issues, use the focused section below or the platform guide.

## Daemon and RPC Unreachable

Symptoms: `hand session`, `hand gateway`, or the TUI cannot connect; doctor **daemon** group warns or fails; stale
`runtime.json`.

### Daemon not running

The daemon owns the agent runtime, storage, and gateways. Start it explicitly:

```bash
hand daemon
```

Or open the TUI (`hand`) — it starts a temporary daemon if none is running. RPC commands such as `hand session list` or
`hand gateway status` do **not** start a daemon on their own.

Doctor shows **daemon runtime** as a warning when nothing is listening. After `hand daemon`, re-run `hand doctor`
— the **daemon** group should pass with the profile's RPC address and port.

### Wrong profile or endpoint

Clients resolve the daemon through profile `runtime.json` or explicit RPC settings. If you pointed RPC at the wrong host
or port:

```bash
hand config get rpc.address rpc.port
hand config set rpc.address 127.0.0.1
hand config set rpc.port 50051
```

Flags and `HAND_RPC_ADDRESS` / `HAND_RPC_PORT` override config. See [Daemon and RPC](../concepts/daemon-and-rpc) and
[Environment Variables](../reference/environment-variables).

### Stale runtime metadata

If a daemon crashed or was killed, `runtime.json` may reference a dead process. Hand removes stale metadata when a
connection fails; starting a fresh daemon (`hand daemon`) or re-opening the TUI usually clears it. If problems
persist, check that nothing else is bound to the RPC port.

## Provider Auth and Model Errors

Symptoms: turns fail immediately; doctor **config validation** or **models** checks fail; TUI setup prompts for credentials.

### Check what Hand resolved

```bash
hand auth status
hand auth status <provider>
hand doctor
```

Doctor's **models** group reports **main**, **summary**, and **embedding** (when vector search is on) with the provider,
model name, and credential source. Readiness failures often include the exact next command — for example
`hand auth login openai-codex` or setting a role-specific API key.

Hand resolves credentials in order: role config → stored `auth.json` → environment → provider config. See
[Provider Auth](./provider-auth).

### Common fixes

| Problem | What to do |
| --- | --- |
| No credential for provider | `hand auth login <provider>` or `hand auth login <provider> --api-key "<key>"` |
| Wrong provider in config | `hand config set models.main.provider …` and `models.main.name …` — see [Config Guide](./config) |
| OAuth model not available on subscription | Pick a model that provider supports via OAuth, or use API key auth |
| Summary/embedding failures | Set `models.summary` / `models.embedding` and auth for those providers — background memory and vector search depend on them |

Gateway turns use the same model runtime as the TUI. Fix **models** before debugging Slack or Telegram reply issues.

## Config Validation Failures

Symptoms: `hand config set` rejects a value; doctor **config validation** fails; daemon refuses invalid config.

`hand config set` validates the **whole** profile config before writing. A common pattern is enabling a gateway
platform without its required tokens — enable `gateway.enabled` first, then set platform tokens together. See
[Gateway Overview](./gateway/).

Other frequent validation errors:

- **Log level** — must be `debug`, `info`, `warn`, or `error` (`hand config set log.level debug`).
- **Gateway bind** — non-loopback `gateway.address` requires `gateway.authToken`.
- **Slack/Telegram mode** — socket mode needs `gateway.slack.appToken`; webhook/HTTP modes need signing secrets or
  webhook secrets.

Read the error message literally — it names the missing or invalid field. Cross-check [Config Reference](../reference/config).

`.env` changes are **not** picked up automatically; restart the daemon after editing `.env`. Changes to `config.yaml`
via `hand config set` restart the daemon when valid.

## SQLite FTS5 and Source Builds

Symptoms: search fails; tests error with `no such module: fts5`; session message search returns nothing after a source build.

Pre-built binaries from the install script include FTS5. **Source builds** must enable CGO and the FTS5 tag:

```bash
make build
make test
```

For a focused package test:

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./cmd/hand
```

You need a C toolchain for CGO. See [Installation — Verify the runtime build](../getting-started/installation#verify-the-runtime-build)
and [Search and Traces — SQLite FTS5](./search-and-traces#sqlite-fts5).

End users who install via the script do not configure FTS5 separately.

## Gateway Issues

Symptoms: `hand gateway status` shows `state=failed`; no Slack/Telegram events; webhooks return 401/404; messages ignored.

### Gateway readiness

Run `hand doctor` and read the **gateway** group:

| Check | Typical issue |
| --- | --- |
| **listener** | Gateway enabled on a public bind without `gateway.authToken` |
| **telegram** | Enabled without `gateway.telegram.botToken`, or webhook mode without `gateway.telegram.webhookSecret` |
| **slack** | Enabled without bot token, socket mode without `gateway.slack.appToken`, or HTTP mode without signing secret |

Fix with `hand config set` as doctor suggests, then `hand gateway restart` after the daemon has reloaded config.

### No events or no reply

- **Daemon/gateway running** — `hand gateway status` should show `state=running`.
- **Model auth** — gateway turns need working main model credentials.
- **Sender authorization** — Slack/Telegram require allowlist or pairing; generic HTTP needs the bearer token. See
  [Pairing and Allowlists](./gateway/pairing-and-allowlists).
- **Platform setup** — tokens, event subscriptions, and mode-specific requirements differ by surface.

Platform guides with focused troubleshooting:

- [Gateway Overview](./gateway/#troubleshooting)
- [Telegram](./gateway/telegram#troubleshooting)
- [Slack](./gateway/slack#troubleshooting)
- [Generic HTTP](./gateway/generic-http#troubleshooting)
- [Pairing and Allowlists](./gateway/pairing-and-allowlists#troubleshooting)

### Webhook 401 or 404

**401 Unauthorized**

- **Telegram** — `gateway.telegram.webhookSecret` must match the `secret_token` passed to Telegram's `setWebhook`. The
  proxy must forward `X-Telegram-Bot-Api-Secret-Token`.
- **Slack HTTP mode** — signing secret must match the Slack app; proxy must forward the raw body and
  `X-Slack-Signature` / `X-Slack-Request-Timestamp` unchanged.
- **Generic HTTP** — when `gateway.authToken` is set, callers need `Authorization: Bearer <token>` on `/v1/respond`.

**404 Not Found**

- Confirm the platform is enabled and mode matches (Telegram `webhook`, Slack `http`).
- URL path must be exact: `/gateway/telegram/webhook` or `/gateway/slack/webhook` on the gateway listener port
  (`gateway.address`:`gateway.port`, default `127.0.0.1:50052`).
- Reverse proxy must route to Hand's gateway listener, not the RPC port.

See [Gateway Routes](../reference/gateway-routes).

## Search, Vector, and Memory

Symptoms: search returns nothing; doctor **search** or **memory** warnings; promotion or retrieval seems broken.

### Search

- Confirm messages were **persisted** — only stored session content is indexed.
- **Vector search** needs `search.vector.enabled`, embedding model auth, and (when `search.vector.required` is true)
  a passing **embedding** check in doctor. Lexical BM25 search works without vectors.
- **Stale hybrid rankings** — `hand session repair` for the affected session. See [Search and Traces](./search-and-traces).

### Memory

Doctor's **memory** group lists effective state for pinned, retrieval, flush, episodic, reflection, promotion, and write
paths. Common issues:

- **Daemon required** — background episodic/reflection/promotion loops run in the daemon, not in one-shot `hand -c`.
- **Summary model** — background memory work uses `models.summary`; verify auth in doctor.
- **Writes blocked** — safety rejection or `memory.write.enabled` / `cap.mem` off. Inspect traces for
  `memory.safety.blocked` or `memory.promotion.decision`.

See [Memory Guide — Troubleshooting](./memory#troubleshooting) and [Search and Traces — Troubleshooting](./search-and-traces#troubleshooting).

## TUI and Terminal

Symptoms: blank or garbled UI; keybindings seem dead; TUI exits immediately.

### Connection and setup

The TUI talks to the daemon over RPC. If credentials are missing, `/setup` or the first-run flow walks you through model
configuration — see [TUI Guide](./tui) and [Provider Auth](./provider-auth).

If the TUI started a temporary daemon, exiting stops it; a separately started daemon keeps running.

### Rendering

- Use a **modern terminal** with adequate size — very narrow windows can clip the layout.
- Disable color if your terminal mishandles ANSI: `hand config set log.noColor true`.
- **Cancel a stuck turn** with **Esc**; exit with **Ctrl+C** (twice to confirm).

For keybindings and slash commands, see [TUI Guide](./tui) and [Slash Commands](../reference/slash-commands).

## Logging and Debug

Use logging when doctor passes but runtime behavior is still wrong.

### Log level and file

```bash
hand config set log.level debug
hand config get log.file
```

Valid levels: `debug`, `info`, `warn`, `error`. Override with `HAND_LOG_LEVEL` or `--log.level`.

Optional file logging via `log.file` in config (with rotation settings `log.maxSizeMB`, `log.maxBackups`,
`log.maxAgeDays`, `log.compress`). Daemon logs include gateway dispatch, memory loops, and model errors.

### Debug request dumps

For verbose provider request logging (development only — may include sensitive content):

```bash
hand config set debug.requests true
```

Restart the daemon after changing `debug.requests`. See [Config Guide](./config) for `log` and `debug` sections.

### Traces

For turn-level detail — tool calls, memory events, timing — enable tracing and use `hand trace view` or database-backed
timeline hydration. See [Search and Traces](./search-and-traces).

## Where To Go Next

- [Doctor](../operations/doctor): PASS, WARN, FAIL, and readiness groups in depth.
- [Provider Auth](./provider-auth): credentials and model roles.
- [Config Guide](./config): changing settings safely.
- [Daemon Operations](../operations/daemon): starting and stopping the daemon.
- [Gateway Overview](./gateway/): external messaging surfaces.
- [Installation](../getting-started/installation): install script, source build, FTS5.
- [Profiles and Config](../getting-started/profiles-and-config): profile layout and precedence.
- [Learning Path](../getting-started/learning-path): doc routes by goal.

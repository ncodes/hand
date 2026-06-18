---
title: CLI Reference
description: Hand command-line interface reference.
---

# CLI Reference

This page documents the `hand` binary: subcommands, global flags, and common invocation patterns. For task-oriented
workflows, see the [TUI Guide](../guides/tui), [Profiles and Config](../getting-started/profiles-and-config), and
[Learning Path](../getting-started/learning-path). For config keys behind the flags, see
[Config Reference](./config) and [Environment Variables](./environment-variables).

The single binary opens the TUI by default, or runs one of the subcommands below for profile, daemon, gateway, session,
and diagnostic workflows.

## Invocation

```text
hand [global options] [command [command options] [arguments]]
hand [global options] --chat|-c [--session ID] [--instruct TEXT] "message"
```

Global flags may appear before subcommands: `hand --profile work daemon`.

### Default behavior

| Invocation | Result |
| --- | --- |
| `hand` | Interactive TUI |
| `hand --chat "…"` / `hand -c "…"` | One-shot chat over RPC; starts a quiet local daemon if none is reachable |
| `hand --version` / `-v` | Version line |
| `hand --help` | Help with examples |

Before command dispatch, Hand resolves the active profile (`--profile` / `-p` or `HAND_PROFILE`) and preloads the
profile `.env` file. Commands that need config or RPC then run their own validation or readiness checks. One-shot chat
uses the active profile's `runtime.json` when a daemon is already running; otherwise it falls back to config and starts
a temporary daemon for the request.

:::note[Doctor is explicit]
`hand doctor` is the full readiness command. Other commands do not run the entire doctor suite before they start,
though they may fail fast on the specific config, auth, or daemon state they require.
:::

### Root-only flags

| Flag | Alias | Description |
| --- | --- | --- |
| `--chat` | `-c` | Send root arguments as a one-shot chat message |
| `--instruct` | | Per-request instruction; **cleared when the response finishes** |
| `--session` | | Session ID for the chat request (default: current session) |

:::tip[Two `--instruct` semantics]
Root `--chat --instruct` applies to **one request**. `hand daemon --instruct` sets a **server instruction** that
persists until the daemon exits.
:::

## Global flags

Visible flags appear in default help. Many advanced settings also exist as hidden global flags that mirror
[Config Reference](./config) paths with `--` instead of dots.

### Profile and paths

| Flag | Alias | Env | Default | Description |
| --- | --- | --- | --- | --- |
| `--profile` | `-p` | `HAND_PROFILE` | active profile | Profile for config, env, and runtime |
| `--env-file` | | `HAND_ENV_FILE` | `.env` | Env file to preload |
| `--config` | | `HAND_CONFIG` | `config.yaml` | Profile config YAML path |

### Agent and model

| Flag | Description |
| --- | --- |
| `--name` | Agent display name |
| `--model` | Main model ID |
| `--model.summary` | Summary/compaction model (defaults to main) |
| `--model.stream` | Stream assistant text |

### RPC and gateway

| Flag | Description |
| --- | --- |
| `--rpc.address`, `--rpc.port` | Daemon gRPC bind |
| `--gateway.enabled` | Enable HTTP gateway in daemon |
| `--gateway.address`, `--gateway.port` | Gateway bind |
| `--gateway.telegram.enabled`, `--gateway.telegram.mode` | Telegram ingress (`polling` / `webhook`) |
| `--gateway.slack.enabled`, `--gateway.slack.mode`, `--gateway.slack.response-mode` | Slack ingress (`socket` / `http`; `thread` / `message`) |

### Logging and trace

| Flag | Description |
| --- | --- |
| `--log.level` | `debug`, `info`, `warn`, or `error` |
| `--debug.requests` | Log sanitized model requests at debug |
| `--trace.enabled` | Persist per-session trace events |

Hidden globals mirror most config keys: capabilities (`--cap.*`), exec rules, storage, memory, web, compaction,
session limits, trace disk/database paths, and more. Prefer `hand config set` for durable changes; see
[Config Guide](../guides/config).

## Subcommands

### `auth` — provider credentials

| Subcommand | Usage | Notes |
| --- | --- | --- |
| `auth login` | `hand auth login <provider>` | `--api-key`, `--token`, `--refresh-token`, `--expires-at`, `--scope` |
| `auth status` | `hand auth status [provider…]` | Shows credential sources |
| `auth logout` | `hand auth logout <provider>` | Removes stored credentials |

See [Provider Auth](../guides/provider-auth).

### `config` — profile YAML

| Subcommand | Usage |
| --- | --- |
| `config get` | `hand config get <path>…` |
| `config set` | `hand config set <path> <value>` or `hand config set path=value …` |

Paths use dot notation (`session.maxIterations`). See [Config Reference](./config).

### `daemon` — run the daemon

| Invocation | Behavior |
| --- | --- |
| `hand daemon` | Start daemon with config reload |
| `hand daemon status` | Print health, PID, RPC address, uptime |

See [Daemon Operations](../operations/daemon).

| Flag | Description |
| --- | --- |
| `--instruct` | Persistent server instruction until process exit |

### `db` — local database

| Subcommand | Usage | Notes |
| --- | --- | --- |
| `db reset` | `hand db reset --force` | Deletes SQLite DB + WAL/SHM sidecars; **requires `--force`**; `storage.backend` must be `sqlite` |

See [Backups and State](../operations/backups-and-state).

### `doctor` — readiness checks

```bash
hand doctor
hand doctor --json
```

Exit code `1` on FAIL. Default output is human text; `--json` prints structured diagnostics. See
[Doctor](../operations/doctor).

### `gateway` — gateway runtime and pairing

| Subcommand | Description |
| --- | --- |
| `gateway status` | Daemon gateway runtime status |
| `gateway start` / `stop` / `restart` | Control gateway without full daemon restart |
| `gateway setwebhook telegram [url]` | Register or clear Telegram webhook URL |
| `gateway pairing list [source]` | Pending and approved pairings |
| `gateway pairing approve <source> <code>` | Approve a pairing request |
| `gateway pairing revoke <source> <sender-id>` | Revoke an approved sender |
| `gateway pairing clear-pending [source]` | Clear pending requests |

`hand gateway stop` stops the **gateway runtime**, not the daemon. See [Gateway Management](../operations/gateway-management)
and [Gateway Routes](./gateway-routes).

### `profile` — profile selection

Profile subcommands do **not** take `--profile` — they manage which profile is active.

| Subcommand | Description |
| --- | --- |
| `profile use <name>` | Set machine-local current profile |
| `profile list` | List profile directories |
| `profile current` | Print stored current profile |
| `profile init <name>` | Create profile (`--bare`, `--use`) |
| `profile path [name]` | Print profile home path |
| `profile doctor [name]` | Print paths and file existence |

See [Profiles](../concepts/profiles) and [Profiles and Config](../getting-started/profiles-and-config).

### `session` — sessions over RPC

Requires a subcommand. Uses daemon RPC; start the daemon first.

| Subcommand | Description |
| --- | --- |
| `session new <id>` | Create session (does not switch current) |
| `session list` | List persisted sessions |
| `session use <id>` | Set current session |
| `session current` | Show current selection |
| `session compact [id]` | Force summary compaction |
| `session repair [id]` | Repair storage artifacts (`--full`) |
| `session status [id]` | Context usage metrics |
| `session unarchive <id>` | Restore archived session |

See [Sessions Guide](../guides/sessions) and [RPC Reference](./rpc#sessionservice).

### `trace` — trace viewer

| Subcommand | Flags | Description |
| --- | --- | --- |
| `trace view` | `--trace-dir`, `--listen`, `--username`, `--password` | Local HTTP UI for JSONL traces |

Default listen: `127.0.0.1:0`. Basic auth requires both `--username` and `--password`. See
[Search and Traces](../guides/search-and-traces).

### `version`

```bash
hand version
```

Prints version and commit hash.

## Quick command index

| Command | Purpose |
| --- | --- |
| *(default)* | TUI |
| `auth` | Provider login / status / logout |
| `config` | Get/set profile config |
| `daemon` | Run daemon or check status |
| `db` | SQLite reset |
| `doctor` | Readiness diagnostics |
| `gateway` | Gateway runtime, webhooks, pairing |
| `profile` | Profile create, select, inspect |
| `session` | Session CRUD and maintenance |
| `trace` | Trace viewer |
| `version` | Version info |

## Examples

```bash
# TUI on a named profile
hand --profile work

# Start daemon
hand daemon
hand --profile work daemon

# One-shot chat
hand --chat "summarize the failing tests"
hand -c --session ses_abc123 --instruct "be brief" "continue"

# Config and doctor
hand config set session.maxIterations 30
hand doctor --json

# Gateway and sessions
hand gateway status
hand session list
HAND_PROFILE=work hand session compact
```

## Where To Go Next

- [Slash Commands](./slash-commands): in-TUI `/` commands
- [Config Reference](./config): every config key and default
- [Environment Variables](./environment-variables): `HAND_*` overrides
- [RPC Reference](./rpc): gRPC services used by CLI clients
- [FAQ](./faq): common CLI questions

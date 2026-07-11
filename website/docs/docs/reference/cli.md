---
title: CLI Reference
description: Morph command-line interface reference.
---

# CLI Reference

This page documents the `morph` binary: subcommands, global flags, and common invocation patterns. For task-oriented
workflows, see the [TUI Guide](../guides/tui), [Profiles and Config](../getting-started/profiles-and-config), and
[Learning Path](../getting-started/learning-path). For config keys behind the flags, see
[Config Reference](./config) and [Environment Variables](./environment-variables).

The single binary opens the TUI by default, or runs one of the subcommands below for profile, daemon, gateway, session,
and diagnostic workflows.

## Invocation

```text
morph [global options] [command [command options] [arguments]]
morph [global options] --chat|-c [--session ID] [--instruct TEXT] "message"
```

Global flags may appear before subcommands: `morph --profile work daemon`.

### Default behavior

| Invocation | Result |
| --- | --- |
| `morph` | Interactive TUI |
| `morph --chat "…"` / `morph -c "…"` | One-shot chat over RPC; starts a quiet local daemon if none is reachable |
| `morph --version` / `-v` | Version line |
| `morph --help` | Help with examples |

Before command dispatch, Morph resolves the active profile (`--profile` / `-p` or `MORPH_PROFILE`) and preloads the
profile `.env` file. Commands that need config or RPC then run their own validation or readiness checks. One-shot chat
uses the active profile's `runtime.json` when a daemon is already running; otherwise it falls back to config and starts
a temporary daemon for the request.

:::note[Doctor is explicit]
`morph doctor` is the full readiness command. Other commands do not run the entire doctor suite before they start,
though they may fail fast on the specific config, auth, or daemon state they require.
:::

### Root-only flags

| Flag | Alias | Description |
| --- | --- | --- |
| `--chat` | `-c` | Send root arguments as a one-shot chat message |
| `--instruct` | | Per-request instruction; **cleared when the response finishes** |
| `--session` | | Session ID for the chat request (default: current session) |
| `--pull` | | Pull the selected Ollama model before one-shot chat when using the Ollama provider |
| `--pull-quiet` | | Suppress Ollama pull progress output |

:::tip[Two `--instruct` semantics]
Root `--chat --instruct` applies to **one request**. `morph daemon --instruct` sets a **server instruction** that
persists until the daemon exits.
:::

## Global flags

Visible flags appear in default help. Many advanced settings also exist as hidden global flags that mirror
[Config Reference](./config) paths with `--` instead of dots.

### Profile and paths

| Flag | Alias | Env | Default | Description |
| --- | --- | --- | --- | --- |
| `--profile` | `-p` | `MORPH_PROFILE` | active profile | Profile for config, env, and runtime |
| `--env-file` | | `MORPH_ENV_FILE` | `.env` | Env file to preload |
| `--config` | | `MORPH_CONFIG` | `config.yaml` | Profile config YAML path |

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
session limits, trace disk/database paths, and more. Prefer `morph config set` for durable changes; see
[Config Guide](../guides/config).

## Subcommands

### `auth`: provider credentials

| Subcommand | Usage | Notes |
| --- | --- | --- |
| `auth login` | `morph auth login <provider>` | `--api-key`, `--token`, `--refresh-token`, `--expires-at`, `--scope` |
| `auth status` | `morph auth status [provider…]` | Shows credential sources |
| `auth logout` | `morph auth logout <provider>` | Removes stored credentials |

See [Provider Auth](../guides/provider-auth).

### `automation`: scheduled agent jobs

| Subcommand | Usage | Notes |
| --- | --- | --- |
| `automation status` | `morph automation status` | Scheduler snapshot |
| `automation list` | `morph automation list [--all] [--limit N]` | List jobs |
| `automation add` | `morph automation add --schedule … --prompt …` | Create a job |
| `automation update` | `morph automation update <job-id> …` | Patch a job; omitted flags unchanged |
| `automation pause` / `resume` | `morph automation pause\|resume <job-id>` | Toggle `enabled` |
| `automation run` | `morph automation run <job-id>` | Trigger a run now |
| `automation remove` | `morph automation remove <job-id>` | Delete the job definition |
| `automation runs` | `morph automation runs [--job ID] [--status …] [--limit N]` | List run history |
| `automation diagnose` / `inspect` / `recover` | | Operator diagnostics and recovery |

Full flag and enum reference: [Automation Reference](./automation). Walkthrough: [Automation Guide](../guides/automation).

### `config`: profile YAML

| Subcommand | Usage |
| --- | --- |
| `config get` | `morph config get <path>…` |
| `config set` | `morph config set <path> <value>` or `morph config set path=value …` |

Paths use dot notation (`session.maxIterations`). See [Config Reference](./config).

### `daemon`: run the daemon

| Invocation | Behavior |
| --- | --- |
| `morph daemon` | Start daemon with config reload |
| `morph daemon status` | Print health, PID, RPC address, uptime |

See [Daemon Operations](../operations/daemon).

| Flag | Description |
| --- | --- |
| `--instruct` | Persistent server instruction until process exit |

### `db`: local database

| Subcommand | Usage | Notes |
| --- | --- | --- |
| `db reset` | `morph db reset --force` | Deletes SQLite DB + WAL/SHM sidecars; **requires `--force`**; `storage.backend` must be `sqlite` |

See [Backups and State](../operations/backups-and-state).

### `doctor`: readiness checks

```bash
morph doctor
morph doctor --json
```

Exit code `1` on FAIL. Default output is human text; `--json` prints structured diagnostics. See
[Doctor](../operations/doctor).

### `gateway`: gateway runtime and pairing

| Subcommand | Description |
| --- | --- |
| `gateway status` | Daemon gateway runtime status |
| `gateway start` / `stop` / `restart` | Control gateway without full daemon restart |
| `gateway setwebhook telegram [url]` | Register or clear Telegram webhook URL |
| `gateway pairing list [source]` | Pending and approved pairings |
| `gateway pairing approve <source> <code>` | Approve a pairing request |
| `gateway pairing revoke <source> <sender-id>` | Revoke an approved sender |
| `gateway pairing clear-pending [source]` | Clear pending requests |

`morph gateway stop` stops the **gateway runtime**, not the daemon. See [Gateway Management](../operations/gateway-management)
and [Gateway Routes](./gateway-routes).

### `profile`: profile selection

Profile subcommands do **not** take `--profile`; they manage which profile is active.

| Subcommand | Description |
| --- | --- |
| `profile use <name>` | Set machine-local current profile |
| `profile list` | List profile directories |
| `profile current` | Print stored current profile |
| `profile init <name>` | Create profile (`--bare`, `--use`) |
| `profile path [name]` | Print profile home path |
| `profile doctor [name]` | Print paths and file existence |

See [Profiles](../concepts/profiles) and [Profiles and Config](../getting-started/profiles-and-config).

### `setup`: guided provider setup

| Invocation | Behavior |
| --- | --- |
| `morph setup provider` | Interactive provider and model setup |
| `morph setup provider ollama` | Start setup with Ollama selected |
| `morph setup provider --provider ollama --model <model>` | Non-interactive local provider setup |

Local provider flags:

| Flag | Description |
| --- | --- |
| `--provider` | Provider ID to persist, such as `ollama` |
| `--model` | Model ID to persist |
| `--base-url` | Provider endpoint, such as `http://127.0.0.1:11434` for native Ollama |
| `--api` | Provider API mode, such as `ollama-native` or `openai-completions` |
| `--pull` | Pull the selected Ollama model when missing |
| `--pull-quiet` | Suppress Ollama pull progress output |

See [Local Models](../guides/local-models) for the complete Ollama setup flow.

### `session`: sessions over RPC

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

### `trace`: trace viewer

| Subcommand | Flags | Description |
| --- | --- | --- |
| `trace view` | `--trace-dir`, `--listen`, `--username`, `--password` | Local HTTP UI for JSONL traces |

Default listen: `127.0.0.1:0`. Basic auth requires both `--username` and `--password`. See
[Search and Traces](../guides/search-and-traces).

### `version`

```bash
morph version
```

Prints version and commit hash.

## Quick command index

| Command | Purpose |
| --- | --- |
| *(default)* | TUI |
| `auth` | Provider login / status / logout |
| `automation` | Scheduled agent jobs, runs, delivery |
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
morph --profile work

# Start daemon
morph daemon
morph --profile work daemon

# One-shot chat
morph --chat "summarize the failing tests"
morph -c --session ses_abc123 --instruct "be brief" "continue"

# Local Ollama one-shot chat
morph --provider ollama \
  --model <model-id> \
  --base-url http://127.0.0.1:11434 \
  --pull \
  -c "hello"

# Local Ollama setup
morph setup provider \
  --provider ollama \
  --base-url http://127.0.0.1:11434 \
  --model <model-id> \
  --pull

# Config and doctor
morph config set session.maxIterations 30
morph doctor --json

# Gateway and sessions
morph gateway status
morph session list
MORPH_PROFILE=work morph session compact
```

## Where To Go Next

- [Slash Commands](./slash-commands): in-TUI `/` commands
- [Config Reference](./config): every config key and default
- [Local Models](../guides/local-models): local Ollama setup and diagnostics
- [Environment Variables](./environment-variables): `MORPH_*` overrides
- [RPC Reference](./rpc): gRPC services used by CLI clients
- [FAQ](./faq): common CLI questions

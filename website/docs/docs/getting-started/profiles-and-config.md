---
title: Profiles and Config
description: Understand profile homes, active profiles, and config precedence.
---

# Profiles and Config

Hand keeps configuration and state in profiles. A profile is a self-contained home directory that holds its own
config, credentials, sessions, search index, memory, and daemon runtime metadata. One machine can run several profiles
side by side without sharing any of that state.

This page explains where profiles live, how to manage them, how to read and change config, and how config sources
combine. For the conceptual model, see [Profiles](../concepts/profiles). For the setup walkthrough, see the
[Quickstart](./quickstart).

## What A Profile Is

A profile is a named directory under the machine-local Hand root. Everything Hand writes for that profile stays inside
it, so switching profiles switches your whole working context: a different model config, different credentials, and a
different set of saved sessions.

Profiles are useful when you want to separate work and personal usage, isolate a provider or model experiment, or run a
gateway profile that is configured differently from your interactive one.

## Where Profiles Live

The machine-local Hand root is `~/.hand`. Profiles live under it, and a small state file records which profile is
current:

```text
~/.hand/
  state.json                 # machine-local selector: current_profile
  profiles/
    default/                 # the default profile home
      config.yaml            # profile-local configuration
      .env                   # optional environment overrides
      auth.json              # credentials stored by hand auth login
      runtime.json           # daemon runtime metadata: RPC endpoint, pid, start time
      data/                  # SQLite store (state.db; WAL sidecars while in use)
      traces/                # on-disk trace files
    work/                    # another profile home
      ...
```

Print the exact home for the active profile, or for a named one:

```bash
hand profile path
hand profile path work
```

## Manage Profiles

### Create A Profile

`init` creates the profile home and writes a starter `config.yaml`. Add `--use` to also select it as the current
profile:

```bash
hand profile init work --use
```

If you never run `init`, Hand still falls back to the `default` profile and creates its home directory on demand, but it
will not write a `config.yaml` or store a current selection until you do. Use `--bare` if you want only the directory
without a `config.yaml`.

### Select The Current Profile

`use` sets the machine-local current profile. The profile must already exist:

```bash
hand profile use work
```

This writes `current_profile` to `~/.hand/state.json`. Every later `hand` command without an explicit `--profile` uses
that profile.

### Inspect Profiles

List existing profile directories, show the stored current profile, and print profile paths and file status:

```bash
hand profile list
hand profile current
hand profile doctor
hand profile doctor work
```

`hand profile doctor` prints the resolved name and the home, config, env, runtime, and pid paths, and reports whether
the home, config, env, and runtime files exist. Use it to confirm a profile is set up the way you expect. (The pid path
is reserved and is not written by the current daemon; the running daemon's pid is stored in `runtime.json`.)

### Select A Profile For One Command

You do not have to switch the current profile to use another one. Three mechanisms select a profile, in order of
precedence:

1. The `--profile` (`-p`) flag: `hand --profile work session list`.
2. The `HAND_PROFILE` environment variable: `HAND_PROFILE=work hand session list`.
3. The stored current profile from `hand profile use`.

When none of these is set, Hand uses `default`.

## Read And Change Config

Each profile has a `config.yaml`. You rarely edit it by hand. Use `hand config get` and `hand config set`, which read
and write the active profile's config file.

### Get Values

Pass one or more dotted key paths. A single key prints just the value; multiple keys print `path=value` lines:

```shell-session
$ hand config get models.main.name
gpt-5.5

$ hand config get models.main.provider models.main.name models.main.api
models.main.provider=openai-codex
models.main.name=gpt-5.5
models.main.api=openai-responses
```

### Set Values

Set accepts either `path value` pairs or `path=value` arguments, and reports the previous value:

```bash
hand config set models.main.name gpt-5.5
hand config set models.main.provider=openai-codex models.main.api=openai-responses
```

Both `get` and `set` accept `--profile` to target a profile other than the current one:

```bash
hand config set --profile work models.main.provider openrouter
```

`hand config set` writes to the profile `config.yaml`. A daemon running for that profile watches the file and restarts
automatically to apply the change; see [Profiles and the Daemon](#profiles-and-the-daemon).

For the full list of config keys and sections, see the [Config Reference](../reference/config). For practical config
recipes, see the [Config Guide](../guides/config).

## Config Precedence

When Hand starts, it builds the effective config by combining several sources. Later sources override earlier ones:

1. Built-in defaults.
2. The profile `config.yaml`.
3. Environment variables, including any loaded from the profile `.env` file.
4. Command-line flags.

So a flag always wins over an environment variable, which always wins over the config file, which wins over the
defaults. For example, with `models.main.provider: openrouter` in `config.yaml`:

```bash
# Environment override beats the config file.
HAND_MODEL_PROVIDER=openai-codex hand --chat "hello"

# A flag beats both the environment and the config file.
hand --model.provider anthropic --chat "hello"
```

Environment variables use the `HAND_` prefix and uppercase, underscore-separated names, for example
`HAND_MODEL_PROVIDER`, `HAND_MODEL` (the main model name), and `HAND_LOG_LEVEL`. They are useful for one-off runs,
secrets you do not want written to disk, and service environments. See the
[Environment Variables](../reference/environment-variables) reference for the supported variables.

Two flags select the files Hand reads instead of the profile defaults:

- `--config <path>` reads base settings from a specific YAML file.
- `--env-file <path>` loads environment overrides from a specific `.env` file.

`hand config set` only writes to the profile `config.yaml`; it never writes to the `.env` file. Both `get` and `set` do
read the profile `.env` so that values reflect any environment overrides, but they do not apply command-line override
flags. To confirm what a running command will actually use, prefer `hand doctor` and check the daemonup output.

## Profiles and the Daemon

Each profile has its own daemon runtime metadata in `runtime.json`, which records the RPC endpoint the CLI and TUI
connect to. For how that endpoint is resolved and how clients talk to the daemon, see
[Daemon and RPC](../concepts/daemon-and-rpc). This has a few practical consequences:

- `hand profile use` only changes which profile new commands target. A daemon already running for another profile keeps
  running and is unaffected.
- The daemon watches its profile `config.yaml` and automatically restarts the runtime when the file changes with a
  valid config. Editing `config.yaml` or running `hand config set` is therefore picked up automatically after a brief
  debounce; you do not need to restart manually. An invalid change is logged and ignored, and the daemon keeps running
  the previous config.
- The profile `.env` is not watched. Changes to environment overrides still require a manual restart.
- Commands resolve the profile first, then connect to that profile's runtime endpoint. Interactive `hand` and
  `hand --chat` can start a temporary daemon when none is reachable; other client commands such as `hand session ...`
  and `hand gateway ...` expect a daemon to already be running. The only command that launches a daemon directly is
  `hand daemon`.

To apply an `.env` change, restart the profile's daemon. Stop the running daemon first: press `Ctrl+C` if you started
it in the foreground with `hand daemon`, or exit the TUI that started it. Then start it again:

```bash
hand --profile work daemon
```

For daemon lifecycle details, see [Daemon Operations](../operations/daemon).

## Verify

After changing profiles or config, confirm the active setup:

```bash
hand profile current
hand config get models.main.provider models.main.name models.main.api
hand doctor
```

`hand doctor` exits cleanly when the selected profile is ready. If it reports a problem, recheck the provider routing
keys above and confirm credentials with `hand auth status`.

## Where To Go Next

- [Profiles](../concepts/profiles): the conceptual model behind profile homes.
- [Config Guide](../guides/config): practical config changes by section.
- [Config Reference](../reference/config): every config key.
- [Environment Variables](../reference/environment-variables): supported `HAND_` overrides.
- [Doctor](../operations/doctor): readiness checks and diagnostics.
- [Backups and State](../operations/backups-and-state): back up or move a profile home.

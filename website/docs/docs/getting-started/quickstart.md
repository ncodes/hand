---
title: Quickstart
description: Get Hand running and start your first conversation.
---

# Quickstart

This guide takes you from installation to a working Hand conversation. It keeps the setup local: one profile, one model provider credential, the daemon, and either the TUI or a one-shot prompt.

## Requirements

Before you start, make sure you have:

- A model provider credential.
- A terminal on the machine where you want Hand to run.

Hand can use several model providers. For a first run, OpenRouter or OpenAI is usually the shortest path because both work with the OpenAI-compatible request flow.

## Install Hand

The preferred installation path is the hosted installer:

```bash
curl -fsSL https://handagent.ai/install.sh | bash
```

After installation, verify the CLI is available:

```bash
hand version
```

If your shell cannot find `hand`, restart the shell or make sure the install directory printed by the installer is on your `PATH`.

Once `hand version` works, you can start the terminal UI and follow the setup prompts:

```bash
hand
```

The sections below show the same setup steps explicitly for source builds, scripted setup, or users who prefer to configure Hand from commands.

## Build From Source

Use this path when you are contributing to Hand, testing local changes, or prefer to build your tools from source.

Before running the Makefile targets, install:

- Go `1.26.1`.
- `make`.
- A C compiler toolchain with CGO support for the SQLite-backed runtime.

From the repository root:

```bash
make build
```

The compiled binary is written to `build/hand`.

You can also install the local source build into your Go binary directory:

```bash
make install
```

If you did not run `make install`, replace `hand` in the examples below with `./build/hand`.

## Create A Profile

Profiles keep config, credentials, runtime metadata, sessions, search state, and memory separate from other Hand setups.

Create a profile named `default`:

```bash
hand profile init default
hand profile use default
```

Check the active profile:

```bash
hand profile current
```

By default, the profile lives under:

```text
~/.hand/profiles/default
```

The important files are:

- `config.yaml`: profile-local configuration.
- `.env`: optional environment overrides.
- `auth.json`: credentials stored by `hand auth login`.
- `runtime.json`: daemon runtime metadata.

## Choose A Model

Set the main model provider and model in the profile config. This example uses OpenRouter:

```bash
hand config set models.main.provider openrouter
hand config set models.main.name openai/gpt-4o-mini
hand config set models.main.api openai-responses
```

For OpenAI directly:

```bash
hand config set models.main.provider openai
hand config set models.main.name gpt-4o-mini
hand config set models.main.api openai-responses
```

You can inspect the current values at any time:

```bash
hand config get models.main.provider models.main.name models.main.api
```

## Store Credentials

Store your model credential in the active profile. Do not paste real credentials into docs, tickets, or shared config.

For OpenRouter:

```bash
hand auth login openrouter --api-key "<openrouter-api-key>"
```

For OpenAI:

```bash
hand auth login openai --api-key "<openai-api-key>"
```

Verify what Hand can see:

```bash
hand auth status openrouter
hand doctor
```

If you configured OpenAI instead, run:

```bash
hand auth status openai
hand doctor
```

## Start The Daemon

Hand's long-lived runtime runs in the daemon. The TUI, one-shot chat requests, session commands, and gateway management commands talk to it over local RPC.

Start it in one terminal:

```bash
hand daemon start
```

You should see logs showing that configuration loaded, the agent started, and the RPC server is listening.

Keep this process running while you use Hand. In another terminal, confirm readiness:

```bash
hand doctor
```

## Start Your First Chat

For the terminal UI, run:

```bash
hand
```

Type a message and press Enter.

For a one-shot request, run:

```bash
hand --chat "Say hello in one sentence."
```

You can also select the profile explicitly:

```bash
hand --profile default --chat "Summarize what you can do."
```

## Check Sessions

Hand persists conversation state. After your first chat:

```bash
hand session current
hand session list
```

To continue in a specific session:

```bash
hand --chat --session "<session-id>" "Continue from there."
```

## First-Run Troubleshooting

### `hand` Cannot Find A Model Credential

Run:

```bash
hand auth status
hand config get models.main.provider models.main.name
```

Make sure the provider in config matches the provider you logged into with `hand auth login`.

### The Daemon Is Not Reachable

Start the daemon in a separate terminal:

```bash
hand daemon start
```

Then retry:

```bash
hand doctor
hand --chat "hello"
```

### SQLite Or Search Tests Fail With `fts5`

For development and validation, use the Makefile targets. They set the SQLite FTS5 build tag:

```bash
make test
```

For a focused package test, mirror the same flags:

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./cmd/hand
```

## Where To Go Next

- [Installation](./installation): build and install paths.
- [First Chat](./first-chat): the conversation workflow in more detail.
- [Profiles and Config](./profiles-and-config): profile-local setup and config precedence.
- [Model Auth](../guides/model-auth): provider credentials and model roles.
- [TUI Guide](../guides/tui): daily terminal usage.
- [Doctor](../operations/doctor): readiness checks and diagnostics.

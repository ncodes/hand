---
title: Quickstart
description: Get Hand running and start your first conversation.
---

# Quickstart

This guide gets Hand installed, configured, and ready for your first chat.

## Requirements

You need:

- A model provider account or API key.
- A terminal on the machine where you want Hand to run.

Hand can store subscription accounts or API keys. If you already pay for ChatGPT, Claude, or GitHub Copilot,
subscription login is often fastest. Hand stores OAuth credentials for supported providers.

API keys are also supported. They are often better for OpenRouter, servers, service accounts, and automation.

See [Model Auth](../guides/model-auth) for provider notes and API key links.

## Install Hand

Install Hand with the hosted installer:

```bash
curl -fsSL https://handagent.ai/install.sh | bash
```

After installation, verify the CLI is available:

```bash
hand version
```

If your shell cannot find `hand`, restart the shell or check that the install directory is on your `PATH`.
For source builds, platform notes, and update or uninstall guidance, see [Installation](./installation).

Once `hand version` works, start the terminal UI:

```bash
hand
```

You can finish setup there and start chatting. See the [TUI Guide](../guides/tui) to learn more about Hand's TUI.

The rest of this page shows the same setup with commands.

## Create A Profile

Profiles keep config, credentials, sessions, search state, and memory separate.

The first run of `hand` usually creates and selects `default`. To do it yourself:

```bash
hand profile init default --use
```

Check it:

```bash
hand profile current
```

Print the exact profile path:

```bash
hand profile path
```

Common profile files:

- `config.yaml`: profile-local configuration.
- `.env`: optional environment overrides.
- `auth.json`: credentials stored by `hand auth login`.
- `runtime.json`: daemon runtime metadata.

## Choose A Provider

First choose how Hand should route model requests. These commands set the provider, model name, and API shape in the
active profile; they do not authenticate the provider yet.

Replace `<model-name>` with a [model supported by the provider you choose](../guides/model-auth). After this section,
continue to [Store Credentials](#store-credentials) and authenticate the same provider.

Subscription login is usually quickest if you already pay for ChatGPT, Claude, or GitHub Copilot. Providers like
OpenRouter require an API key. API keys also fit servers, automation, and team-managed credentials.

### Subscription Login

Use one of these when you want to authenticate later with OAuth.

Use OpenAI Codex if you want to sign in with an OpenAI subscription:

```bash
hand config set models.main.provider openai-codex
hand config set models.main.name <model-name>
hand config set models.main.api openai-responses
```

Anthropic for Claude models:

```bash
hand config set models.main.provider anthropic
hand config set models.main.name <model-name>
hand config set models.main.api anthropic-messages
```

GitHub Copilot uses your Copilot subscription:

```bash
hand config set models.main.provider github-copilot
hand config set models.main.name <model-name>
hand config set models.main.api openai-completions
```

### API Key

Use one of these when you want to authenticate later with an API key.

OpenRouter gives one API key to access many hosted models:

```bash
hand config set models.main.provider openrouter
hand config set models.main.name <model-name>
hand config set models.main.api openai-responses
```

For OpenAI, you'll need to provide an OpenAI API key:

```bash
hand config set models.main.provider openai
hand config set models.main.name <model-name>
hand config set models.main.api openai-responses
```

To use Anthropic's models, you'll need an Anthropic API key:

```bash
hand config set models.main.provider anthropic
hand config set models.main.name <model-name>
hand config set models.main.api anthropic-messages
```

Check the routing config before adding credentials:

```bash
hand config get models.main.provider models.main.name models.main.api
```

To learn about credentials and available models for each provider, see the [Model Auth](../guides/model-auth) guide.

## Store Credentials

Now authenticate the provider you selected in [Choose A Provider](#choose-a-provider). Hand cannot work until the active
profile has credentials for `models.main.provider`.

Store credentials in the active profile. Do not put real keys in shared docs, tickets, or config.

For subscription login, run only the command for your configured provider and omit `--api-key`:

```bash
hand auth login openai-codex
```

For Claude subscription login:

```bash
hand auth login anthropic
```

For GitHub Copilot:

```bash
hand auth login github-copilot
```

For API key auth, run only the command for your configured provider:

OpenRouter:

```bash
hand auth login openrouter --api-key "<openrouter-api-key>"
```

OpenAI:

```bash
hand auth login openai --api-key "<openai-api-key>"
```

Anthropic:

```bash
hand auth login anthropic --api-key "<anthropic-api-key>"
```

Verify:

```bash
hand auth status
hand doctor
```

If credentials are missing, check that `models.main.provider` matches the provider you logged into.

## Start Your First Chat

Open the TUI:

```bash
hand
```

If the daemon is not running at the expected address, `hand` starts a temporary daemon in the same process.

Type a message and press Enter.

Or send one message:

```bash
hand --chat "Say hello in one sentence."
```

Select a profile explicitly:

```bash
hand --profile default --chat "Summarize what you can do."
```

## Check The Daemon

For manual daemon control or readiness checks:

```bash
hand doctor
hand gateway status
```

If you want to run the daemon in the foreground or don't want a TUI-managed daemon, you can run:

```bash
hand daemon start
```

then start the TUI:

```bash
hand
```

## Check Sessions

Hand saves chats as sessions. Use them to find recent conversations or continue work later.

Show the active session:

```bash
hand session current
```

List recent sessions:

```bash
hand session list
```

Continue a specific session from a one-shot prompt:

```bash
hand --chat --session "<session-id>" "Continue from there."
```

For daily use, the TUI can continue from the current session automatically.

## Where To Go Next

- [Installation](./installation): build and install paths.
- [First Chat](./first-chat): the conversation workflow in more detail.
- [Profiles and Config](./profiles-and-config): profile-local setup and config precedence.
- [Model Auth](../guides/model-auth): provider credentials and model roles.
- [TUI Guide](../guides/tui): daily terminal usage.
- [Doctor](../operations/doctor): readiness checks and diagnostics.

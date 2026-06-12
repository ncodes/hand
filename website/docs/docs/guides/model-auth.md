---
title: Model Auth
description: Configure provider credentials and model choices.
---

# Model Auth

Hand can authenticate model providers with subscription OAuth credentials or static API keys. Prefer subscription
login when you are setting up a personal workstation and already have a paid OpenAI, Claude, or GitHub Copilot
account. Prefer API keys for OpenRouter, service accounts, servers, CI, shared deployments, and automation.

Credentials are profile-local. By default, `hand auth login` stores them in:

```text
~/.hand/profiles/<profile>/auth.json
```

Do not commit `auth.json`, `.env`, or real provider tokens.

## Subscription Login

Subscription login stores an OAuth credential. Run the login command without `--api-key`:

```bash
hand auth login openai-codex
hand auth login anthropic
hand auth login github-copilot
```

Use the provider you selected in config:

```bash
hand config set models.main.provider openai-codex
hand config set models.main.name gpt-5.5
hand config set models.main.api openai-responses
```

Supported subscription providers:

- `openai-codex`: OpenAI account / ChatGPT subscription flow for Codex models.
- `anthropic`: Claude account subscription flow for OAuth-capable Claude models.
- `github-copilot`: GitHub Copilot subscription device flow.

Some models are API-key-only even when the provider supports subscription login. If a selected model is not available
through OAuth, choose an OAuth-capable model or use API key auth for that provider.

## API Key Login

API key login stores a static provider key:

```bash
hand auth login openrouter --api-key "<openrouter-api-key>"
hand auth login openai --api-key "<openai-api-key>"
hand auth login anthropic --api-key "<anthropic-api-key>"
```

Provider API key pages:

- OpenRouter: `https://openrouter.ai/settings/keys`
- OpenAI: `https://platform.openai.com/api-keys`
- Anthropic: `https://console.anthropic.com/settings/keys`
- GitHub Copilot: use subscription login unless you already have a compatible Copilot token.

## Check Credential Status

Show every provider Hand can see:

```bash
hand auth status
```

Check one provider:

```bash
hand auth status openai-codex
```

Remove a stored credential:

```bash
hand auth logout openai-codex
```

## How Hand Resolves Credentials

For model requests, Hand resolves credentials in this order:

1. Role-specific config such as `models.main.apiKey`.
2. Provider config such as `models.providers.openai.apiKey`.
3. Environment variables recognized by the provider, such as `OPENAI_API_KEY` or `ANTHROPIC_API_KEY`.
4. Stored credentials from `hand auth login`.

Run `hand doctor` after changing model config or credentials. It checks whether the active profile can resolve the
main model credential and explains the next command to run when something is missing.

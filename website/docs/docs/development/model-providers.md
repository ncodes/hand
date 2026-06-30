---
title: Model Providers
description: Add and maintain provider integrations.
displayed_sidebar: null
---

# Model Providers

This page should document provider runtime internals.

## Provider Registry

Morph routes hosted and local providers through the same provider registry. A provider definition describes the provider
ID, display name, supported API modes, default base URLs, auth behavior, and local-provider metadata when applicable.
Model definitions then attach capabilities such as text, vision, tool support, reasoning, context window, max output,
OAuth support, and display defaults.

Local providers use the same registry path so `/models`, `/providers`, setup, doctor, and runtime session metadata do
not need one-off local-only branches.

## Local Provider Catalog Flow

The model catalog combines several sources:

1. **Registry catalog** — built-in suggested models.
2. **Explicit profile config** — user-pinned provider model definitions. Explicit config wins and disables discovery for
   that provider.
3. **Runtime discovery** — for Ollama, Morph queries `GET /api/tags` and `POST /api/show`.
4. **Short-lived cache** — local discovery results are cached briefly and refreshed manually by setup surfaces.

For Ollama, installed models from discovery are shown before suggested catalog models. Suggested models that are not
installed are marked as missing and can be pulled from setup flows.

## API Modes

Current generation API modes include:

- `ollama-native` for native Ollama `/api/chat`.
- `openai-completions` for OpenAI-compatible `/v1/chat/completions`.
- `openai-responses` for OpenAI Responses-compatible providers.
- `anthropic-messages` for Anthropic Messages.

Native Ollama is preferred for Ollama because it can use Ollama-specific request options, streaming shape, and tool-call
behavior directly. Ollama OpenAI-compatible mode remains selectable for proxies or deployments that expose only `/v1`.
Context sizing for Ollama OpenAI-compatible mode should be handled with an Ollama `Modelfile` `PARAMETER num_ctx`, not
by assuming an OpenAI-compatible request field exists.

## Auth Resolution

Local providers can define a non-secret auth marker. That marker lets Morph pass through the same credential resolution
pipeline used by hosted providers without requiring a fake API key or sending an Authorization header to a local runtime.

Hosted providers still resolve role config, stored credentials, environment variables, and provider config as described
in the user-facing [Provider Auth](../guides/provider-auth) guide.

## Model Roles

- Provider config.
- Main, summary, embedding, and reranker clients.
- Adding a provider.

Main, summary, embedding, and reranker roles are configured independently. Local chat support does not imply local
embedding support; Ollama supports both native chat and `/api/embeddings`.

## Local Provider Implementation Notes

For local provider implementations:

- Add a provider definition with local metadata, default base URL, auth marker behavior, and supported API modes.
- Add discovery only when the provider has a reliable model listing endpoint.
- Keep explicit config as the override for discovery.
- Add doctor checks that distinguish reachability, empty model list, selected model missing, and endpoint-shape mistakes.
- Avoid claiming support in the website until setup, diagnostics, and runtime behavior are implemented.

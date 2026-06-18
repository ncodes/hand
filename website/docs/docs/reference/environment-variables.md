---
title: Environment Variables
description: Environment variable reference for Hand profiles.
---

# Environment Variables

Hand profiles load configuration from **`config.yaml`**, optional **`.env`**, and **environment variables**. The `.env`
file is preloaded into the process environment; then `HAND_*` overrides apply after YAML load and before normalization.

Practical workflows: [Profiles and Config](../getting-started/profiles-and-config),
[Config Guide](../guides/config). Full key semantics: [Config Reference](./config). CLI flags that mirror env vars:
[CLI Reference](./cli#global-flags).

## Precedence

```text
config.yaml  →  HAND_* overrides  →  Normalize()  →  effective config
```

Profile selection uses **`HAND_PROFILE`** or `--profile` / `-p` (not a config key). Config path overrides:
**`HAND_CONFIG`**, **`HAND_ENV_FILE`**.

Provider credential env vars such as `OPENAI_API_KEY` do not rewrite config fields. They are resolved later, when Hand
needs credentials for a model or web provider.

## Value formats

| Type | Format |
| --- | --- |
| Lists | Comma-separated (`HAND_FS_ROOTS=/tmp,/home/user/proj`) |
| Durations | Go duration syntax (`24h`, `720h`, `10s`) |
| Booleans | For most `HAND_*` keys: **`1`**, **`true`**, or **`yes`** → true; any other non-empty value, including `false` or `no`, → false |
| JSON | `HAND_RERANKER_OVERRIDES` — JSON object matching `reranker.overrides` |

Invalid numeric or duration env values are **ignored** (YAML value kept).

:::warning[Bool env overrides replace YAML]
Setting a boolean env var to any non-empty non-true value overrides YAML to `false`. Unset the variable, or remove it
from `.env`, when you want the YAML/default value to win. `HAND_LOG_NO_COLOR`, `HAND_DEBUG_REQUESTS`, and
`HAND_TRACE_ENABLED` use the same true tokens but assign concrete bool fields instead of optional bool pointers.
:::

## Profile and platform

| Variable | Config key / effect |
| --- | --- |
| `HAND_PROFILE` | Selects active profile (not a config key) |
| `HAND_CONFIG` | Overrides config YAML path |
| `HAND_ENV_FILE` | Overrides `.env` path |
| `HAND_NAME` | `name` |
| `HAND_PLATFORM` | `platform` |

## Models

| Variable | Config key |
| --- | --- |
| `HAND_MODEL` | `models.main.name` |
| `HAND_MODEL_PROVIDER` | `models.main.provider` |
| `HAND_MODEL_API` | `models.main.api` |
| `HAND_MODEL_BASE_URL` | `models.main.baseUrl` |
| `HAND_MODEL_STREAM` | `models.main.stream` |
| `HAND_MODEL_CONTEXT_LENGTH` | `models.main.contextLength` |
| `HAND_MODEL_MAX_RETRIES` | `models.maxRetries` |
| `HAND_MODEL_SUMMARY` | `models.summary.name` |
| `HAND_MODEL_SUMMARY_PROVIDER` | `models.summary.provider` |
| `HAND_MODEL_SUMMARY_API` | `models.summary.api` |
| `HAND_MODEL_SUMMARY_BASE_URL` | `models.summary.baseUrl` |
| `HAND_MODEL_EMBEDDING_PROVIDER` | `models.embedding.provider` |
| `HAND_MODEL_EMBEDDING_MODEL` | `models.embedding.name` |

Role `apiKey` fields and `models.providers.*` are **not** overridden by `HAND_*` — use YAML, `hand auth login`, or
provider env vars below.

## RPC

| Variable | Config key |
| --- | --- |
| `HAND_RPC_ADDRESS` | `rpc.address` |
| `HAND_RPC_PORT` | `rpc.port` |

## Gateway

| Variable | Config key |
| --- | --- |
| `HAND_GATEWAY_ENABLED` | `gateway.enabled` |
| `HAND_GATEWAY_ADDRESS` | `gateway.address` |
| `HAND_GATEWAY_PORT` | `gateway.port` |
| `HAND_GATEWAY_AUTH_TOKEN` | `gateway.authToken` |
| `HAND_GATEWAY_PAIRING_SECRET` | `gateway.pairingSecret` |
| `HAND_GATEWAY_ALLOWED_USERS` | `gateway.allowedUsers` |
| `HAND_GATEWAY_TELEGRAM_ENABLED` | `gateway.telegram.enabled` |
| `HAND_GATEWAY_TELEGRAM_MODE` | `gateway.telegram.mode` |
| `HAND_GATEWAY_TELEGRAM_BOT_TOKEN` | `gateway.telegram.botToken` |
| `HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET` | `gateway.telegram.webhookSecret` |
| `HAND_GATEWAY_TELEGRAM_ALLOWED_USERS` | `gateway.telegram.allowedUsers` |
| `HAND_GATEWAY_SLACK_ENABLED` | `gateway.slack.enabled` |
| `HAND_GATEWAY_SLACK_MODE` | `gateway.slack.mode` |
| `HAND_GATEWAY_SLACK_RESPONSE_MODE` | `gateway.slack.responseMode` |
| `HAND_GATEWAY_SLACK_BOT_TOKEN` | `gateway.slack.botToken` |
| `HAND_GATEWAY_SLACK_APP_TOKEN` | `gateway.slack.appToken` |
| `HAND_GATEWAY_SLACK_SIGNING_SECRET` | `gateway.slack.signingSecret` |
| `HAND_GATEWAY_SLACK_ALLOWED_USERS` | `gateway.slack.allowedUsers` |

## Session

| Variable | Config key |
| --- | --- |
| `HAND_SESSION_MAX_ITERATIONS` | `session.maxIterations` |
| `HAND_SESSION_INSTRUCT` | `session.instruct` |
| `HAND_SESSION_DEFAULT_IDLE_EXPIRY` | `session.defaultIdleExpiry` |
| `HAND_SESSION_ARCHIVE_RETENTION` | `session.archiveRetention` |

## Capabilities, filesystem, exec, storage

| Variable | Config key |
| --- | --- |
| `HAND_CAP_FS` | `cap.fs` |
| `HAND_CAP_NET` | `cap.net` |
| `HAND_CAP_EXEC` | `cap.exec` |
| `HAND_CAP_MEM` | `cap.mem` |
| `HAND_CAP_BROWSER` | `cap.browser` |
| `HAND_FS_ROOTS` | `fs.roots` |
| `HAND_EXEC_ALLOW` | `exec.allow` |
| `HAND_EXEC_ASK` | `exec.ask` |
| `HAND_EXEC_DENY` | `exec.deny` |
| `HAND_STORAGE_BACKEND` | `storage.backend` |
| `HAND_RULES_FILES` | `rules.files` |

No `HAND_*` override exists for `fs.noProfileAccess` or `compaction.recentSessionTail`.

## Search and reranker

| Variable | Config key |
| --- | --- |
| `HAND_SEARCH_ENABLE_RERANK` | `search.enableRerank` |
| `HAND_SEARCH_VECTOR_ENABLED` | `search.vector.enabled` |
| `HAND_SEARCH_VECTOR_REQUIRED` | `search.vector.required` |
| `HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE` | `search.vector.rebuildBatchSize` |
| `HAND_RERANKER_ENABLED` | `reranker.enabled` |
| `HAND_RERANKER_TYPE` | `reranker.type` |
| `HAND_RERANKER_MODEL` | `reranker.model` |
| `HAND_RERANKER_MAX_CANDIDATES` | `reranker.maxCandidates` |
| `HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS` | `reranker.maxCandidateTextChars` |
| `HAND_RERANKER_MAX_OUTPUT_TOKENS` | `reranker.maxOutputTokens` |
| `HAND_RERANKER_OVERRIDES` | `reranker.overrides` (JSON) |

## Compaction

| Variable | Config key |
| --- | --- |
| `HAND_COMPACTION_ENABLED` | `compaction.enabled` |
| `HAND_COMPACTION_TRIGGER_PERCENT` | `compaction.triggerPercent` |
| `HAND_COMPACTION_WARN_PERCENT` | `compaction.warnPercent` |

## Memory

| Variable | Config key |
| --- | --- |
| `HAND_MEMORY_ENABLED` | `memory.enabled` |
| `HAND_MEMORY_PROVIDER` | `memory.provider` |
| `HAND_MEMORY_BACKEND` | `memory.backend` |
| `HAND_MEMORY_PINNED_ENABLED` | `memory.pinned.enabled` |
| `HAND_MEMORY_PINNED_MAX_CHARS` | `memory.pinned.maxChars` |
| `HAND_MEMORY_PINNED_MAX_ITEM_CHARS` | `memory.pinned.maxItemChars` |
| `HAND_MEMORY_RETRIEVAL_ENABLED` | `memory.retrieval.enabled` |
| `HAND_MEMORY_FLUSH_ENABLED` | `memory.flush.enabled` |
| `HAND_MEMORY_FLUSH_MAX_CALLS` | `memory.flush.maxCalls` |
| `HAND_MEMORY_FLUSH_MAX_OUTPUT_TOKENS` | `memory.flush.maxOutputTokens` |
| `HAND_MEMORY_FLUSH_TIMEOUT` | `memory.flush.timeout` |
| `HAND_MEMORY_WRITE_ENABLED` | `memory.write.enabled` |
| `HAND_MEMORY_EPISODIC_ENABLED` | `memory.episodic.enabled` |
| `HAND_MEMORY_EPISODIC_INTERVAL` | `memory.episodic.interval` |
| `HAND_MEMORY_EPISODIC_IDLE_AFTER` | `memory.episodic.idleAfter` |
| `HAND_MEMORY_EPISODIC_MIN_MESSAGES` | `memory.episodic.minMessages` |
| `HAND_MEMORY_EPISODIC_WINDOW_SIZE` | `memory.episodic.windowSize` |
| `HAND_MEMORY_EPISODIC_MAX_WINDOWS` | `memory.episodic.maxWindows` |
| `HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS` | `memory.episodic.maxWindowChars` |
| `HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS` | `memory.episodic.maxWindowTokens` |
| `HAND_MEMORY_EPISODIC_MAX_RETRIES` | `memory.episodic.maxRetries` |
| `HAND_MEMORY_REFLECTION_ENABLED` | `memory.reflection.enabled` |
| `HAND_MEMORY_REFLECTION_INTERVAL` | `memory.reflection.interval` |
| `HAND_MEMORY_REFLECTION_LIMIT` | `memory.reflection.limit` |
| `HAND_MEMORY_REFLECTION_RELATED_LIMIT` | `memory.reflection.relatedLimit` |
| `HAND_MEMORY_PROMOTION_ENABLED` | `memory.promotion.enabled` |
| `HAND_MEMORY_PROMOTION_INTERVAL` | `memory.promotion.interval` |
| `HAND_MEMORY_PROMOTION_LIMIT` | `memory.promotion.limit` |

## Web

| Variable | Config key |
| --- | --- |
| `HAND_WEB_PROVIDER` | `web.provider` |
| `HAND_WEB_API_KEY` | `web.apiKey` |
| `HAND_WEB_BASE_URL` | `web.baseUrl` |
| `HAND_WEB_MAX_CHAR_PER_RESULT` | `web.maxCharPerResult` |
| `HAND_WEB_MAX_EXTRACT_CHAR_PER_RESULT` | `web.maxExtractCharPerResult` |
| `HAND_WEB_MAX_EXTRACT_RESPONSE_BYTES` | `web.maxExtractResponseBytes` |
| `HAND_WEB_CACHE_TTL` | `web.cacheTTL` |
| `HAND_WEB_BLOCKED_DOMAINS_ENABLED` | `web.blockedDomains.enabled` |
| `HAND_WEB_BLOCKED_DOMAINS` | `web.blockedDomains.domains` |
| `HAND_WEB_BLOCKED_DOMAIN_FILES` | `web.blockedDomains.files` |
| `HAND_WEB_NATIVE_ALLOWED_HOSTS` | `web.native.allowedHosts` |
| `HAND_WEB_NATIVE_BLOCKED_HOSTS` | `web.native.blockedHosts` |
| `HAND_WEB_NATIVE_ALLOWED_HOST_FILES` | `web.native.allowedHostFiles` |
| `HAND_WEB_NATIVE_BLOCKED_HOST_FILES` | `web.native.blockedHostFiles` |
| `HAND_WEB_EXTRACT_MIN_SUMMARIZE_CHARS` | `web.extractMinSummarizeChars` |
| `HAND_WEB_EXTRACT_MAX_SUMMARY_CHARS` | `web.extractMaxSummaryChars` |
| `HAND_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS` | `web.extractMaxSummaryChunkChars` |
| `HAND_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS` | `web.extractRefusalThresholdChars` |

### Web provider auto-detection

When `web.provider` / `web.apiKey` are empty, these variables also **select** a provider:

| Variable | Selects |
| --- | --- |
| `HAND_FIRECRAWL_API_KEY` or `HAND_FIRECRAWL_API_URL` | `firecrawl` |
| `HAND_PARALLEL_API_KEY` | `parallel` |
| `HAND_TAVILY_API_KEY` | `tavily` |
| `HAND_EXA_API_KEY` | `exa` |

Order: firecrawl → parallel → tavily → exa.

## Logging, debug, trace, TUI, safety

| Variable | Config key |
| --- | --- |
| `HAND_LOG_LEVEL` | `log.level` |
| `HAND_LOG_FILE` | `log.file` |
| `HAND_LOG_MAX_SIZE_MB` | `log.maxSizeMB` |
| `HAND_LOG_MAX_BACKUPS` | `log.maxBackups` |
| `HAND_LOG_MAX_AGE_DAYS` | `log.maxAgeDays` |
| `HAND_LOG_COMPRESS` | `log.compress` |
| `HAND_LOG_NO_COLOR` | `log.noColor` |
| `HAND_DEBUG_REQUESTS` | `debug.requests` |
| `HAND_TRACE_ENABLED` | `trace.enabled` |
| `HAND_TRACE_DISK_ENABLED` | `trace.disk.enabled` |
| `HAND_TRACE_DISK_DIR` | `trace.disk.dir` |
| `HAND_TRACE_DATABASE_ENABLED` | `trace.database.enabled` |
| `HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION` | `trace.database.maxEventsPerSession` |
| `HAND_TUI_THINKING_COMPOSER` | `tui.thinkingComposer` |
| `HAND_SAFETY_INPUT` | `safety.input` |
| `HAND_SAFETY_OUTPUT` | `safety.output` |
| `HAND_SAFETY_PII` | `safety.pii` |

## Model provider credentials (non-`HAND_*`)

Resolved at runtime for model calls. These variables do not rewrite `config.yaml` fields.

| Provider | Default env var(s) |
| --- | --- |
| `openrouter` | `OPENROUTER_API_KEY` |
| `openai` | `OPENAI_API_KEY` |
| `anthropic` | `ANTHROPIC_API_KEY`, OAuth: `ANTHROPIC_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN` |
| `github-copilot` | `COPILOT_GITHUB_TOKEN` |
| `openai-codex` | OAuth token store only |

Custom names via YAML:

```yaml
models:
  providers:
    openrouter:
      apiKeyEnv:
        - MY_OPENROUTER_KEY
```

Resolution order: role `apiKey` in YAML → stored token → OAuth env → configured `apiKeyEnv` list → provider default
env vars → `models.providers.<p>.apiKey`.

## Web provider credentials

| Provider | Env vars (first non-empty wins) |
| --- | --- |
| `firecrawl` | `HAND_FIRECRAWL_API_KEY`, `FIRECRAWL_API_KEY`, `HAND_WEB_API_KEY` |
| `parallel` | `HAND_PARALLEL_API_KEY`, `PARALLEL_API_KEY`, `HAND_WEB_API_KEY` |
| `tavily` | `HAND_TAVILY_API_KEY`, `TAVILY_API_KEY`, `HAND_WEB_API_KEY` |
| `exa` | `HAND_EXA_API_KEY`, `EXA_API_KEY`, `HAND_WEB_API_KEY` |

Firecrawl base URL: `HAND_FIRECRAWL_API_URL`. Details: [Provider Auth](../guides/provider-auth).

## Secret handling

- Prefer **`hand auth login`** or profile `auth.json` over long-lived shell exports for OAuth and API keys.
- Do not commit `.env` files with secrets. See [Security](../operations/security).
- Gateway tokens in env (`HAND_GATEWAY_*`) are as sensitive as YAML values — restrict file permissions on profile home.

## Keys without env overrides

These config paths are YAML / `hand config set` only:

- `personalities.*`
- `models.providers.*.apiKey` (use provider env or auth store)
- `compaction.recentSessionTail`
- `fs.noProfileAccess`

## Where To Go Next

- [Config Reference](./config): defaults and validation rules
- [Config Guide](../guides/config): common changes
- [Provider Auth](../guides/provider-auth): model and web credentials
- [CLI Reference](./cli): flags mirroring env vars
- [Security](../operations/security): protecting secrets and listeners
- [FAQ](./faq): profile and env troubleshooting

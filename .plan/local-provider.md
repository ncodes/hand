# Local Model Provider Support Plan

## Goal

Add first-class local model provider support that feels native in Morph, beginning with Ollama and leaving a clean provider surface for vLLM, SGLang, LiteLLM, LM Studio, llama.cpp, and other OpenAI-compatible local runtimes.

The first milestone is a reliable Ollama path that can be configured, discovered, selected, used for agent chat, and diagnosed without requiring users to understand provider internals.

## Guiding Principles

- Local providers should use the same model selection, status, routing, telemetry, and session metadata surfaces as hosted providers.
- Ollama should use its native API where that improves reliability, especially for streaming and tool calling.
- OpenAI-compatible local runtimes should share one adapter path instead of creating one-off provider implementations.
- Local providers should not require real secrets when the runtime does not enforce auth.
- Discovery must be best effort and non-blocking; explicit config should always win.
- Failures should produce actionable setup hints, not generic model/provider errors.

## Phase 0 - Provider Architecture Shape

- [x] Define a `LocalProvider` capability model that can represent:
  - [x] Native Ollama chat API.
  - [x] OpenAI-compatible chat completions.
  - [x] OpenAI-compatible responses if needed later.
  - [x] Embeddings support, separately from chat support.
  - [x] Vision/tool-calling capability flags.
- [x] Decide where local provider config lives:
  - [x] Global config defaults.
  - [x] Profile-specific overrides.
  - [x] Runtime/session overrides.
- [x] Add a normalized provider id and model ref format:
  - [x] `ollama/<model>`.
  - [x] `vllm/<model>`.
  - [x] `sglang/<model>`.
  - [x] `custom-local/<model>` or configured custom provider ids.
- [x] Define a non-secret local auth marker, for example `ollama-local`, so SDK/client setup can proceed without treating the marker as a real secret.
- [x] Document which settings are provider-level versus model-level:
  - [x] `base_url`.
  - [x] `api_mode` or adapter type.
  - [x] `headers`.
  - [x] `context_window`.
  - [x] `max_output_tokens`.
  - [x] tool/vision/reasoning support.

## Phase 1 - Ollama MVP

- [ ] Add an Ollama provider entry with defaults:
  - [ ] Provider id: `ollama`.
  - [ ] Default base URL: `http://127.0.0.1:11434`.
  - [ ] Default auth marker: `ollama-local`.
  - [ ] Default suggested model: pick one small, broadly available model.
- [ ] Implement Ollama discovery:
  - [ ] Query `GET /api/tags`.
  - [ ] Query `POST /api/show` for context length when available.
  - [ ] Convert discovered models into Morph model metadata.
  - [ ] Mark cost as zero/unknown local cost.
  - [ ] Infer reasoning capability conservatively from known model names only.
- [ ] Implement Ollama onboarding/config:
  - [ ] Prompt for base URL.
  - [ ] Validate reachability.
  - [ ] Show discovered models.
  - [ ] Offer to pull the selected model when missing.
  - [ ] Persist the selected default model.
- [ ] Implement non-interactive setup:
  - [ ] `--provider ollama`.
  - [ ] `--base-url`.
  - [ ] `--model`.
  - [ ] `--pull-missing`.
- [ ] Add runtime execution:
  - [ ] Convert Morph messages to Ollama `/api/chat` messages.
  - [ ] Convert tool definitions to Ollama tool schema.
  - [ ] Parse streaming NDJSON.
  - [ ] Accumulate streamed text and tool calls.
  - [ ] Convert Ollama tool calls back into Morph tool invocation events.
  - [ ] Respect cancellation.
  - [ ] Preserve usage metadata when Ollama returns token counts.
- [ ] Add compatibility switches:
  - [ ] Native Ollama mode by default.
  - [ ] Optional OpenAI-compatible mode for proxies that only expose `/v1/chat/completions`.
  - [ ] Optional `num_ctx` injection for OpenAI-compatible Ollama mode.
- [ ] Add user-facing diagnostics:
  - [ ] Ollama not running.
  - [ ] Base URL points at `/v1` while native mode is selected.
  - [ ] Model not pulled.
  - [ ] Tool calling failed or returned raw tool JSON.
  - [ ] Context too small.

## Phase 2 - Provider Registry and Model Catalog Integration

- [ ] Route local providers through the same provider registry used by hosted providers.
- [ ] Add model catalog entries from:
  - [ ] Explicit config.
  - [ ] Runtime discovery.
  - [ ] Cached discovery results.
- [ ] Make model picker local-aware:
  - [ ] Group Ollama models under `Ollama`.
  - [ ] Show local/remote base URL.
  - [ ] Show installed versus suggested models.
  - [ ] Offer pull action for missing Ollama models.
- [ ] Add provider status checks:
  - [ ] Reachability.
  - [ ] Model list availability.
  - [ ] Selected model availability.
  - [ ] Tool support smoke check where practical.
- [ ] Cache discovery with short TTL and manual refresh.
- [ ] Ensure explicit config disables or overrides discovery when the user has pinned model definitions.

## Phase 3 - OpenAI-Compatible Local Providers

- [ ] Add a shared OpenAI-compatible local provider adapter.
- [ ] Support vLLM:
  - [ ] Default base URL: `http://127.0.0.1:8000/v1`.
  - [ ] Env key: `VLLM_API_KEY`.
  - [ ] Discovery from `GET /models`.
  - [ ] Manual model config fallback.
- [ ] Support SGLang:
  - [ ] Default base URL: `http://127.0.0.1:30000/v1`.
  - [ ] Env key: `SGLANG_API_KEY`.
  - [ ] Discovery from `GET /models`.
  - [ ] Manual model config fallback.
- [ ] Support generic custom local endpoints:
  - [ ] User-defined provider id.
  - [ ] User-defined base URL.
  - [ ] OpenAI-compatible adapter.
  - [ ] Optional API key.
  - [ ] Explicit model id required when discovery is unavailable.
- [ ] Add compatibility presets for common runtimes:
  - [ ] LM Studio.
  - [ ] llama.cpp server.
  - [ ] LiteLLM proxy.
  - [ ] Text Generation Inference if compatible enough.
- [ ] Normalize common endpoint mistakes:
  - [ ] Missing `/v1` for OpenAI-compatible runtimes.
  - [ ] Extra `/v1` for native Ollama.
  - [ ] Empty model list but reachable server.

## Phase 4 - Capabilities, Fallbacks, and Routing

- [ ] Add provider/model capability detection:
  - [ ] Tool calling.
  - [ ] JSON/schema mode.
  - [ ] Vision input.
  - [ ] Reasoning/thinking controls.
  - [ ] Max context.
  - [ ] Max output.
- [ ] Add fallback chains across local and hosted providers:
  - [ ] Local-first fallback.
  - [ ] Hosted fallback when local runtime is unavailable.
  - [ ] Local fallback for privacy-sensitive profiles.
- [ ] Add policy controls:
  - [ ] Require local-only mode.
  - [ ] Allow hosted fallback.
  - [ ] Deny network model calls.
  - [ ] Per-profile provider allowlist.
- [ ] Add smart routing integration:
  - [ ] Cheap/simple turns to small local model.
  - [ ] Complex turns to configured primary.
  - [ ] Never route out of local-only profiles.
- [ ] Add clear runtime labels in session metadata:
  - [ ] Provider.
  - [ ] Model.
  - [ ] Base URL host.
  - [ ] Local/native/OpenAI-compatible adapter.

## Phase 5 - Local Embeddings and Memory

- [ ] Keep chat models and embedding models configured independently.
- [ ] Add Ollama embeddings:
  - [ ] Default model such as `nomic-embed-text`.
  - [ ] Use `/api/embeddings`.
  - [ ] Reuse Ollama base URL and auth marker.
- [ ] Add local GGUF embeddings later if needed:
  - [ ] Decide runtime dependency strategy.
  - [ ] Add model path/cache config.
  - [ ] Add setup diagnostics for native dependencies.
- [ ] Add memory search fallback policy:
  - [ ] `local`.
  - [ ] `ollama`.
  - [ ] hosted embedding providers.
  - [ ] `none`.
- [ ] Surface embedding provider status in diagnostics.

## Phase 6 - UX, Docs, and Migration

- [ ] Add `morph provider add ollama` or equivalent guided setup.
- [ ] Add `morph providers status` with local runtime checks.
- [ ] Add `morph models list --provider ollama`.
- [ ] Add `morph models pull <ollama/model>` if Morph should wrap `ollama pull`; otherwise link to the exact command.
- [ ] Add examples:
  - [ ] Pure local Ollama setup.
  - [ ] Remote LAN Ollama setup.
  - [ ] vLLM setup.
  - [ ] SGLang setup.
  - [ ] LM Studio/custom OpenAI-compatible setup.
  - [ ] Local-first with hosted fallback.
  - [ ] Strict local-only mode.
- [ ] Add migration guidance for existing custom endpoint users:
  - [ ] Detect localhost OpenAI-compatible configs that look like Ollama.
  - [ ] Suggest native Ollama provider when applicable.
  - [ ] Preserve custom endpoint behavior unless the user opts in.

## Phase 7 - Testing and Validation

- [ ] Unit tests:
  - [x] Provider id/model ref parsing.
  - [ ] Ollama API base URL normalization.
  - [ ] `/api/tags` discovery.
  - [ ] `/api/show` context extraction.
  - [ ] Native Ollama message conversion.
  - [ ] Native Ollama tool conversion.
  - [ ] NDJSON stream parsing.
  - [x] OpenAI-compatible local provider config.
  - [ ] Diagnostic messages.
- [ ] Integration tests with mocked local servers:
  - [ ] Ollama reachable with no models.
  - [ ] Ollama with one model.
  - [ ] Ollama missing model pull path.
  - [ ] Ollama streaming text.
  - [ ] Ollama streaming tool calls.
  - [ ] vLLM `/models` discovery.
  - [ ] Custom endpoint with failed discovery but explicit model.
- [ ] Optional live tests guarded by env:
  - [ ] `MORPH_TEST_OLLAMA_URL`.
  - [ ] `MORPH_TEST_VLLM_URL`.
  - [ ] `MORPH_TEST_SGLANG_URL`.
- [ ] End-to-end smoke tests:
  - [ ] Configure Ollama.
  - [ ] Select model.
  - [ ] Run one tool-calling task.
  - [ ] Restart daemon/CLI and confirm persisted model still works.

## Phase 8 - Rollout

- [ ] Ship Ollama behind a feature flag or experimental label.
- [ ] Dogfood native Ollama with at least:
  - [ ] One small text-only model.
  - [ ] One tool-capable coding model.
  - [ ] One remote/LAN Ollama instance.
- [ ] Promote Ollama to stable after:
  - [ ] Setup success path is reliable.
  - [ ] Tool calls are stable.
  - [ ] Diagnostics are actionable.
  - [ ] Fallback behavior is predictable.
- [ ] Add vLLM and SGLang after the shared OpenAI-compatible local adapter is stable.
- [ ] Revisit broader local providers after real user configs appear.

## Open Questions

- [ ] Should Morph wrap `ollama pull`, or should it only detect and print the command?
- [ ] Should native Ollama mode be mandatory, or should OpenAI-compatible Ollama mode remain user-selectable?
- [ ] How should local-only mode interact with remote tools and web-enabled features?
- [ ] Do we need a provider plugin system now, or is a registry abstraction enough for the first two providers?
- [ ] Should local model discovery run on startup, on model picker open, or only on explicit refresh?
- [ ] How much of provider state should be persisted versus discovered each run?

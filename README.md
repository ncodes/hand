# Hand

Hand is a terminal-first personal agent written in Go.

The project is inspired in part by the [dæmon from *His Dark Materials*](https://en.wikipedia.org/wiki/D%C3%A6mon_(His_Dark_Materials)): close companions that are personal, steady, and meaningfully present. The goal is not just to build another CLI assistant, but to shape a tool that feels like a capable working partner.

The long-term dream for Hand is a personal agent that can understand your workflow, help you think, assist with research and coding, carry useful context over time, and become dependable enough to feel like a real extension of how you work. A further part of that dream is meaningful interaction between daemons, where your Hand can collaborate with other trusted Hands on shared tasks and coordination.

## Requirements

- Go `1.26.1`
- a valid model provider key

## Quick Start

1. Copy one of the provided examples:
   - `cp example.env .env`
   - `cp example.yaml config.yaml`
2. Uncomment the values you want to use and replace the placeholder key values.
3. Set at least `HAND_NAME`, `HAND_MODEL`, `HAND_MODEL_PROVIDER`, and a provider auth value such as
   `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `ANTHROPIC_API_KEY`, or `COPILOT_GITHUB_TOKEN`.
4. Start the daemon:

```bash
go run ./cmd/hand daemon start
```

The `daemon start` command prepares the runtime service.

You can also send a direct prompt through the root command:

```bash
go run ./cmd/hand \
  --chat \
  --name Daemon \
  --model qwen/qwen3.5-27b \
  --model.provider openrouter \
  --model.api-key "$OPENROUTER_API_KEY" \
  "hello"
```

## Config

Config precedence is:

`flags > env > config file`

Config file values:
- `name`
- `model.name`
- `model.summaryModel`
- `model.provider`
- `model.summaryProvider`
- `model.baseUrl`
- `model.summaryBaseUrl`
- `model.api`
- `model.summaryApi`
- `model.stream`
- `rpc.address`
- `rpc.port`
- `log.level`
- `log.noColor`
- `debug.requests`
- `memory.enabled`
- `memory.provider`

Env equivalents:
- `HAND_NAME`
- `HAND_MODEL`
- `HAND_MODEL_SUMMARY`
- `HAND_MODEL_PROVIDER`
- `HAND_MODEL_SUMMARY_PROVIDER`
- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `ANTHROPIC_API_KEY`
- `COPILOT_GITHUB_TOKEN`
- `HAND_MODEL_BASE_URL`
- `HAND_MODEL_SUMMARY_BASE_URL`
- `HAND_MODEL_API`
- `HAND_MODEL_SUMMARY_API`
- `HAND_MODEL_STREAM`
- `HAND_RPC_ADDRESS`
- `HAND_RPC_PORT`
- `HAND_LOG_LEVEL`
- `HAND_LOG_NO_COLOR`
- `HAND_DEBUG_REQUESTS`
- `HAND_MEMORY_ENABLED`
- `HAND_MEMORY_PROVIDER`

### Model Configuration

Supported `model.provider` values (default when unset: `openrouter`):
- `openrouter`: routes model requests through the OpenRouter API
- `openai`: uses the official OpenAI client with its default base URL (https://api.openai.com/v1), sending requests directly to the OpenAI API.
- `anthropic`: Anthropic provider definition for Anthropic-native APIs
- `github-copilot`: token-backed GitHub Copilot provider definition

Current config direction:
- put stable defaults in `config.yaml`
- use `.env` for local secrets and machine-specific values
- use CLI flags for one-off overrides

Typical model settings:
- `model.name`: provider model ID such as `qwen/qwen3.5-27b`
- `model.summaryModel`: optional slug for compaction/summary; defaults to `model.name` when unset
- `model.provider`: `openrouter`, `openai`, `anthropic`, or `github-copilot`
- `model.summaryProvider`: optional provider for compaction/summary API calls; defaults to `model.provider` when unset
- `models.providers.<provider>.apiKey`: provider-specific static API key
- `models.providers.<provider>.apiKeyEnv`: provider-specific environment key lookup order
- `models.main.apiKey`, `models.summary.apiKey`, `models.embedding.apiKey`: role-specific API keys
- `model.baseUrl`: explicit provider base URL when needed
- `model.summaryBaseUrl`: base URL for the summary provider when it differs from the main provider (optional)
- `model.api`: `openai-completions` or `openai-responses` for chat requests
- `model.summaryApi`: optional; same values as `model.api`, used for compaction/summary; defaults to `model.api` when unset. When the effective summary API or provider differs from the main chat settings, the summary client base URL is derived accordingly unless `model.summaryBaseUrl` is set.
- `model.stream`: stream assistant text responses during chat requests; defaults to `true`
- `rpc.address`: interface the daemon binds to
- `rpc.port`: port the daemon binds to
- `debug.requests`: emits model request metadata at debug level without request bodies

## Commands

- `hand daemon start`: start the runtime service
- `hand doctor`: run startup diagnostics and readiness checks
- `hand --chat "<message>"` or `hand -c "<message>"`: send a single chat message

## gRPC

The daemon exposes a gRPC service defined in [internal/rpc/proto/hand.proto](./internal/rpc/proto/hand.proto).

Current RPC surface:
- `HandService/Respond`: streams assistant text deltas and a terminal done event

The generated Go stubs live under [internal/rpc/proto](./internal/rpc/proto), and the service implementation lives in [internal/rpc/service.go](./internal/rpc/service.go).

## Development

```bash
# Generate protobuf stubs
make build-proto

# Build and install the binary
make build
make install

# Run the tests
make lint
make test
```

## TODOs

Foundation:
- [x] Define package boundaries for runtime, tools, UI, storage, and integrations
- [x] Implement config file loading plus env overrides
- [x] Implement provider-specific auth resolution and validation
- [x] Define a normalized model client interface
- [x] Add structured logging and request metadata diagnostics
- [x] Add startup diagnostics and doctor checks

Agent Runtime:
- [x] Implement message model and conversation state
- [x] Implement the synchronous tool-calling loop
- [x] Add max-iteration and shared-budget logic
- [ ] Add interrupt and cancel support
- [x] Add request normalization for different APIs
- [ ] Add session log persistence for debugging
- [ ] Expand prompt assembly and context injection layers

Capabilities:
- [ ] Implement the tool registry and toolset system
- [ ] Add built-in tools
- [ ] Implement terminal execution backends
- [ ] Add persistence, sessions, and search
- [ ] Add durable memory persistence and retrieval behavior

Product Surface:
- [ ] Build the interactive CLI experience
- [ ] Add messaging and external integrations
- [ ] Add automation and scheduling
- [ ] Add editor and ACP surfaces

## License

MIT. See [LICENSE](./LICENSE).

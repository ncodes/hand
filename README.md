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
3. Set at least `NAME`, `MODEL`, `MODEL_PROVIDER`, and one auth value:
   `MODEL_KEY`, `OPENAI_API_KEY`, or `OPENROUTER_API_KEY`.
4. Start the daemon:

```bash
go run ./cmd/hand up
```

The `up` command prepares the runtime and starts the gRPC service.

You can also send a direct prompt through the root command:

```bash
go run ./cmd/hand \
  --name Daemon \
  --model qwen/qwen3.5-27b \
  --model.provider openrouter \
  --model.key "$MODEL_KEY" \
  "hello"
```

## Config

Config precedence is:

`flags > env > config file`

Config file values:
- `name`
- `model.name`
- `model.provider`
- `model.key`
- `model.baseUrl`
- `rpc.address`
- `rpc.port`
- `log.level`
- `log.noColor`
- `debug.requests`

Env equivalents:
- `NAME`
- `MODEL`
- `MODEL_PROVIDER`
- `MODEL_KEY`
- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `MODEL_BASE_URL`
- `RPC_ADDRESS`
- `RPC_PORT`
- `LOG_LEVEL`
- `LOG_NO_COLOR`
- `DEBUG_REQUESTS`

### Model Configuration

Supported `model.provider` values (default when unset: `openrouter`):
- `openrouter`: routes model requests through the OpenRouter API
- `openai`: uses the official OpenAI client with its default base URL (https://api.openai.com/v1), sending requests directly to the OpenAI API.

Current config direction:
- put stable defaults in `config.yaml`
- use `.env` for local secrets and machine-specific values
- use CLI flags for one-off overrides

Typical model settings:
- `model.name`: provider model slug such as `qwen/qwen3.5-27b`
- `model.provider`: `openrouter` or `openai`
- `model.key`: generic provider API key fallback
- `model.openaiApiKey`: provider-specific OpenAI API key
- `model.openrouterApiKey`: provider-specific OpenRouter API key
- `model.baseUrl`: explicit provider base URL when needed
- `rpc.address`: interface the daemon binds to
- `rpc.port`: port the daemon binds to
- `debug.requests`: emits sanitized model request dumps at debug level

## Commands

- `hand up`: prepare the runtime and start the gRPC daemon
- `hand doctor`: run startup diagnostics and readiness checks
- `hand "<message>"`: send a single chat message

## gRPC

The daemon exposes a gRPC service defined in [internal/rpc/proto/hand.proto](./internal/rpc/proto/hand.proto).

Current RPC surface:
- `HandService/Echo`: returns the provided message

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
- [x] Add structured logging and request debug dumps
- [x] Add startup diagnostics and doctor checks

Agent Runtime:
- [x] Implement message model and conversation state
- [x] Implement the synchronous tool-calling loop
- [x] Add max-iteration and shared-budget logic
- [ ] Add interrupt and cancel support
- [x] Add request normalization for different API modes
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

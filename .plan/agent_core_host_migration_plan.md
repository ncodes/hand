# Agent Core Host Migration Plan

## Goal

Make the agent core reusable outside Hand by moving application-agnostic orchestration into `pkg/agent` and keeping Hand-specific wiring in `internal/host`.

External applications should be able to instantiate an agent with their own model client, session store, tool registry, trace sink, memory provider, and prompt/instruction provider without importing Hand internals.

## Non-Goals

- Do not change CLI, TUI, RPC, storage, memory, tracing, or tool behavior during extraction.
- Do not move Hand-native tools into `pkg/agent`.
- Do not make `pkg/agent` depend on any `internal/*` package.
- Do not redesign prompts, compaction, planning, or memory semantics while moving boundaries.

## Target Shape

```text
pkg/agent
  agent.go
  turn.go
  options.go
  event.go
  config.go

pkg/agent/message
pkg/agent/model
pkg/agent/session
pkg/agent/tool
pkg/agent/trace
pkg/agent/memory
pkg/agent/prompt

internal/host
  config.go
  session.go
  tools.go
  trace.go
  memory.go
  prompt.go
  environment.go
```

`pkg/agent` owns reusable orchestration. `internal/host` owns Hand defaults and wires current Hand packages into the reusable interfaces.

The final architecture must not look like a compatibility bridge. Hand should use the public agent core as first-class infrastructure, with `internal/host` acting as the native Hand runtime assembly layer.

## Final Architecture Target

The migration is not complete while Hand still depends on compatibility aliases, fallback constructors, legacy turn wrappers, or host types that only exist to mimic the old `internal/agent` surface.

The intended final shape is:

- `pkg/agent` contains the reusable agent core, public runtime types, and orchestration contracts.
- `internal/host` is the Hand runtime composition package. It constructs `pkg/agent.Agent` directly from Hand config, state, tools, prompts, traces, memory, and environment services.
- Hand application packages depend on `internal/host` for construction and on public/shared packages for data types. They do not depend on `internal/agent`.
- `internal/agent` is deleted or reduced to non-orchestration feature code only if a real Hand-specific feature remains there. It must not be a compatibility facade.
- Adapters are allowed only when they represent real ownership boundaries, such as Hand state to agent session store, Hand tools to agent tool registry, or Hand traces to agent trace sink. They must be named by their domain, not by compatibility.

Forbidden end-state patterns:

- No `legacy`, `compat`, `fallback`, or wrapper-only runtime paths.
- No public type aliases whose only purpose is preserving old internal import paths.
- No constructors that silently choose between old and new runtime paths.
- No reflection-based or nil-fallback adaptation to support obsolete callers.
- No host service surface that mirrors `internal/agent` just because callers used to expect it.
- No `internal/host` import of `internal/agent`.
- No application caller imports from `internal/agent`.

## Public Constructor Sketch

```go
agent, err := agent.New(agent.Options{
	Config:        agent.Config{MaxIterations: 6, Stream: true},
	ModelClient:   modelClient,
	SummaryClient: summaryClient,
	Sessions:      sessionStore,
	Tools:         toolRegistry,
	Trace:         traceFactory,
	Memory:        memoryProvider,
	Prompts:       promptProvider,
	Logger:        logger,
})
```

Hand then becomes one host:

```go
core, err := host.NewAgent(ctx, host.Options{
	Config: config,
	Profile: profile,
})
```

Other applications can build their own host package and pass their own implementations to `pkg/agent`.

## Core Interfaces

### Model

Keep the current model shape mostly intact and move it behind `pkg/agent/model`.

```go
type Client interface {
	Complete(context.Context, Request) (*Response, error)
	CompleteStream(context.Context, Request, func(StreamDelta)) (*Response, error)
}
```

### Session

Expose only what the turn loop needs, not the full Hand state manager.

```go
type Store interface {
	Resolve(context.Context, string) (Session, error)
	GetMessages(context.Context, string, MessageQuery) ([]message.Message, error)
	AppendMessages(context.Context, string, []message.Message) error
	UpdateLastPromptTokens(context.Context, string, int) error
}
```

Trace persistence can either be part of a trace sink or a small optional interface.

### Tools

Separate tool definitions from Hand's native tool registry.

```go
type Registry interface {
	Resolve(Policy) ([]Definition, error)
	Invoke(context.Context, Call) message.Message
}
```

Hand's current tool system becomes an adapter in `internal/host/tools.go`.

### Trace

Keep trace events generic, with Hand-specific hydration/rendering outside the core.

```go
type Factory interface {
	NewSession(RunContext) Session
}

type Session interface {
	Record(EventType, any)
	Close()
}
```

### Memory

Core should know only that memory can produce prompt context and accept compaction flush hooks.

```go
type Provider interface {
	LoadPromptInstruction(context.Context, Query) (prompt.Instruction, error)
	FlushBeforeCompaction(context.Context, FlushInput) error
}
```

### Prompts

Move instruction assembly behind explicit prompt inputs.

```go
type Provider interface {
	LoadBaseInstructions(context.Context, RunContext) (prompt.Instructions, error)
	BuildEnvironmentInstruction(context.Context, EnvironmentInput) (prompt.Instruction, error)
}
```

## Migration Rules

- Each phase must compile and pass focused tests before the next phase.
- Use type aliases only as short-lived scaffolding inside the active phase that introduces them.
- Move one dependency boundary at a time.
- Remove old `internal/agent` entry points once Hand callers are migrated.
- Add a dependency guard so `pkg/agent` cannot import `internal/*`.
- Add dependency guards so `internal/host` and application packages cannot depend on `internal/agent`.
- Preserve trace event names and payloads until TUI hydration and live rendering are explicitly migrated.
- Keep `internal/host` names plain: `config.go`, `session.go`, `tools.go`, `trace.go`, `memory.go`, `prompt.go`, `environment.go`.
- Treat adapters as first-class host wiring, not compatibility shims. If an adapter has no stable domain boundary, remove it.
- Prefer direct construction and explicit dependencies over fallback behavior.

## [x] Phase 0: Baseline And Dependency Map

Objective: freeze current behavior before extraction.

Work:

- Capture current package dependencies with `go list`.
- Add or identify focused tests for `internal/agent`, summary compaction, trace streaming, tool turns, and TUI trace conversion.
- Add a dependency check target or test that can later enforce no `internal/*` imports from `pkg/agent`.

Done when:

- Existing agent and TUI tests pass.
- There is a clear dependency graph for current `internal/agent`.
- The first migration PR can be reviewed as behavior-preserving.

Baseline commands:

```text
make agent-deps
make test-agent-baseline
make check-pkg-agent-deps
```

`make agent-deps` prints direct and transitive Hand imports for `internal/agent`.
`make test-agent-baseline` runs the focused agent, compaction, summary, TUI trace conversion, and CLI package tests.
`make check-pkg-agent-deps` skips cleanly until `pkg/agent` exists, then fails if any `pkg/agent` package imports `github.com/wandxy/hand/internal/*`.

Risk:

- Existing tests may not cover enough of the turn loop. Add focused tests before moving code that touches request assembly, tool loops, or compaction.

Completed:

- Added `make agent-deps` to capture direct and transitive `internal/agent` package dependencies.
- Added `make test-agent-baseline` for the focused agent, compaction, summary, TUI, and CLI baseline.
- Added `make check-pkg-agent-deps` as the guard for future `pkg/agent` imports.
- Verified `make agent-deps`, `make check-pkg-agent-deps`, and `make test-agent-baseline`.
- Committed as `a6e52702 chore(agent): add migration baseline checks`.

## [x] Phase 1: Extract Generic Data Types

Objective: move generic request, response, message, and event data shapes into public packages.

Work:

- Move or alias `internal/models` into `pkg/agent/model`.
- Move or alias `internal/messages` into `pkg/agent/message`.
- Move generic agent event types into `pkg/agent`.
- Update internal callers through aliases first to avoid one huge diff.

Done when:

- Current code compiles with aliases.
- No behavior changes are introduced.
- Existing tests pass.

Risk:

- Broad import churn can obscure behavior changes. Prefer aliases first, then direct imports in later phases.

Progress:

- Added `pkg/agent/event` public event shapes.
- Added `pkg/agent/message` public message shapes and behavior tests.
- Added `pkg/agent/model` public model client/request/response shapes.
- Routed `internal/messages` through `pkg/agent/message` compatibility aliases.
- Routed `internal/models` through `pkg/agent/model` compatibility aliases.
- Verified `go test ./pkg/agent/...`, `make check-pkg-agent-deps`, and `make test-agent-baseline`.

Deferred:

- Keep the Hand trace-backed `internal/agent.Event` shape until the trace boundary is extracted.
- Migrate direct internal callers to public packages later when broader import churn is safer.

## [x] Phase 2: Define Session Store Boundary

Objective: replace direct `state/manager` usage in the core with a small session interface.

Work:

- Add `pkg/agent/session.Store`.
- Implement `internal/host/session.go` adapter over the existing state manager.
- Change turn loading, appending, prompt-token updates, and trace persistence to use the interface.
- Keep state search, SQLite, vector stores, and profile data inside Hand internals.

Done when:

- `Turn` no longer depends directly on `internal/state/manager`.
- Hand behavior is unchanged.
- Session timeline and compaction still read current state correctly.

Risk:

- The current state manager exposes more than the agent needs. Resist leaking the whole manager through the interface.

Progress:

- Added `pkg/agent/session` with public session, compaction, message query, store, and trace recorder shapes.
- Added `internal/host/session.go` as the first Hand host adapter over the current state manager surface.
- Verified `go test ./pkg/agent/... ./internal/host` and `make check-pkg-agent-deps`.
- Replaced turn-scoped session resolution, message reads/writes, prompt-token writes, reasoning trace persistence, and plan hydration with the session store/trace recorder interfaces.
- Added an Agent turn factory so normal response and memory-flush paths construct turns through the session boundary.
- Kept the old `NewTurn` constructor as a compatibility wrapper for tests and transitional callers.

Deferred:

- Broader Agent service methods still use the state manager until the public Agent API moves in later phases.
- Summary compaction still uses the existing summary store interface until summary is extracted from Hand internals.

## [x] Phase 3: Define Tool Boundary

Objective: make the core execute abstract tools while Hand supplies its native tools.

Work:

- Add `pkg/agent/tool.Registry`, `Definition`, `Call`, and `Policy`.
- Implement `internal/host/tools.go` adapter over existing `internal/tools`.
- Move tool resolution and invocation in `Turn` to the registry interface.
- Preserve current tool trace payloads.

Done when:

- `pkg/agent` can request tool definitions and invoke tools without importing Hand tools.
- Tool-call turns, plan tool state, and process tool state still trace correctly.

Risk:

- Tool context currently carries Hand run/session metadata. Keep that metadata in host context injection, not in core tool types.

Progress:

- Added `pkg/agent/tool` with public registry, call, definition, policy, group, and capability shapes.
- Added conversion helpers between public tool calls/definitions and model calls/definitions.
- Added `internal/host/tools.go` to adapt the current Hand environment tool registry into the public tool registry boundary.
- Moved normal `Agent` turn construction onto the public tool registry while keeping the legacy `NewTurn` wrapper behavior-preserving for tests and transitional callers.
- Routed turn tool resolution and invocation through the tool registry interface when present.
- Routed memory-flush tool definition filtering through the same turn tool boundary.
- Added host adapter tests for policy/definition conversion and invocation delegation.
- Verified `go test -tags sqlite_fts5 ./internal/agent ./internal/host ./pkg/agent/...`, `make check-pkg-agent-deps`, and `make test-agent-baseline`.

Deferred:

- `internal/agent` still uses Hand environment and tool-context helpers until Phase 4 extracts trace, memory, and prompt/environment boundaries.
- Agent-level helper methods still expose the older environment-backed tool path until the public Agent API moves in later phases.

## [x] Phase 4: Define Trace, Memory, And Prompt Boundaries

Objective: remove remaining Hand environment coupling from the turn loop.

Work:

- Add `pkg/agent/trace` session/factory interfaces.
- Add `pkg/agent/memory` provider interface.
- Add `pkg/agent/prompt` instruction types or move current `internal/instructions` behind aliases.
- Implement host adapters for current environment, memory provider, safety trace events, and prompt instructions.
- Split plan hydration interfaces from the concrete Hand environment.

Done when:

- The turn loop no longer depends on `internal/environment`.
- Trace events still stream live and hydrate from persisted state.
- Memory retrieval and flush-before-compaction behavior are unchanged.

Risk:

- `environment.Environment` currently mixes tools, prompts, tracing, memory, and plan state. Extract it gradually instead of replacing it in one pass.

Progress:

- Added `pkg/agent/prompt` with public prompt provider, instruction, run-context, and environment-input shapes.
- Added `pkg/agent/trace` with public trace session/factory/event shapes.
- Added `pkg/agent/memory` with public prompt-retrieval and compaction-flush provider shapes.
- Added `internal/host/prompt.go` to adapt Hand environment instructions into the public prompt boundary.
- Routed normal Agent turn construction through the host prompt provider.
- Updated `Turn.load` to load base instructions through the prompt provider when present, with the legacy environment path retained for transitional tests and callers.
- Split the turn runtime dependencies into small capability interfaces for trace sessions, safety trace events, memory providers, iteration budgets, plan state, prompts, and tools.
- Updated normal Agent turn construction to pass the existing Hand environment as those discrete host capabilities instead of storing it as the turn runtime.
- Kept a legacy `env any` shim for transitional tests and compatibility wrappers, while avoiding a direct `environment.Environment` dependency in the turn loop.
- Verified `go test -tags sqlite_fts5 ./internal/agent ./internal/host ./pkg/agent/...`, `make check-pkg-agent-deps`, `make test-agent-baseline`, and `make test`.

## [x] Phase 5: Move Turn Core To `pkg/agent`

Objective: move the reusable turn loop after its dependencies are abstract.

Work:

- Move `turn.go`, preflight tracing, summary fallback flow, and context assembly pieces that are not Hand-specific into `pkg/agent`.
- Keep compaction and summary logic public only where needed.
- Keep a compatibility wrapper in `internal/agent` if callers still import it.

Done when:

- `pkg/agent` owns the model/tool iteration loop.
- `internal/agent` contains little or no orchestration logic.
- Tests prove tool turns, streaming, compaction, safety, memory, and plan hydration still work.

Risk:

- The turn loop is high-blast-radius. Move after interfaces are stable and preserve logs/traces carefully.

Progress:

- Added `pkg/agent.RunModelToolLoop` as the public reusable model/tool loop runner.
- Updated `internal/agent.Turn` to delegate repeated model/tool iteration, completion detection, and exhausted-budget fallback to the public loop runner.
- Kept Hand-specific compaction, memory, safety, persistence, trace, streaming, and tool execution behavior in the turn step hook so behavior remains stable during migration.
- Moved generic model-to-message tool-call conversion into `pkg/agent/model`.
- Kept the internal `models` compatibility package forwarding to the public conversion helper.
- Updated `internal/agent` to use the public helper through the compatibility package.
- Added public package coverage for the loop runner and conversion helper.
- Verified `go test -tags sqlite_fts5 ./internal/agent ./pkg/agent/...`, `make check-pkg-agent-deps`, `make test-agent-baseline`, and `make test`.

## [x] Phase 6: Move Agent Service API To `pkg/agent`

Objective: expose the reusable Agent constructor and public API.

Work:

- Move `Agent`, `Options`, `RespondOptions`, `CompactSessionResult`, `ContextStatus`, and response event types into `pkg/agent`.
- Keep Hand-specific defaults out of the constructor.
- Keep `internal/agent` as a thin compatibility wrapper or remove it after all callers migrate.

Done when:

- Another Go package can instantiate `pkg/agent.Agent` with fake in-memory dependencies.
- A minimal external integration test can respond to a message without importing `internal/*`.

Risk:

- Public API can harden too early. Keep exported surface small and add only what a host actually needs.

Progress:

- Added a public `pkg/agent.Agent` with `Options`, `RespondOptions`, `Responder`, `CompactSessionResult`, and `ContextStatus`.
- Added `pkg/agent.New` and `pkg/agent.NewAgent` constructors that accept public model, session, tool, and prompt dependencies.
- Implemented a public response path that resolves sessions, appends user/assistant/tool messages, runs model calls, executes public tool registries, and uses the public loop runner.
- Kept Hand-specific defaults, config, storage, compaction, trace, and environment wiring outside `pkg/agent`.
- Added external-package tests proving another application can instantiate `pkg/agent.Agent` with fake in-memory dependencies and receive normal and tool-loop responses without importing `internal/*`.
- Verified `go test ./pkg/agent/...` and `make check-pkg-agent-deps`.

## [x] Phase 7: Build `internal/host`

Objective: make Hand a host of the reusable core.

Work:

- Create `internal/host` with plain adapter files:
  - `config.go`
  - `session.go`
  - `tools.go`
  - `trace.go`
  - `memory.go`
  - `prompt.go`
  - `environment.go`
- Move Hand-specific construction from `internal/agent.NewAgent` into host assembly.
- Keep profile, datadir, constants, SQLite, vector search, native tools, and TUI/RPC trace choices inside the host.

Done when:

- Daemon, RPC, CLI, and TUI construct the agent through `internal/host`.
- `pkg/agent` has no Hand-specific imports.

Risk:

- Host can become a dumping ground. Keep it as wiring/adapters only; business logic belongs either in `pkg/agent` or existing internal feature packages.

Progress:

- Added `internal/host.NewAgent` as the application-facing Hand agent constructor.
- Added host-owned `config.go`, `environment.go`, `memory.go`, `session.go`, `trace.go`, `tools.go`, and `prompt.go` adapter seams.
- Updated daemon startup and the e2e harness to construct agents through `internal/host`.
- Removed the transitional `internal/agent` dependency on `internal/host` so host can wrap agent construction without an import cycle.
- Kept direct `internal/agent.NewAgent` usage in internal agent tests as compatibility coverage.
- Verified focused agent, host, daemon, and e2e tests, the `pkg/agent` dependency guard, and the full test suite.

## [x] Phase 8: Migrate Hand Callers

Objective: switch application entry points to the new host/core split.

Work:

- Update daemon startup to call `host.NewAgent`.
- Update RPC service construction and tests.
- Update any direct `internal/agent` imports in TUI, CLI, and tests.
- Keep CLI behavior and config keys unchanged.

Done when:

- `hand up`, `hand version`, manual compaction, auto compaction, normal turns, and TUI transcript rendering behave as before.
- There are no important application callers left on the compatibility wrapper.

Risk:

- RPC and TUI may depend on concrete internal event shapes. Migrate those carefully with type aliases where possible.

Progress:

- Added host-owned service/type aliases for RPC, TUI, CLI, e2e, and test harness consumers.
- Migrated daemon, RPC server/service/client, e2e harnesses, TUI app, CLI tests, and agent stubs from direct `internal/agent` service-surface imports to `internal/host`.
- Kept `internal/agent/runcontext` imports unchanged where callers need the runtime identity type directly.
- Verified focused daemon, RPC, e2e, CLI, TUI, mock, and host tests, the `pkg/agent` dependency guard, and the full test suite.

## [x] Phase 9: Clean Up Compatibility Layer

Objective: remove transitional aliases and obsolete package paths.

Work:

- Delete or shrink `internal/agent` once callers are migrated.
- Replace aliases with direct imports where churn is now safe.
- Enforce dependency guard in CI/test.
- Remove dead methods from old environment abstractions.

Done when:

- `pkg/agent` is the only owner of core orchestration.
- `internal/host` is the only owner of Hand-specific wiring.
- Dependency guard prevents regressions.

Risk:

- Removing aliases too early can make review painful. Do this only after behavior is stable.

Progress:

- Started Phase 9 by wiring `check-pkg-agent-deps` into the default `make test` target so public core dependency regressions fail in the normal verification path.
- Replaced host service aliases with host-owned service, option, event, status, compaction, and timeline types.
- Wrapped the legacy internal agent behind `internal/host.Agent` so application callers depend on the host service surface, not the internal agent contract.
- Removed the dead `internal/agent.ServiceAPI` compatibility interface.
- Kept the remaining `internal/agent/runcontext` imports as runtime identity dependencies; moving that type is a separate package-boundary cleanup.

## [x] Phase 10: External Integration Example

Objective: prove the reusable package works outside Hand.

Work:

- Add a small test-only integration using in-memory session store, fake model, and one fake tool.
- Optionally add a compact example package if needed later.
- Keep documentation minimal unless requested.

Done when:

- The example/test constructs `pkg/agent.Agent` without importing Hand internals.
- The example covers a normal response and a tool-call response.

Risk:

- Avoid turning the example into another product surface. It exists to verify the package boundary.

Progress:

- Added public `pkg/agent` examples that construct an agent with an in-memory session store, fake model client, and fake tool registry without importing Hand internals.
- Covered both a normal response and a model tool-call response in executable examples.
- Reused the test-only fake dependencies already used by the public package boundary tests to avoid adding another product surface.

## [x] Phase 11: Promote The Core Service Surface

Objective: make `pkg/agent` own every reusable service type so the host does not mirror the old internal service contract.

Work:

- Move reusable response options, stream events, context status, compaction results, and timeline/status data into `pkg/agent` or focused public subpackages.
- Keep Hand-only presentation or RPC details in `internal/host`, `internal/rpc`, or `internal/tui`.
- Replace host aliases and wrapper-only types with direct use of the public core types where they are genuinely reusable.
- Remove any type whose only purpose is keeping the old `internal/agent` shape alive.
- Update tests to assert behavior against the public core and host-native surfaces, not compatibility aliases.

Done when:

- `internal/host` no longer defines service types by copying or aliasing old `internal/agent` types.
- Application callers use either `internal/host` construction types or `pkg/agent` core types directly.
- No service-surface type exists only to preserve a migrated import path.

Risk:

- Moving too much into `pkg/agent` can make Hand-specific details public. Keep public types limited to reusable agent concepts.

Progress:

- Moved the reusable response, event, context status, compaction result, and session timeline shapes into `pkg/agent`.
- Added public timeline records backed by `pkg/agent/message` and `pkg/agent/session` instead of Hand storage internals.
- Changed `internal/host.ServiceAPI` to use public agent service types directly while keeping Hand-only storage, summary, and repair methods in the host boundary.
- Removed host-owned copied response/status/timeline structs and wrapper conversion helpers.
- Collapsed live trace streaming into the public `agent.Event` stream with an explicit `TraceEvents` opt-in flag.
- Updated RPC, TUI, e2e, CLI tests, and stubs to assert against public core service types where those types are reusable.
- Verified focused public core, host, agent, RPC, TUI, e2e, CLI, and stub packages with SQLite FTS5 tags.

## [x] Phase 12: Move Remaining Turn Runtime Into `pkg/agent`

Objective: make the reusable core own the full turn lifecycle instead of delegating only the model/tool loop.

Work:

- Move request assembly, context building, summary fallback control flow, tool-turn continuation, streaming event production, prompt-token accounting hooks, and cancellation handling into `pkg/agent`.
- Keep Hand-specific compaction storage, memory extraction, trace payload conversion, safety policy, and plan persistence behind explicit dependencies.
- Replace legacy turn construction with a single public runtime constructor that receives all dependencies explicitly.
- Remove nil-driven fallback paths that switch between legacy environment behavior and public interfaces.
- Keep logs and trace events behavior-preserving while moving ownership.

Done when:

- `pkg/agent` owns the turn lifecycle, not only a loop helper.
- Hand passes dependencies into the core explicitly through host wiring.
- There is no `internal/agent.NewTurn` compatibility constructor or equivalent fallback path.

Risk:

- This phase has high blast radius. Move with focused tests for normal turns, tool turns, streaming, compaction re-checks, memory flush, output safety, and summary fallback.

Progress:

- Added `pkg/agent.RunTurnLifecycle` to own the reusable turn lifecycle order: load, request instruction setup, trace/session opening, preparation hooks, input checks, user message acceptance, memory loading, model/tool iteration, and exhaustion fallback.
- Moved lifecycle behavior coverage into `pkg/agent` tests.
- Updated `internal/agent.Turn.Run` to delegate the top-level lifecycle to `pkg/agent` while keeping Hand-specific safety, memory, summary, trace, persistence, and model request details behind callbacks.
- Removed the production `internal/agent.NewTurn` compatibility constructor. The old constructor shape now exists only as package-local test support for existing focused turn tests.
- Kept Hand runtime construction on the explicit `NewTurnWithSessionStore` dependency path.

## [x] Phase 13: Rebuild `internal/host` As Native Hand Runtime Wiring

Objective: make the host package read like Hand's intended runtime assembly layer, not a bridge from old internals to new core.

Work:

- Make `internal/host.NewAgent` construct `pkg/agent.Agent` directly.
- Split host wiring by domain: config, sessions, tools, traces, memory, prompts, safety, compaction, plans, and runtime identity.
- Rename or delete adapters that describe their transitional role instead of their domain responsibility.
- Remove any wrapper that delegates wholesale to `internal/agent.Agent`.
- Keep Hand policies and defaults in host-owned constructors with explicit dependencies.

Done when:

- `internal/host` imports `pkg/agent` but does not import `internal/agent`.
- Daemon, RPC, TUI, CLI, and e2e construction paths use the native host assembly.
- The host package has no compatibility naming, compatibility comments, or fallback runtime branches.

Risk:

- Host can become too broad. It should compose dependencies, not own business logic that belongs in the core or existing feature packages.

Completed:

- Replaced the wrapper-only `internal/host.Agent` with host-owned runtime assembly and turn execution.
- `internal/host.NewAgent` now starts a host runtime that owns state, environment, tools, traces, memory, summaries, timelines, and session operations directly.
- Host startup now builds a public `pkg/agent.Agent` from Hand model, session, tool, and prompt dependencies.
- Removed the root `internal/agent` import from `internal/host`; remaining imports from `internal/agent/context` and `internal/agent/runcontext` are tracked for Phase 14 package-path cleanup.
- Renamed the host runtime files by domain so the package no longer reads as a compatibility bridge.

Verified:

- `go test -tags sqlite_fts5 ./internal/host ./cmd/up ./internal/e2e ./internal/rpc ./internal/rpc/client ./internal/tui/app ./internal/cli ./internal/mocks/agentstub`

## [ ] Phase 14: Delete Compatibility Package Paths

Objective: remove obsolete internal package paths and aliases after the native host/core split is complete.

Work:

- Delete or shrink `internal/agent` after all orchestration has moved.
- Replace `internal/models` and `internal/messages` compatibility aliases with direct imports from `pkg/agent/model` and `pkg/agent/message`, or delete them if no longer needed.
- Move runtime identity types out of `internal/agent/runcontext` into `pkg/agent/runcontext` or a first-class non-compatibility internal package.
- Remove dead tests that only prove compatibility wrappers.
- Update package comments and names so remaining packages describe real ownership.

Done when:

- `go list ./...` shows no application or host dependency on `internal/agent`.
- Remaining internal packages are feature packages, not compatibility package paths.
- Import paths and package names match the final architecture.

Risk:

- Large import churn can hide regressions. Use mechanical import updates where possible, then inspect behavioral diffs separately.

## [ ] Phase 15: Enforce First-Class Boundaries

Objective: prevent the compatibility layer from creeping back into the codebase.

Work:

- Extend dependency checks so:
  - `pkg/agent/...` cannot import `internal/...`.
  - `internal/host/...` cannot import `internal/agent/...`.
  - application entry points cannot import `internal/agent/...`.
- Add a lightweight source check that fails on compatibility-only names in the final runtime path, such as `legacy`, `compat`, or `fallback`, unless explicitly allowlisted.
- Wire the guards into the normal verification path.
- Add focused tests proving Hand construction runs through host-native wiring and public core dependencies.

Done when:

- `make test` fails if the old compatibility direction is reintroduced.
- The architecture can be understood from imports alone: applications -> host -> public core and Hand feature packages.
- The migration plan can be closed with no remaining compatibility cleanup deferred.

Risk:

- Over-broad text guards can block valid language. Keep allowlists small and reviewed.

## Verification Gates

Run focused tests after each phase:

```text
go test ./internal/agent
go test ./internal/tui/app
go test ./cmd/hand
go test ./pkg/agent/...
```

Run broader tests before removing compatibility wrappers:

```text
go test ./...
```

Add a guard equivalent to:

```text
go list -deps ./pkg/agent/... | reject paths containing /internal/
```

## Suggested First Slice

Start with a small, reviewable PR:

- Add `pkg/agent/model` as aliases around current model types.
- Add `pkg/agent/message` as aliases around current message types.
- Add dependency guard scaffolding.
- Do not change runtime behavior.

This creates public import paths and gives later phases somewhere stable to land without rewriting the turn loop immediately.

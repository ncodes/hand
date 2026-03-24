# Hermes Clone Plan

## Goal

Build a Hermes-like agent system in deliberate layers, starting with the smallest useful product and adding the higher-leverage systems only after the foundation is stable.

## Assumptions

- The clone should preserve Hermes' main strengths:
  - conversational agent loop
  - tool calling
  - terminal-centric workflows
  - configurable prompt/context system
  - memory and skills
  - multi-surface delivery
  - optional delegation and automation
- The clone should be built component by component, with each component delivering an immediately testable improvement.
- The first useful target is a strong CLI agent. Messaging, automations, ACP/editor integration, and RL features come after that.

## Recommended Delivery Order

1. Foundation and configuration
2. Core agent runtime
3. Tool registry and baseline tools
4. Terminal execution backends
5. CLI experience
6. Persistence, memory, and context files
7. Skills system
8. Delegation and code execution
9. Messaging gateway
10. Automations and scheduler
11. ACP/editor integration
12. Research and RL infrastructure

## Kanban Board

### Backlog

#### Component 1: Foundation, Config, and Provider Layer

What it does:
- Defines project structure, config loading, env var management, model/provider auth, and dependency boundaries.
- Establishes the internal contracts that everything else depends on.

What it offers:
- one source of truth for config
- provider-agnostic model access
- reproducible startup behavior
- safer future extension work

How it improves the clone:
- Prevents architecture drift early.
- Makes the clone model-portable instead of hard-coding one provider.
- Reduces rework when tools, gateway, and memory are added later.

Tasks:
- [x] Define package/module boundaries for runtime, tools, UI, storage, and integrations
- [x] Implement config file loading plus env overrides
- [x] Implement provider-specific auth resolution and validation
- [x] Define a normalized model client interface
- [x] Add structured logging and request debug dumps
- [x] Add startup diagnostics/doctor checks

Proposed package boundaries:
- `cmd/hand`: the top-level CLI entrypoint, root command wiring, config bootstrap, and process lifecycle startup
- `cmd/up`: explicit runtime boot command and future runtime-only operational subcommands
- `internal/config`: config schema, normalization, validation, env loading, and provider/model resolution inputs
- `internal/agent`: the high-level runtime coordinator that prepares the environment, owns the conversation loop, and orchestrates model/tool interactions
- `internal/context`: prompt assembly inputs, instruction layers, conversation state primitives, and shared runtime context values
- `internal/environment`: runtime environment preparation and backend-neutral execution context composition
- `internal/instruction`: base instruction builders and built-in instruction templates
- `internal/models`: provider-agnostic model interface plus provider-specific client adapters
- `internal/tools`: tool registry contracts, tool dispatch, tool result normalization, and built-in tool implementations
- `internal/storage`: persisted sessions, memory records, config snapshots, indexing, and search persistence
- `internal/ui`: CLI presentation concerns such as interactive shell state, streaming output, slash commands, and user-facing formatting
- `internal/integrations`: external surfaces such as messaging gateways, schedulers, editors, remote APIs, and platform-specific bridges
- `pkg/logutils`: cross-package logging primitives only; no domain logic

Dependency rules:
- `cmd/*` may depend on `internal/*` and `pkg/*`, but never the reverse
- `internal/agent` may depend on `config`, `context`, `environment`, `identity`, `models`, `tools`, and `storage`, but it should not depend on `ui` or external integrations
- `internal/models`, `internal/tools`, and `internal/storage` should expose contracts consumed by `agent`; they should not depend on each other through concrete implementations unless routed through interfaces
- `internal/ui` should depend on `agent` contracts and view models, not on provider clients or storage internals directly
- `internal/integrations` should call into `agent` and shared contracts, not bypass them to reach provider implementations
- `pkg/*` should stay small and generic; if a package starts carrying domain behavior it belongs under `internal/*`

Acceptance:
- Agent can boot with config and connect to at least one provider cleanly.

#### Component 2: Core Agent Runtime

What it does:
- Runs the conversation loop, builds system prompts, sends model requests, handles tool calls, and stops when a final answer is ready.

What it offers:
- the actual agent brain and execution loop
- message history handling
- iteration limits and interruption support
- provider-specific request shaping

How it improves the clone:
- This is the minimum viable Hermes-like core.
- Once this exists, every new component becomes additive instead of hypothetical.

Tasks:
- [x] Implement message model and conversation state
- [x] Implement the synchronous tool-calling loop
- [x] Add max-iteration and shared-budget logic
- [x] Add interrupt/cancel support
- [x] Add request normalization for different API modes
- [ ] Add session log persistence for debugging

Acceptance:
- A user prompt can trigger tools, consume tool results, and return a final answer reliably.

#### Component 3: Prompt Assembly and Context Injection

What it does:
- Builds the effective system prompt from base identity, memory, context files, skills metadata, platform hints, and temporary overlays.

What it offers:
- layered instructions
- workspace awareness
- personality overlays
- safer prompt injection boundaries

How it improves the clone:
- Gives the clone Hermes-like behavior instead of generic chat behavior.
- Makes project-specific and user-specific adaptation possible without code changes.

Tasks:
- [ ] Implement default identity/base instructions
- [ ] Add support for `AGENTS.md`-style workspace rules
- [ ] Add support for personality file overlays
- [ ] Add support for ephemeral/system prompt overrides
- [ ] Add prompt injection scanning for imported context files
- [ ] Add truncation rules for oversized context files

Acceptance:
- The same agent behaves differently across projects and user setups without changing code.

#### Component 4: Tool Registry and Toolset System

What it does:
- Registers tools, exposes schemas to the model, dispatches calls, groups tools into toolsets, and gates availability by requirements.

What it offers:
- centralized tool discovery
- consistent schema management
- per-platform and per-surface tool control
- extensible plugin-like growth

How it improves the clone:
- Makes tool growth manageable.
- Prevents tool sprawl from turning into routing chaos.
- Allows safe "small surface first, wider surface later" rollout.

Tasks:
- [ ] Define a tool registry contract
- [ ] Implement tool schema registration and dispatch
- [ ] Add requirement checks and environment gating
- [ ] Add named toolsets and composed toolsets
- [ ] Add platform-specific tool filtering
- [ ] Add error normalization for tool responses

Acceptance:
- Tools can be added without modifying the core loop beyond registration.

#### Component 5: Baseline Tools

What it does:
- Provides the first useful tool surface: web, files, process management, terminal, browser, memory, search, planning.

What it offers:
- the practical capabilities users actually feel
- composable workflows
- a base for coding and research use cases

How it improves the clone:
- Converts the runtime from "LLM wrapper" into a usable agent.
- Establishes the clone's real product value.

Tasks:
- [ ] Implement `read_file`, `write_file`, `patch`, `search_files`
- [ ] Implement `web_search` and `web_extract`
- [ ] Implement `process` and background process tracking
- [ ] Implement `todo`
- [ ] Implement `session_search`
- [ ] Implement `memory`
- [ ] Implement a browser automation surface

Acceptance:
- The agent can perform non-trivial research and coding workflows without manual operator glue.

#### Component 6: Terminal Runtime and Sandbox Backends

What it does:
- Gives the agent a real execution environment across local, container, remote, or cloud backends.

What it offers:
- command execution
- persistent shell state
- sandboxing and isolation
- backend flexibility by deployment context

How it improves the clone:
- This is the biggest differentiator for Hermes-like coding and ops work.
- Makes the agent operationally useful, not just conversationally useful.

Tasks:
- [ ] Implement a common `Environment` interface
- [ ] Implement local execution backend
- [ ] Implement Docker backend
- [ ] Implement SSH backend
- [ ] Implement one cloud sandbox backend
- [ ] Add persistent filesystem options
- [ ] Add cleanup, inactivity expiry, and interrupt handling
- [ ] Add command approval and guardrails

Acceptance:
- The same terminal tool can run against multiple backends with a consistent contract.

#### Component 7: CLI Product Surface

What it does:
- Provides the interactive user experience: terminal chat UI, slash commands, tool streaming, and session control.

What it offers:
- a usable primary surface
- command discovery
- live interaction with long-running work
- strong operator feedback

How it improves the clone:
- This is the fastest path to a compelling product.
- It turns the runtime into something people can actually use daily.

Tasks:
- [ ] Build the interactive CLI shell
- [ ] Add multiline input and slash command parsing
- [ ] Add session lifecycle commands
- [ ] Add streaming output and tool activity display
- [ ] Add retry/undo/reset flows
- [ ] Add config, model, tools, and skills commands

Acceptance:
- A user can run the clone comfortably from the terminal without touching config files directly.

#### Component 8: Persistence, Sessions, and Search

What it does:
- Stores sessions, enables retrieval of old conversations, and supports continuity across restarts and surfaces.

What it offers:
- durable conversation history
- searchable transcripts
- resumable sessions
- cross-surface continuity

How it improves the clone:
- Gives the system memory-like continuity even before richer memory layers.
- Enables debugging, analytics, and better user experience.

Tasks:
- [ ] Define session storage schema
- [ ] Persist message history and metadata
- [ ] Add full-text search over session logs
- [ ] Add session resume and listing
- [ ] Add usage/insights reporting

Acceptance:
- Users can return to prior sessions and recover relevant past work.

#### Component 9: Memory System

What it does:
- Stores compact durable facts across sessions and re-injects them into future runs.

What it offers:
- user preference persistence
- stable environment knowledge
- cross-session continuity

How it improves the clone:
- Reduces repeated steering.
- Makes the clone feel personalized and stateful instead of stateless.

Tasks:
- [ ] Define memory data model and storage
- [ ] Implement memory write/read tool behavior
- [ ] Add memory injection into prompt assembly
- [ ] Add memory size and compaction policies
- [ ] Add distinction between durable memory and session history

Acceptance:
- The agent remembers durable user and environment facts across sessions without bloating prompts.

#### Component 10: Skills System

What it does:
- Stores reusable instructions and workflows as skill documents that the agent can discover, apply, and update.

What it offers:
- procedural reuse
- domain specialization
- user-extensible behavior
- reduced prompt repetition

How it improves the clone:
- This is one of Hermes' strongest differentiators.
- Lets the clone improve over time without hard-coding every behavior.

Tasks:
- [ ] Define skill file format and metadata
- [ ] Implement skill discovery and listing
- [ ] Implement skill viewing and invocation hooks
- [ ] Implement skill create/patch flows
- [ ] Add skill compatibility gating by platform or environment

Acceptance:
- Users can add or evolve specialized capabilities without changing source code.

#### Component 11: Delegation and Multi-Agent Work

What it does:
- Lets the main agent spawn subagents with isolated context and task scope.

What it offers:
- decomposition of complex work
- parallelizable workflows
- smaller context windows per subtask

How it improves the clone:
- Increases throughput on large tasks.
- Moves the clone closer to Hermes' agentic orchestration model.

Tasks:
- [ ] Define subagent lifecycle and ownership model
- [ ] Implement isolated subagent context creation
- [ ] Implement `delegate_task`
- [ ] Add result collection and integration
- [ ] Add shared iteration budgeting across agents

Acceptance:
- Parent agent can hand off bounded work and consume the result without corrupting its own context.

#### Component 12: Execute-Code Runtime

What it does:
- Lets the agent write and run small scripts that call tools programmatically rather than spending many LLM turns on repetitive orchestration.

What it offers:
- lower context cost
- fewer round trips
- better multi-step automation

How it improves the clone:
- Makes repetitive workflows cheaper and faster.
- Reduces LLM overhead on deterministic tool pipelines.

Tasks:
- [ ] Define a safe script execution contract
- [ ] Expose a constrained RPC layer to selected tools
- [ ] Add execution logs and failure reporting
- [ ] Add sandboxing and timeout enforcement

Acceptance:
- The agent can collapse long repetitive workflows into one scripted tool run.

#### Component 13: Messaging Gateway

What it does:
- Delivers the same agent across Telegram, Slack, Discord, WhatsApp, Signal, email, or similar surfaces.

What it offers:
- multi-surface reach
- unified sessions
- home-channel delivery
- background accessibility

How it improves the clone:
- Makes the system operational outside the developer terminal.
- Enables the “agent works where you are” product story.

Tasks:
- [ ] Define platform adapter contract
- [ ] Implement session bridging across platforms
- [ ] Implement at least one messaging adapter first
- [ ] Add slash-command parity where possible
- [ ] Add media/file delivery support
- [ ] Add auth and allowed-user controls

Acceptance:
- A user can talk to the same agent outside the CLI and keep continuity.

#### Component 14: Scheduler and Automations

What it does:
- Runs prompts automatically on a schedule and delivers results to users or channels.

What it offers:
- unattended execution
- recurring reports and checks
- background work with delivery routing

How it improves the clone:
- Turns the clone from reactive assistant into proactive operator.
- Greatly expands utility for maintenance and monitoring tasks.

Tasks:
- [ ] Define job model and persistence
- [ ] Implement recurring scheduler
- [ ] Implement job execution with agent runtime
- [ ] Add job delivery targets
- [ ] Add pause/resume/trigger/list controls

Acceptance:
- Users can schedule recurring agent work without manual intervention.

#### Component 15: ACP / Editor Integration

What it does:
- Exposes the agent to editors and IDEs through a machine-facing protocol/server layer.

What it offers:
- coding workflow integration
- richer local context
- editor-triggered agent actions

How it improves the clone:
- Makes the clone competitive in coding workflows where IDE presence matters.
- Expands reach without duplicating core logic.

Tasks:
- [ ] Define ACP/editor session model
- [ ] Expose a coding-focused tool surface
- [ ] Add file and terminal coordination for editor contexts
- [ ] Add auth and permission boundaries

Acceptance:
- Editor clients can call into the agent safely and predictably.

#### Component 16: Research, Batch, and RL Infrastructure

What it does:
- Supports trajectory generation, evaluation environments, batch processing, and RL-style training loops.

What it offers:
- model evaluation
- training data generation
- benchmark harnesses
- experimental research workflows

How it improves the clone:
- Useful if the clone is also a research platform, not just a product.
- Lower priority for product parity, higher priority for platform ambition.

Tasks:
- [ ] Add batch runner for large prompt sets
- [ ] Add trajectory logging format
- [ ] Add benchmark/eval harnesses
- [ ] Add training environment abstractions

Acceptance:
- The clone can support repeatable evaluation or data generation workflows.

### In Progress

#### Component 17: Delivery Strategy and Milestones

What it does:
- Converts the component list into releaseable slices.

What it offers:
- clearer scope control
- reduced execution risk
- better sequencing

How it improves the clone:
- Prevents overbuilding advanced systems before the core is stable.

Tasks:
- [ ] Milestone 1: Foundation + runtime + registry + basic tools + CLI
- [ ] Milestone 2: Terminal backends + sessions + search + memory
- [ ] Milestone 3: Skills + delegation + execute-code
- [ ] Milestone 4: Messaging + automations
- [ ] Milestone 5: ACP + research stack

Definition of done per milestone:
- Each milestone must ship with tests, documentation, and one end-to-end demo workflow.

### Done

#### Component 18: Architectural Reconnaissance

What it does:
- Captures the subsystem map from the current Hermes repo.

What it offers:
- a concrete basis for cloning
- less guesswork

How it improves the clone:
- Keeps the plan grounded in Hermes' actual architecture rather than assumptions.

Completed observations:
- [x] Identified the core runtime in `run_agent.py`
- [x] Identified prompt assembly in `agent/prompt_builder.py`
- [x] Identified tool registration and dispatch layers
- [x] Identified terminal backends and sandbox abstractions
- [x] Identified CLI, gateway, cron, ACP, memory, skills, and research subsystems

## Suggested MVP Scope

If you want the fastest credible Hermes clone, start with this subset only:

- Foundation, config, provider layer
- Core agent runtime
- Prompt assembly
- Tool registry and toolsets
- File tools
- Web tools
- Terminal tool with local plus Docker backend
- CLI
- Session persistence and search
- Basic memory

That subset gives you a real Hermes-like product without the cost of gateway, delegation, ACP, or RL work.

## Suggested Post-MVP Priority

After MVP, add these next:

1. Skills system
2. Delegation
3. Scheduler/automations
4. Messaging gateway
5. ACP/editor integration

## Risks to Manage

- Tool surface grows faster than architecture discipline.
- Prompt layering becomes hard to reason about without clear precedence rules.
- Terminal/runtime abstractions become leaky across backends.
- Memory and skills can easily bloat prompts if compaction rules are weak.
- Messaging and scheduling add operational complexity early.

## Implementation Principle

Clone Hermes by preserving its contracts, not by copying every edge feature immediately.

The highest-value contracts to preserve are:

- layered system prompt construction
- central tool registry plus toolsets
- terminal-first execution model
- durable sessions plus memory
- skill-based extensibility
- optional multi-surface delivery

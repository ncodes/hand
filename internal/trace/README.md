# trace

The `trace` package records **structured agent session traces** as newline-delimited JSON ([JSON Lines](https://jsonlines.org/)). Each session maps to one file on disk; each line is one `Event` with a type string, UTC timestamp, session ID, and optional JSON payload.

## Purpose

Traces support debugging and inspection of a single chat run: what was sent to the model, what came back, tool calls, compaction/summary activity, and failures. Payloads are passed through the project’s **redactor** (`guardrails.Redactor`) before encoding, so secrets in maps and structs can be scrubbed consistently.

## Core types


| Type       | Role                                                                                    |
| ---------- | --------------------------------------------------------------------------------------- |
| `Factory`  | Creates a `Session` for a run (`NewSession(ctx, Metadata)`).                            |
| `Session`  | `ID()`, `Record(eventType, payload)`, `Close()`.                                        |
| `Metadata` | First-event payload: `agent_name`, `model`, `api_mode`, `source`, optional `trace_dir`. |
| `Event`    | JSON shape: `session_id`, `type`, `timestamp`, `payload`.                               |


## Factories

- `**NewFactory(directory, redactor)`** — If `directory` is non-empty and writable, each `NewSession` creates `<traceDir>/<UTC-timestamp>-<random>.jsonl`, writes `EvtChatStarted` with the given `Metadata`, and returns a real session. On failure or empty directory, returns a **noop** session (no file, `Record` is a no-op).
- `**NoopFactory()`** / `**NoopSession()`** — For tests or when tracing is disabled; no I/O.

## Event names

Event type strings are defined as constants in `events.go` (e.g. `EvtChatStarted = "chat.started"`). Below, **constant** refers to that name; **type string** is the wire value.

### Session lifecycle


| Constant           | Type string      | Meaning                                                                                                                   |
| ------------------ | ---------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `EvtChatStarted`   | `chat.started`   | Emitted automatically when a JSONL session starts. Payload: `Metadata` (agent, model, API mode, source, trace directory). |
| `EvtSessionFailed` | `session.failed` | Unrecoverable or traced error. Payload is typically `{"error": "<message>"}`.                                             |


### User message and model I/O


| Constant                    | Type string                | Meaning                                                                                |
| --------------------------- | -------------------------- | -------------------------------------------------------------------------------------- |
| `EvtUserMessageAccepted`    | `user.message.accepted`    | User input accepted for the turn. Payload includes `message`.                          |
| `EvtModelRequest`           | `model.request`            | Outgoing model request (serialized `models.Request`; sensitive fields redacted).       |
| `EvtModelResponse`          | `model.response`           | Model response (`models.Response`).                                                    |
| `EvtFinalAssistantResponse` | `final.assistant.response` | Final assistant text for the user. Payload includes `message` (assistant output text). |


### Tool calls


| Constant                     | Type string                 | Meaning                                                                              |
| ---------------------------- | --------------------------- | ------------------------------------------------------------------------------------ |
| `EvtToolInvocationStarted`   | `tool.invocation.started`   | A tool call is starting. Payload: `models.ToolCall`.                                 |
| `EvtToolInvocationCompleted` | `tool.invocation.completed` | Tool result message. Payload: `handmsg.Message` (name, content, tool call ID, etc.). |


### Summary fallback (iterations)


| Constant                    | Type string                | Meaning                                                                                                                           |
| --------------------------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `EvtSummaryFallbackStarted` | `summary.fallback.started` | Fallback path when the model must be invoked again (e.g. remaining iteration budget). Payload may include `remaining_iterations`. |


### Context estimation and usage (preflight / postflight)


| Constant                        | Type string                         | Meaning                                                                                                                                         |
| ------------------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `EvtContextPreflight`           | `context.preflight`                 | Pre-request estimate: token usage vs limits. Typical fields: `source`, `prompt_tokens`, `context_limit`, `trigger_threshold`, `warn_threshold`. |
| `EvtContextCompactionTriggered` | `context.compaction.triggered`      | Estimate crossed the compaction trigger threshold (same style of payload as preflight).                                                         |
| `EvtContextCompactionWarning`   | `context.compaction.warning`        | Estimate is in the warning band (may overlap with triggered).                                                                                   |
| `EvtContextPostflightUsage`     | `context.postflight.usage_recorded` | After a response, recorded usage: `source`, `prompt_tokens`, `completion_tokens`, `total_tokens`.                                               |


### Context compaction lifecycle


| Constant                        | Type string                    | Meaning                                                                                |
| ------------------------------- | ------------------------------ | -------------------------------------------------------------------------------------- |
| `EvtContextCompactionPending`   | `context.compaction.pending`   | Compaction queued / waiting.                                                           |
| `EvtContextCompactionRunning`   | `context.compaction.running`   | Compaction in progress.                                                                |
| `EvtContextCompactionSucceeded` | `context.compaction.succeeded` | Compaction finished successfully.                                                      |
| `EvtContextCompactionFailed`    | `context.compaction.failed`    | Compaction failed; payload includes error details alongside session/compaction fields. |


### Rolling summary (context summarization)


| Constant                | Type string                    | Meaning                                             |
| ----------------------- | ------------------------------ | --------------------------------------------------- |
| `EvtSummaryRequested`   | `context.summary.requested`    | A summary generation was requested for the session. |
| `EvtSummarySaved`       | `context.summary.saved`        | Summary persisted successfully.                     |
| `EvtSummaryFailed`      | `context.summary.failed`       | Summary step failed.                                |
| `EvtSummaryParseFailed` | `context.summary.parse_failed` | Model output could not be parsed as a summary.      |
| `EvtSummaryApplied`     | `context.summary.applied`      | Summary applied to the conversation store.          |


Payloads for summary and compaction events include session identifiers, offsets/counts, timestamps, and errors as applicable; see `internal/trace/inspect` for the JSON views used when rendering traces.

## Consumers

- `**cmd/trace`** — CLI over trace files.
- `**internal/trace/inspect`** — Loads JSONL sessions and maps event types into structured views for display.

## Stability

Event **type strings** are part of the on-disk format. New events may be added; existing type strings should remain backward compatible for old trace files. When adding events, define a constant in `events.go` and document it here.
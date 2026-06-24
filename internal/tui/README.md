# TUI Package

This package contains Morph's interactive terminal UI. It is built with Bubble
Tea, so the main mental model is:

```text
terminal input or async result
  -> Bubble Tea message
  -> model.Update
  -> app event, action, or effect
  -> tuiState changes
  -> model.View renders from state
```

The TUI should be understood as a state machine. Code should update state and
let `View` redraw the screen from that state.

## Entry Point

The CLI command is outside this package:

- `cmd/tui/tui.go` exposes the root TUI runner used by `morph`.
- `cmd/tui/program.go` loads config, creates the RPC client, builds the TUI
  model, and starts Bubble Tea.

The root app model is in `app/model.go`.

## Main Directories

- `app`: the interactive app, rendering, input handling, chat streaming, and
  transcript behavior.
- `composer`: parsing and history helpers for prompt input.
- `events`: small typed app events used inside the TUI.
- `layout`: terminal region sizing for transcript, composer, jump panel, and
  status row.
- `render`: shared render/theme primitives.
- `rpc`: adapters that convert RPC streaming into Bubble Tea messages.
- `state`: small state helpers shared by the app.
- `status`: status-line model.
- `transcript`: transcript text/document helpers.

## Runtime Flow

Startup:

```text
cmd/tui
  -> load config and RPC client
  -> app.NewModelWithClientContextAndConfig
  -> tea.NewProgram(model).Run
  -> model.Init
```

`model.Init` focuses the composer and starts loading the current session
timeline.

Input:

```text
keyboard/mouse/window message
  -> app/bubbletea_adapter.go Update
  -> handleKeyPressMsg or mouse handler
  -> handleAppEvent
  -> action or effect
```

Chat:

```text
submit prompt
  -> startResponse
  -> client.Respond(stream=true)
  -> agent/RPC events
  -> agentEventToTUIMessage
  -> applyTUIMessage
  -> transcript cell update
```

Hydration:

```text
model.Init
  -> loadSessionTimelineEffect
  -> client.GetSessionTimeline
  -> hydrateSessionTimeline
  -> sessionTimelineToTranscriptCells
```

## State, Actions, And Effects

`app/state.go` defines `tuiState`, which is the durable UI state for the
current screen: transcript messages, live response, command menu state, session
title, status, dimensions, and response flags.

`app/actions.go` contains synchronous state mutations. Use actions for local
state changes such as:

- adding or replacing transcript cells
- clearing the transcript
- changing session/title data
- resizing viewport dimensions
- marking response state

`app/effects.go` contains operations that leave the local state machine, such
as sending a prompt, copying to the clipboard, or loading timeline data.

Prefer this split:

```text
action = mutate local state
effect = do async or external work
```

## Rendering

The top-level render path is:

```text
model.View
  -> renderTranscript
  -> renderTranscriptComposerGap
  -> renderInput
```

Important files:

- `app/view.go`: top-level screen composition.
- `app/layout.go`: converts terminal dimensions into screen regions.
- `app/transcript.go`: builds transcript viewport content.
- `app/transcript_renderer.go`: renders transcript cells to terminal strings.
- `app/markdown.go`: renders Markdown assistant content.
- `app/chrome.go`: builds header and notice panel data.
- `app/chrome_renderer.go`: renders header and notice panel.
- `app/input.go`: renders and resizes the composer.
- `app/bottom_status_panel.go`: bottom status row.

Rendering should be derived from state. Avoid storing rendered strings as
long-lived state unless they represent an incremental stream buffer.

## Transcript Cells

Transcript content is modeled as `transcriptCell` values in
`app/transcript_cell.go`.

Current cell kinds include:

- user
- assistant
- reasoning
- thought
- tool
- safety
- error
- system
- compaction

The typical path is:

```text
TUI message
  -> tuiMessageToTranscriptCell
  -> appendTranscriptCellAction
  -> renderTranscriptContent
  -> viewport.SetContent
```

Use a new transcript cell when adding a new kind of durable transcript entry.
Use `live` state for content that is still streaming and should collapse into a
final assistant cell.

## Commands

Slash commands are defined in `app/commands.go`.

The menu is rendered and filtered in `app/command_menu.go`.

To add a command:

1. Add it to `slashCommandDefinitions`.
2. Handle it in `handleSlashCommand`.
3. Add tests for parsing, menu filtering, and command behavior when useful.

## Streaming And Trace Events

Streaming starts in `app/chat.go`.

Trace and text events are translated in `app/events.go`:

```text
agent.Event
  -> agentEventToTUIMessage
  -> traceEventToTUIMessage
  -> applyTUIMessage
```

If a trace event appears in logs but not in the TUI, check:

1. Whether the agent/RPC stream emits it.
2. Whether `traceEventToTUIMessage` handles it.
3. Whether `tuiMessageToTranscriptCell` can turn it into a cell.
4. Whether timeline hydration handles the same event path.

Live and hydrated rendering should usually share the same transcript cell
conversion path.

## Layout

The layout package computes rectangles for:

- transcript
- jump-to-bottom indicator
- composer
- bottom status panel

`app/layout.go` adapts those rectangles into app-local types.

When changing spacing or viewport size behavior, start with:

- `internal/tui/layout/layout.go`
- `internal/tui/app/layout.go`
- `internal/tui/app/view.go`
- `internal/tui/app/input.go`

## Where To Change Things

- Keyboard shortcuts: `app/bubbletea_adapter.go`
- Prompt submission: `app/composer.go`, `app/chat.go`
- Slash commands: `app/commands.go`
- Command menu: `app/command_menu.go`
- Transcript entries: `app/transcript_cell.go`, `app/transcript_renderer.go`
- Markdown output: `app/markdown.go`
- Tool display: `app/tool_display.go`, `app/tool_transcript_renderer.go`
- Header/notice bar: `app/chrome.go`, `app/chrome_renderer.go`
- Composer visuals: `app/input.go`
- Bottom status row: `app/bottom_status_panel.go`
- Session timeline hydration: `app/timeline.go`
- Copy behavior: `app/copy.go`
- Selection behavior: `app/selection.go`

## Testing

Most app behavior is covered by focused package tests in `internal/tui/app`.
Prefer testing the smallest layer that owns the behavior:

- parser behavior in `composer`
- layout math in `layout`
- state/action behavior in `app`
- rendering snapshots or stripped ANSI output in `app`
- timeline and trace conversion in `app/timeline_test.go`

For visual changes, add a renderer test that asserts plain text or stripped
ANSI output instead of depending on terminal-specific paint behavior.

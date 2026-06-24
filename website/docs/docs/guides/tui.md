---
title: TUI Guide
description: Use Morph's terminal chat interface.
---

# TUI Guide

The TUI is Morph's interactive terminal chat — the surface you will use most. It is a full-screen app with a scrolling
transcript, a multiline composer, and a status row, and it streams the agent's replies, tool activity, and reasoning
live as a turn runs.

This guide covers daily use: launching the TUI, composing and sending messages, watching a response, navigating the
transcript, and the slash commands. For the in-chat command list on its own, see
[Slash Commands](../reference/slash-commands); for how the interface is built, see [TUI Internals](../development/tui).

## Launching

Run Morph with no arguments to open the TUI:

```bash
morph
```

The TUI connects to the [daemon](../concepts/daemon-and-rpc) over RPC. If no daemon is running for the active
[profile](../concepts/profiles), Morph starts a temporary one for you and stops it again when you exit; if a daemon is
already running, the TUI just attaches to it. On startup it loads the timeline of the profile's current session, so you
resume exactly where you left off.

The bare `morph` command is the only one that opens the TUI. A one-shot, non-interactive request uses `morph --chat`
(or `-c`) instead, which prints a single reply and exits rather than entering the full-screen interface.

## Sending Messages

Type into the composer at the bottom and press **Enter** to send. While the composer has focus:

![Morph TUI composer](/img/page-images/composer.png)

- **Multiline input** — insert a newline without sending using **Shift+Enter**, **Alt+Enter**, or **Ctrl+J**. The
  composer grows to fit, so you can paste or write multi-paragraph prompts.
- **Pasting** is supported directly; pasted content is normalized into the composer.
- **Prompt history** — recall earlier prompts with **Ctrl+P** (previous) and **Ctrl+N** (next). While the prompt is a
  single line, **Up** and **Down** walk the same history; once it spans multiple lines, those keys move the cursor
  within the text instead.

Lines beginning with `/` are treated as [slash commands](#slash-commands) rather than messages, and a command menu
appears as you type.

## Watching a Response

When you send a message, Morph streams the turn into the transcript as it happens. Depending on the model and what the
agent does, you may see several kinds of entries:

![Morph TUI response with tool activity and final answer](/img/page-images/response.png)

- **Assistant** text, rendered as Markdown, streaming in as it is produced.
- **Reasoning** output, when the model exposes it, shown distinctly from the final answer.
- **Tool activity** — each tool call appears as it starts and updates when it completes, so you can follow what the
  agent is doing. See [Tools](../concepts/tools).
- **Safety notices**, when a guardrail blocks or redacts something. See [Safety and Guardrails](../concepts/safety-and-guardrails).
- **Compaction** markers, when the session summary is refreshed mid-conversation. See [Sessions](../concepts/sessions).

Press **Esc** to cancel a response that is in progress. When a turn finishes, the assistant entry is annotated with how
long it took — a "Worked for …" label — so you can see where each turn ended and how long it ran.

## Navigating the Transcript

The transcript scrolls independently of the composer:

- **Scroll** with the mouse wheel or the usual paging keys.
- **Jump to the bottom** with **Ctrl+End**, or by clicking the jump-to-bottom indicator that appears when you have
  scrolled up during a live response.
- **Select text** by dragging with the mouse, and **click links** to open them.
- **Copy** the whole transcript with **Ctrl+Y**, or with the `/copy` command.

## Slash Commands

Type `/` to open the command menu; keep typing to filter it, and use the arrow keys to choose. The available commands
are:

![Morph TUI slash command menu](/img/page-images/commandlist.png)

| Command | What it does |
| --- | --- |
| `/new-chat` | Start a new chat session |
| `/chats` | Show recent chat sessions |
| `/archive` | Show archived chat sessions |
| `/compact` | Compact the current session |
| `/clear` | Clear the transcript view |
| `/copy` | Copy the transcript |
| `/models` | Show supported models |
| `/providers` | Show supported model providers |
| `/setup` | Open setup |
| `/changelog` | Show the latest changelog entry |

A few of these open an interactive panel rather than acting immediately — `/chats` and `/archive` list sessions you can
switch to or act on, and `/setup`, `/models`, and `/providers` drive model configuration. Inside those panels, **Esc**
closes the panel and **Ctrl+Y** copies its contents. Note that `/clear` only clears what is displayed; it does not
delete the session, whose history remains in the store. See [Slash Commands](../reference/slash-commands) for the full
reference.

## Sessions in the TUI

The TUI always operates on the profile's current session, and switching sessions reloads the transcript from stored
history:

- `/new-chat` starts a fresh session and makes it current.
- `/chats` lists recent sessions to switch between; `/archive` lists archived ones.
- `/compact` summarizes the current session on demand when a conversation has grown long.

Because history lives in the daemon's store, the conversation you see is the same one any other client attached to that
profile would see. For the underlying model — identity, summaries, archiving — see [Sessions](../concepts/sessions),
and for the command-line equivalents see the [Session Guide](./sessions).

## Setup and Models

On first run, or when credentials are missing, the TUI walks you through naming and model setup so you can start
chatting without editing config by morph. You can reopen this later with `/setup`, and inspect what is available with
`/models` and `/providers`. For configuring provider credentials in depth, see [Provider Auth](./provider-auth).

## Exiting

Press **Ctrl+C** to exit; press it again to confirm. If the TUI started a temporary daemon for this run, that daemon is
stopped as you leave; a daemon that was already running keeps running.

## Keybindings

| Key | Action |
| --- | --- |
| Enter | Send the message |
| Shift+Enter / Alt+Enter / Ctrl+J | Insert a newline |
| Ctrl+P / Ctrl+N | Previous / next prompt in history |
| Up / Down | Prompt history (single-line prompt) or move within the command menu / cursor |
| Esc | Cancel the in-progress response, or close an open panel |
| Ctrl+End | Jump to the bottom of the transcript |
| Ctrl+Y | Copy the transcript (or the open panel's contents) |
| Ctrl+C | Exit (press again to confirm) |

## Where To Go Next

- [Slash Commands](../reference/slash-commands): the full in-chat command reference.
- [Sessions](../concepts/sessions) and the [Session Guide](./sessions): manage conversations.
- [Daemon and RPC](../concepts/daemon-and-rpc): the process the TUI talks to.
- [Provider Auth](./provider-auth): set up provider credentials.
- [Tools](../concepts/tools): what the tool activity in the transcript represents.
- [TUI Internals](../development/tui): how the interface is implemented.

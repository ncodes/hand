---
title: Session Guide
description: Manage persisted chat sessions.
---

# Session Guide

A session is one durable conversation: its messages, summary, title, and context usage, stored in the active
[profile](../concepts/profiles) so it survives restarts and is shared by every client on that profile. This guide
covers the day-to-day commands for listing, switching, inspecting, and maintaining sessions. For the underlying model
(identity, summaries, archiving, and search) see [Sessions](../concepts/sessions).

The `morph session` subcommands talk to a running [daemon](../concepts/daemon-and-rpc) over RPC; they do not start one.
If you see a connection error, start a daemon first: run `morph daemon`, or keep a TUI (`morph`) open, which runs
one for as long as it is open. All of them accept a global `--profile` to target a profile other than the current one,
for example `morph --profile work session list`.

## Listing and Switching

See your sessions, which one is current, and switch between them:

```bash
morph session list
morph session current
morph session use ses_abc123
```

`list` prints each session as `Title (id)` (or just the id when it has no title yet). `current` prints the id of the
active selection: the session the TUI and one-shot chats use when you do not name one. `use` makes an existing,
non-archived session current.

## Creating Sessions

Create a new session, optionally with your own id:

```bash
morph session new
morph session new ses_review
```

`new` prints the new session's id but does **not** make it current: create-then-switch is two steps, so follow it with
`morph session use <id>` if you want to continue there interactively. The special `default` session always exists and is
the fallback when nothing else is selected.

To continue a specific session in a one-shot request without changing your current selection, pass `--session`:

```bash
morph --chat --session ses_review "pick up where we left off"
```

## Inspecting Context Usage

A session accumulates context as it grows. `status` reports how full it is:

```bash
morph session status
morph session status ses_review
```

It shows the created and updated times, the current compaction state, and the context budget: how much of the model's
context window is used versus remaining, as both token counts and percentages. Watch the used percentage to see when a
conversation is approaching the point where Morph will compact it automatically.

## Compacting

Compaction summarizes older messages so a long conversation keeps working without resending everything. Morph does this
automatically as context fills, but you can force it:

```bash
morph session compact
morph session compact ses_review
```

The command reports the resulting summary boundary and the new context lengths. Persisted history is not deleted;
compaction only trims the live working set. See [Sessions](../concepts/sessions#summaries-and-compaction) for how
summaries and thresholds work, and note that when memory is enabled Morph flushes durable [memory](./memory) before an
automatic compaction.

## Archiving

Archiving removes a session from your active list while keeping its content, and Morph permanently deletes archives after
a retention period. Archiving a session is done from the TUI: the `/chats` panel can archive the session you select,
and `/archive` lists archived ones. The `default` session cannot be archived.

From the CLI you can restore an archived session:

```bash
morph session unarchive ses_old
```

See the [TUI Guide](./tui) for the archive panels and [Sessions](../concepts/sessions#archiving-sessions) for retention
behavior.

## Repairing Search Artifacts

If session search results look stale or incomplete, rebuild a session's search/index artifacts:

```bash
morph session repair
morph session repair ses_review --full
```

By default `repair` only rebuilds missing or stale artifacts; `--full` rebuilds everything repairable. It prints how
many rows were scanned, rebuilt, and so on. See [Search and Traces](./search-and-traces) for how session search works.

## Sessions Across Surfaces

Because sessions live in the daemon's store, every client on the profile sees the same conversations and the same
current selection: the TUI, one-shot `morph --chat`, and the `morph session` commands all agree. Messaging
[gateways](../concepts/gateways) reuse the same sessions too: each external conversation is bound to its own session, so
a Slack or Telegram thread keeps a continuous history without disturbing the current session your terminal uses.

## Where To Go Next

- [Sessions](../concepts/sessions): the conceptual model behind identity, summaries, and archiving.
- [TUI Guide](./tui): switch, compact, and archive sessions interactively.
- [Search and Traces](./search-and-traces): find past work across sessions.
- [Memory](./memory): durable knowledge that complements session history.
- [Gateways](../concepts/gateways): how external conversations map to sessions.
- [Daemon and RPC](../concepts/daemon-and-rpc): the process these commands talk to.

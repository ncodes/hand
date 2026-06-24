---
title: Sessions
description: Durable conversation continuity.
---

# Sessions

Sessions store durable conversation history and support continuity across restarts and surfaces. A session holds the
ordered messages of a conversation, a rolling summary, and metadata such as its title and context usage. Sessions live
in the active profile's store, so they persist after the daemon stops and are shared by every client that connects to
that profile.

This page explains the session model. For the commands and daily workflows, see the
[Session Guide](../guides/sessions). For where session data physically lives, see [Profiles](./profiles).

## Session Identity and the Current Session

Every session has an id. New sessions get a generated id with a `ses_` prefix followed by a short random identifier (for
example `ses_abc123...`). There is also one special, persistent session with the fixed id `default`.

- The **default session** always exists. It is created on first use, cannot be archived or deleted, and is the fallback
  when no other session is selected. It is ideal for quick, ongoing work that you do not need to name or track.
- The **current session** is the active selection for a profile. It is stored in the profile's state, so the TUI and CLI
  agree on which conversation is current. When nothing is selected, the current session is `default`.

Selecting a session with `morph session use` only switches a target that already exists and is not archived. A one-shot
chat can target a specific session for a single request without changing the current selection.

To set current session:

```bash
morph session use <session-id>
```


To continue a specific session from a one-shot prompt:


```bash
morph --session <session-id> -c "Continue from there."
```

## Message History

Each session stores its messages in order in the profile's SQLite store: user, assistant, and tool messages, including
the assistant's tool calls and tool results. This durable history is what lets a conversation resume exactly where it
left off after a restart, and it is the source that the timeline and search read from.

## Summaries and Compaction

A long conversation eventually approaches the model's context window. **Compaction** keeps it workable by summarizing
older messages into a structured session summary while keeping the most recent messages intact, so the agent retains
the gist of earlier turns without resending every message.

- **Automatic compaction** is evaluated as context usage grows. When prompt size crosses a configured fraction of the
  context window (about 85% by default, with a warning near 95%), Morph refreshes the summary using the summary model.
- **Memory flush first.** When memory is enabled, Morph flushes durable memory before auto-compaction so useful facts are
  written out before older messages drop out of the live context. See [Memory](./memory).
- **History is not deleted.** Compaction summarizes and trims the *in-memory* working history to the summarized point; the
  persisted messages remain in the store and stay searchable.
- **Manual compaction** (`morph session compact`) forces a summary regardless of the usage threshold and reports the
  resulting context metrics.

The summary model is configured separately from the main model; see [Provider Auth](../guides/provider-auth).

## Titles

A session has a title and a title source. Auto-generated titles are produced from the early messages of a conversation
using the summary model once the conversation has enough content; their source is recorded as generated. Renaming a
session sets a manual title, and an existing title is never overwritten by auto-generation.

## Archiving Sessions

Archiving removes a session from your active list without losing its content:

- **Archive** marks a session archived and records an expiry. It requires at least one message, and it cannot archive
  the `default` session. If the archived session was current, the current selection is cleared.
- **Unarchive** restores an archived session to the active list.
- **Retention.** Archived sessions are kept for a retention period (30 days by default) and then permanently deleted by a
  background maintenance process, which removes their messages, search rows, summaries, and vectors. Unarchiving before
  then preserves the session.

## Gateway Conversations as Sessions

Messaging gateways reuse the same session model. A Slack or Telegram conversation is bound to a Morph session by
conversation and thread, not by individual sender, so an ongoing channel thread keeps a continuous history. The binding
is created the first time a conversation is seen and reused afterward. Gateway activity does not change the current
session used by the TUI and CLI. See [Gateways](./gateways).

## Search and Timeline

Because history is durable, you can find and revisit past work:

- **Search** runs full-text search over message content and can also use vector similarity with reranking when vector
  search is configured, including across sessions. See [Search and Traces](../guides/search-and-traces).
- **Timeline** hydrates a session for display: paginated messages together with the trace events recorded around them,
  which is how a client rebuilds a transcript view.
- **Repair** rebuilds the vector index for a session when search results look stale or incomplete (`morph session repair`).

The agent can reach this same history mid-conversation through the `session_search` and `session_messages`
[tools](./tools), so it can look back over stored messages without you resending them.

## Where To Go Next

- [Session Guide](../guides/sessions): create, list, switch, compact, repair, unarchive, and inspect sessions.
- [Profiles](./profiles): how session state is isolated per profile.
- [Memory](./memory): how durable memory complements session history.
- [Search and Traces](../guides/search-and-traces): finding past work and inspecting agent activity.
- [Daemon and RPC](./daemon-and-rpc): how a reply is streamed and tied to a session.
- [Architecture](./architecture): where sessions sit in the overall runtime.

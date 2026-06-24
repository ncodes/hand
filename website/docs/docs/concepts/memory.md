---
title: Memory
description: Durable knowledge beyond one transcript.
---

# Memory

Morph memory is separate from session history. Session history is the raw transcript of one conversation; memory is a
curated, durable store of facts, preferences, and useful context that can apply across sessions. Memory lives in the
active profile's store, so what Morph learns in one conversation can help in another.

This page explains the memory model. For enabling and tuning memory, see the [Memory Guide](../guides/memory); for how
it relates to a conversation's transcript, see [Sessions](./sessions).

## Memory Items and Their Lifecycle

Memory is stored as discrete **items**, each with a kind, a status, text, optional tags and metadata, provenance links
back to the sessions it came from, and a confidence score.

Kinds describe what an item is:

- **pinned** — always-in-context facts (see below).
- **semantic** — durable facts and preferences.
- **episodic** — notable events distilled from conversations (decisions, outcomes, blockers).
- **procedural** — how-to knowledge.

Status describes where an item is in its lifecycle:

- **candidate** — newly written, not yet trusted for use.
- **active** — promoted and eligible to be retrieved into prompts.
- **superseded** / **deleted** — replaced or removed.

This candidate-to-active distinction is central: most writes start as candidates, and only **active** items are ever
retrieved into a prompt. Promotion (described below) is what moves a candidate to active.

## Pinned Memory

Pinned memory is the always-nearby context. It comes from two sources: a profile-local `memory.md` file in the profile
home, and any active items of kind `pinned` in the store. Unlike retrieval, pinned memory is query-independent — it is
loaded at the start of every turn rather than searched for, so it is the place for facts that should always be in view.

Pinned content is budgeted so it cannot crowd out the conversation: the profile config sets overall and per-item
character caps, and the agent applies its own hard limits when injecting pinned memory into a turn. Pinned loading can be
turned off independently of the rest of memory.

## Retrieval at Turn Start

Before the model runs, Morph assembles a memory context for the turn:

1. **Pinned** memory is loaded regardless of the message, since it is not searched for.
2. **Semantic retrieval** searches active `semantic`, `episodic`, and `procedural` items for ones relevant to the user's
   message, keeping only a small number of high-scoring hits.
3. The selected items are sanitized (PII redaction and a safety scan), then rendered into a single instruction block.

That block is appended to the system instructions for the turn — it is part of how the prompt is assembled, not a user
message. The number of items, per-item size, a minimum relevance score, and an overall budget all bound how much memory
can enter a prompt, so retrieval stays focused and small. See [Prompt Assembly](../development/prompt-assembly) for how
instructions are composed.

## Episodic, Reflection, and Promotion

Beyond what the agent writes directly, Morph can build memory in the background. These run as loops inside the daemon (not
per request), and each is governed by its own config; depending on configuration, the extraction loops may be off until
you enable them.

- **Episodic extraction** reads bounded windows of an idle session's messages and uses a model to distill notable
  episodic items as candidates, with provenance back to the source messages. A session becomes eligible once it has
  enough messages and has been idle for a configured interval.
- **Reflection** consolidates unreflected episodic items into new, higher-level candidate memories, reusing related
  context to avoid duplicates, and marks the sources as reflected.
- **Promotion** evaluates candidates against a policy — requiring provenance, no conflicts, and a confidence threshold —
  and promotes those that pass to active. Rejected candidates are marked evaluated so they are not reconsidered
  endlessly.

Episodic and reflection use the summary model when they call a model; see [Provider Auth](../guides/provider-auth).

## How Memory Is Written

There are two ways memory gets written, and both feed the same candidate-then-promote lifecycle:

- **Directly during a turn.** When memory write is enabled and the memory capability (`cap.memory`) is on, the agent can
  call memory tools (`memory_add`, `memory_update`, `memory_delete`) as part of its work. A direct add records a
  candidate and immediately runs promotion. See [Tools](./tools).
- **Through a flush pass.** A separate, bounded pass driven by the summary model writes memory at key moments rather than
  mid-turn: **before automatic compaction** (so useful facts are saved before older messages leave the live context),
  on **controlled shutdown**, and when a session is **manually compacted**. The flush pass is limited in calls, output
  size, and time. See [Sessions](./sessions) for compaction.

## Safety Before Memory Enters Prompts

Memory is guarded on both sides. Before an item is stored, its text is safety-scanned and rejected if it trips a rule.
Before stored memory is placed into a prompt, it is redacted for sensitive data and scanned again, and any item that
fails is dropped and recorded to traces. This keeps unsafe or sensitive content from silently persisting or re-entering
the model. See [Safety and Guardrails](./safety-and-guardrails).

## Enablement and Storage

Memory is enabled by default, and pinned memory, turn-start retrieval, the flush pass, and direct writes work out of the
box. The background extraction processes and various limits are configurable per profile. Memory items are persisted in
the profile's SQLite store with full-text search, and with vector similarity and reranking when vector search is
configured. For the exact flags and defaults, see the [Memory Guide](../guides/memory) and
[Config Reference](../reference/config).

## Where To Go Next

- [Memory Guide](../guides/memory): enable, tune, and troubleshoot memory.
- [Sessions](./sessions): how memory complements durable conversation history.
- [Profiles](./profiles): memory is isolated per profile.
- [Tools](./tools): the memory capability and write tools.
- [Safety and Guardrails](./safety-and-guardrails): the scans applied to memory.
- [Memory System](../development/memory-system): the implementation-level design.

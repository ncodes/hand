---
title: Learning Path
description: Pick the right next page for your goal.
---

# Learning Path

Hand's documentation covers setup, daily usage, operations, internals, and reference material. This page routes you to
the right pages based on what you are trying to do, so you can skip the sections that do not apply yet.

Pick the path that matches your goal. Each path is ordered: follow it top to bottom, or jump to the page you need.

## New User

Start here if you have not run Hand yet and want a working first conversation.

1. [Quickstart](./quickstart): install Hand, choose a provider, store credentials, and send a first message.
2. [Installation](./installation): source builds, platform notes, and update or uninstall steps.
3. [First Chat](./first-chat): the conversation workflow in detail, including tool activity and one-shot prompts.
4. [Profiles and Config](./profiles-and-config): where profile state lives and how config sources combine.
5. [TUI Guide](../guides/tui): the terminal chat surface you will use every day.

If something does not work, jump to [Doctor](../operations/doctor) and [Troubleshooting](../guides/troubleshooting).

## Daily Terminal User

Start here once Hand runs and you want to use it well from the terminal.

1. [TUI Guide](../guides/tui): keybindings, panes, and the interactive workflow.
2. [Sessions Guide](../guides/sessions): continue, list, and switch conversations.
3. [Slash Commands](../reference/slash-commands): in-chat commands for fast control.
4. [Memory Guide](../guides/memory): how Hand remembers context across sessions.
5. [Search and Traces](../guides/search-and-traces): find past work and inspect what the agent did.
6. [CLI Reference](../reference/cli): every command and flag for scripting and quick answers.

For the model behind your chats, see [Provider Auth](../guides/provider-auth) and [Config Guide](../guides/config).

## Gateway Operator

Start here if you want to reach Hand from Slack, Telegram, or a generic HTTP client.

1. [Gateways Concept](../concepts/gateways): what a gateway is and how messages flow.
2. [Gateway Overview](../guides/gateway/): enable and configure the external gateway.
3. Pick your transport: [Telegram](../guides/gateway/telegram), [Slack](../guides/gateway/slack), or
   [Generic HTTP](../guides/gateway/generic-http).
4. [Pairing and Allowlists](../guides/gateway/pairing-and-allowlists): control who can talk to your agent.
5. [Gateway Management](../operations/gateway-management): run, monitor, and troubleshoot the gateway.
6. [Security](../operations/security): credentials, exposure, and safe defaults for a reachable agent.

The [Gateway Routes](../reference/gateway-routes) reference lists the HTTP endpoints.

## Memory and Session Power User

Start here if you want to tune how Hand remembers, retrieves, and retains conversations.

1. [Sessions Concept](../concepts/sessions): the durable session model and identity.
2. [Memory Concept](../concepts/memory): episodic, reflection, and promotion behavior.
3. [Memory Guide](../guides/memory): enable and shape memory in practice.
4. [Search and Traces](../guides/search-and-traces): vector search, reranking, and trace inspection.
5. [Config Reference](../reference/config): the `memory`, `search`, `reranker`, `compaction`, and `session` keys.
6. [Backups and State](../operations/backups-and-state): protect and move profile state.

Use [Profiles and Config](./profiles-and-config) to isolate experiments in a separate profile.

## Contributor

Start here if you want to read or change Hand's code.

1. [Developer Architecture](../development/architecture): package boundaries and how the pieces fit.
2. [Agent Loop](../development/agent-loop): how a request becomes model calls and tool steps.
3. [Prompt Assembly](../development/prompt-assembly): how prompts are built before each turn.
4. Subsystem internals: [Model Providers](../development/model-providers),
   [Tools Runtime](../development/tools-runtime), [Session Storage](../development/session-storage),
   [Memory System](../development/memory-system), [Gateway Internals](../development/gateway-internals), and
   [TUI](../development/tui).
5. [Testing](../development/testing): how to run and write tests.
6. [Contributing](../contributing): workflow and expectations for changes.

The [RPC](../reference/rpc) and [Trace Events](../reference/trace-events) references document the daemon's interfaces.

## Still Not Sure

If you do not know which path fits, read the [documentation home](/) for the high-level map, or check the
[FAQ](../reference/faq) for common questions.

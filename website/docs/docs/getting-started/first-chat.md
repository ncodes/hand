---
title: First Chat
description: Send your first message through Hand.
---

# First Chat

This page focuses on the first successful interaction with Hand. It assumes you already installed the CLI, selected a
profile, chose a model provider, and stored credentials.

If you have not done that yet, start with the [Quickstart](./quickstart).

## Check Readiness

Before sending a message, check the active profile:

```bash
hand doctor
hand auth status
```

You are ready when `hand doctor` exits cleanly and `hand auth status` shows credentials for the provider in
`models.main.provider`.

If the provider, model, or API looks wrong, check the active config:

```shell-session
$ hand config get models.main.provider models.main.name models.main.api
models.main.provider=openai-codex
models.main.name=gpt-5.5
models.main.api=openai-responses
```

If the config is wrong, return to [Choose A Provider](./quickstart#choose-a-provider). If credentials are missing,
return to [Store Credentials](./quickstart#store-credentials).

## Start The Daemon

The daemon owns the long-running agent runtime. The TUI can start it for you, so the simplest path is:

```bash
hand
```

When the TUI starts the daemon, it runs the daemon inside the same `hand` process. Exiting the TUI stops that daemon.

If you want to see daemon startup details in the foreground, start it manually in one terminal:

```bash
hand daemon start
```

Then open a second terminal and launch the TUI:

```bash
hand
```

## Send A Message In The TUI

In the TUI, type a small prompt and press Enter:

```text
Say hello in one sentence.
```

Use a prompt that does not need files, web access, or tools. A greeting is enough to prove the profile, daemon,
credentials, model, and streaming path work.

Success looks like streamed assistant text followed by a saved transcript entry. Longer responses and tool steps show
completion labels with elapsed time.

## Understand Tool Activity

Your first message does not need tools, but later prompts may trigger them. Tool activity appears as timeline entries
around the assistant response. For example, a prompt that asks Hand to inspect files may show file or process activity
before the final answer.

Tool labels are status markers, not extra instructions you need to answer. A running label means Hand is still working.
A finished label means that specific tool step completed and the assistant can continue with the result.

If a tool fails, read the nearby error text first. Common causes are missing filesystem permissions, a command that is
not installed, disabled tool capability flags, or a prompt that requires credentials you have not configured.

## Send A One-Shot Prompt

Use a one-shot CLI prompt when you want a quick answer without opening the TUI:

```bash
hand --chat "Say hello in one sentence."
```

`hand --chat` sends the message through the daemon and prints the final answer in the terminal. If no daemon is running,
Hand starts one for the request and stops it when the response is complete. When streaming is enabled, text is printed as
it arrives and ends with a newline.

To use a specific profile:

```bash
hand --profile default --chat "Summarize what you can do."
```

To continue a known session from the CLI:

```bash
hand --chat --session "<session-id>" "Continue from there."
```

For everyday back-and-forth work, prefer the TUI. For scripts, checks, and quick questions, prefer `hand --chat`.

## Confirm The Chat Was Saved

Hand stores conversations as sessions. After your first response, list recent sessions:

```bash
hand session list
```

Show the current session:

```bash
hand session current
```

If you used the TUI, reopening `hand` with the same profile should hydrate the recent transcript so you can continue.

## If The First Chat Fails

Use the error message to narrow the problem:

- `connection refused`: the configured RPC endpoint could not be reached. Check custom RPC address or port settings.
- Authentication errors: the selected provider is missing credentials or the credential belongs to a different provider.
  Run `hand config get models.main.provider models.main.name models.main.api` and `hand auth status`.
- Model errors: the configured model name or API mode does not match the provider. Recheck the provider examples in the
  [Quickstart](./quickstart).
- No visible streaming: streaming may be disabled in config. Check the daemon startup details or run
  `hand config get models.main.stream`.

When in doubt, run:

```bash
hand doctor
```

Then retry the smallest possible prompt:

```bash
hand --chat "Say hello."
```

## Next Step

Once your first message works, continue with the [TUI Guide](../guides/tui) for daily terminal usage or
[Session Guide](../guides/sessions) for continuing previous conversations.

---
title: Automation Guide
description: Create, manage, and deliver scheduled automation jobs.
---

# Automation Guide

This guide walks through creating, inspecting, updating, and removing scheduled automation jobs, then delivering
their output somewhere useful. For the underlying model (jobs, runs, and why "ran" and "delivered" are different
claims), see [Automation](../concepts/automation). For every flag and enum value, see
[Automation Reference](../reference/automation).

The `morph automation` subcommands talk to a running [daemon](../concepts/daemon-and-rpc) over RPC, the same one the
TUI uses; they do not start one. If you see a connection error, run `morph daemon` or keep a TUI (`morph`) open.

## Check Readiness

Before creating jobs, confirm the daemon and automation scheduler are healthy:

```bash
morph doctor
```

Look for the **automation** group. A healthy scheduler shows its store and scheduler checks passing; see
[Automation Operations](../operations/automation) if anything is missing or failing. You can also check the live
scheduler state directly:

```bash
morph automation status
```

This reports whether the scheduler is running, how many jobs exist, how many are currently running, and when it
next wakes up.

## Create a Recurring Job

A recurring job uses `every <duration>`:

```bash
morph automation add \
  --name "Five minute check" \
  --schedule "every 5m" \
  --prompt "Note anything urgent from the last five minutes."
```

The command prints the new job's id:

```text
auto_<job-id>
```

Confirm it, including its computed next run time:

```bash
morph automation list
```

```text
ID             NAME                ENABLED  SCHEDULE     NEXT RUN               LAST STATUS
auto_<job-id>  Five minute check   true     every 5m0s   2026-07-11T18:35:00Z   -
```

Nothing has run yet, so `LAST STATUS` shows `-`. `SCHEDULE` echoes back your interval with an explicit unit for every
part (`5m0s` for an input of `5m`). For cron jobs, `NEXT RUN` is shown in the schedule's effective timezone; schedules
without a timezone fall back to UTC.

## Create a One-Shot Job

A one-shot job runs once, at an absolute time. Use an RFC3339 timestamp:

```bash
morph automation add \
  --name "One-time reminder" \
  --schedule "2026-07-11T18:30:00Z" \
  --prompt "Follow up on the release notes."
```

:::warning[A bare duration is *not* a one-shot delay]
`--schedule "30m"` (without the `every` prefix) is parsed as a **recurring** `every 30m` job, not "run once in 30
minutes." There is no relative one-shot syntax: a one-shot job always needs an absolute timestamp. Compute the
timestamp yourself if you want "run once, N minutes from now."
:::

After it fires once, the job has no further next run; it stays in `morph automation list` with `NEXT RUN` shown as
`-` unless you set `--delete-after-run` at creation time to have it clean itself up after a successful run.

## Create a Cron Job With a Timezone

Cron schedules use standard five-field cron syntax, optionally prefixed with `TZ=<zone>`:

```bash
morph automation add \
  --name "Morning briefing" \
  --schedule "TZ=America/New_York 0 9 * * *" \
  --prompt "Give me a quick briefing to start the day."
```

:::note[The input has no `cron` keyword]
Provide the five fields (and optional `TZ=` prefix) directly, for example `"0 9 * * *"` or
`"TZ=America/New_York 0 9 * * *"`. `morph automation list` displays cron schedules with a `cron` prefix for
readability (`cron 0 9 * * *`), but that prefix is *display-only* and isn't valid input.
:::

Without an explicit `TZ=` prefix, the daemon's configured default timezone applies, falling back to the server's
local system timezone. See [Automation](../concepts/automation#schedules) for the full resolution order.

## List Jobs and Run History

List every enabled job, or include disabled ones:

```bash
morph automation list
morph automation list --all
morph automation list --limit 20
```

List runs, optionally filtered to one job or a set of statuses:

```bash
morph automation runs --job auto_<job-id>
morph automation runs --status error,skipped --limit 10
```

For a single job's last run plus its most recent failures in one call, use `inspect`:

```bash
morph automation inspect auto_<job-id>
```

`inspect` and the rest of the diagnostic toolkit (`diagnose`, `recover`) are covered in
[Automation Operations](../operations/automation); this guide only needs `list` and `runs` for everyday use.

## Update a Job Without Losing Other Settings

Update only the fields you name; everything else on the job is left exactly as it was:

```bash
morph automation update auto_<job-id> --schedule "every 10m"
```

:::tip[Omitted flags are preserved, not cleared]
This job's prompt, delivery configuration, and every other setting are untouched by the command above; only the
schedule changes. The same applies to nested payload and delivery fields: passing `--model` alone doesn't reset
`--prompt`, and passing `--channel` alone doesn't reset an existing `--target`. You only need to repeat a flag when
you actually want to change its value.
:::

## Pause, Resume, and Run Now

Pausing and resuming toggle whether a job is scheduled, without touching its definition or run history:

```bash
morph automation pause auto_<job-id>
morph automation resume auto_<job-id>
```

Trigger a run immediately, independent of its schedule:

```bash
morph automation run auto_<job-id>
```

If the job is already running or the daemon is at its concurrency limit, `run` waits for a free slot rather than
failing outright.

## Remove a Job

```bash
morph automation remove auto_<job-id>
```

This deletes the job definition. Its run history is not automatically deleted with it, though it will eventually age
out under normal run-history retention; see [Automation Operations](../operations/automation).

## Manage Automations Conversationally

You don't need the CLI at all: ask Morph directly, in the TUI or over a gateway, and it uses the same owner-only
automation tool under the hood.

| What you say | Tool action |
| --- | --- |
| "Set up a job that checks X every 30 minutes" | `add` |
| "Show my automation jobs" | `list` |
| "Change that job to run every hour instead" | `update` |
| "Pause the five minute check job" | `pause` |
| "Resume it" | `resume` |
| "Run that job now" | `run` |
| "Remove the one-time reminder job" | `remove` |
| "What did that job's last few runs look like?" | `runs` |

Asking the agent to make a job reply back into the same chat, for example:

> "Remind me here in 2 hours to check the deploy."

works with **`local`** delivery, not `origin` delivery, even though the request sounds like "reply to the origin of
this chat." From a TUI session there's no Slack or Telegram channel to send to, so `origin` delivery (like `gateway`
delivery) still needs one explicitly. If you ask for `origin` delivery by name from a TUI chat, it fails with:

```text
automation origin delivery requires a Slack or Telegram channel and target;
TUI sessions should use local delivery with an explicit session target
```

## Delivery Examples

Delivery is separate from execution: a run can finish successfully and still fail to deliver. See
[Automation](../concepts/automation#delivery-is-a-separate-outcome-from-execution) for the model. Every mode below
has a worked example; `--delivery` defaults to `none` when omitted.

### None

The default. Nothing is delivered beyond the run record itself:

```bash
morph automation add \
  --name "Local-only check" \
  --schedule "every 30m" \
  --prompt "Quick status note; nothing needs to go anywhere."
```

Check its output with `morph automation runs --job auto_<job-id>`: the `DELIVERY` column reads `not_requested`.

### Local

Functionally identical to `none` (nothing external happens either way), but recorded differently:

```bash
morph automation add \
  --name "Daily summary" \
  --schedule "TZ=America/New_York 0 9 * * *" \
  --prompt "Summarize what happened yesterday." \
  --delivery local
```

:::tip["local" still counts as delivered]
The only observable difference from `none` is the recorded delivery status: `local` shows `delivered`, `none` shows
`not_requested`. Neither one sends anything anywhere; both just rely on the persisted run record.
:::

### Origin

Delivers back to the channel/thread a job's context came from. From the CLI there's no active chat to capture that
from automatically, so name it explicitly, exactly like `gateway` delivery below:

```bash
morph automation add \
  --name "Reply here later" \
  --schedule "2026-07-11T18:00:00Z" \
  --prompt "Remind me about the follow-up." \
  --delivery origin \
  --channel telegram \
  --target <chat-id>
```

Automatic origin capture (no `--channel`/`--target` needed) only happens when the agent creates a job for you from
inside a live gateway conversation; see [Manage Automations Conversationally](#manage-automations-conversationally).

### Gateway: Telegram

:::note[`--channel` names the platform, not a Slack/Telegram channel]
`--channel` is one of `telegram` or `slack`; `--target` is the destination id *within* that platform (a Telegram
chat id, or a Slack channel/user id). It's easy to read "channel" as "Slack channel," which is a different field.
:::

```bash
morph automation add \
  --name "Telegram digest" \
  --schedule "every 2h" \
  --prompt "Summarize new activity since the last check." \
  --delivery gateway \
  --channel telegram \
  --target <chat-id>
```

Requires the Telegram gateway to already be enabled and configured; see [Telegram Gateway](./gateway/telegram).
`<chat-id>` is the same numeric chat id used elsewhere in your Telegram setup (for example, from
`morph gateway pairing list telegram`). Add `--thread <thread-id>` to target a specific forum topic.

### Gateway: Slack

```bash
morph automation add \
  --name "Slack digest" \
  --schedule "every 2h" \
  --prompt "Summarize new activity since the last check." \
  --delivery gateway \
  --channel slack \
  --target <channel-id>
```

Requires the Slack gateway to already be enabled; see [Slack Gateway](./gateway/slack). `<channel-id>` is a Slack
channel id (`C…`) or user id (`U…`) for a DM. Add `--thread <thread-ts>` to post into a specific thread.

### Webhook

```bash
morph automation add \
  --name "Webhook notify" \
  --schedule "every 1h" \
  --prompt "Check for anything that needs attention." \
  --delivery webhook \
  --webhook-url <webhook-url>
```

Morph POSTs a JSON payload (job id, run id, status, output, error, session id) to `<webhook-url>` and requires a 2xx
response; anything else is recorded as `not_delivered` with the response status and body. See
[Automation Reference](../reference/automation) for the exact payload shape.

## Where To Go Next

- [Automation](../concepts/automation): the job/run/delivery model behind everything above.
- [Automation Reference](../reference/automation): every command, flag, and enum value.
- [Automation Operations](../operations/automation): diagnostics, recovery, retries, and backoff.
- [Telegram Gateway](./gateway/telegram) and [Slack Gateway](./gateway/slack): credentials and setup for gateway
  delivery.
- [TUI Guide](./tui): the chat surface used for conversational automation management.

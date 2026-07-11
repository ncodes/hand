---
title: Automation Reference
description: Every automation command, flag, enum, and status.
---

# Automation Reference

Exhaustive reference for `morph automation`: every subcommand, flag, enum value, and status string. For the mental
model, see [Automation](../concepts/automation); for a task-oriented walkthrough, see
[Automation Guide](../guides/automation); for diagnostics and recovery workflows, see
[Automation Operations](../operations/automation).

## Command Tree

| Command | Arguments | Purpose |
| --- | --- | --- |
| `automation status` | | Scheduler snapshot: running, job count, running count, next wake |
| `automation list` | `--all`, `--limit` | List jobs |
| `automation add` | [mutation flags](#addupdate-flags) | Create a job |
| `automation update` | `<job-id>` [mutation flags](#addupdate-flags) | Patch a job; omitted flags are unchanged |
| `automation pause` | `<job-id>` | Set `enabled=false` |
| `automation resume` | `<job-id>` | Set `enabled=true` |
| `automation run` | `<job-id>` | Trigger a run now, independent of schedule |
| `automation remove` | `<job-id>` | Delete the job definition (run history is untouched) |
| `automation runs` | `--job`, `--status`, `--limit` | List run history |
| `automation diagnose` | `--all` | Report invalid schedules, stuck jobs, and delivery misconfiguration |
| `automation inspect` | `<job-id>`, `--failures` | Job detail plus its last run and recent failures |
| `automation recover` | subcommands | Repair scheduler state: recompute schedules, fix a job wrongly stuck showing as running, or rerun a failed job |

`pause` and `resume` are equivalent to `update <job-id> --enabled=false` / `--enabled=true` at the API level; they
exist as shortcuts and take no flags of their own.

## Add/Update Flags

| Flag | Type | On `add` | On `update` | Field |
| --- | --- | --- | --- | --- |
| `--name` | string | optional | optional | `Job.Name` |
| `--description` | string | optional | optional | `Job.Description` |
| `--schedule` | string | **required** | optional | `Job.Schedule` (see [Schedule Syntax](#schedule-syntax)) |
| `--prompt` | string | one of prompt/system-event required | optional | `Payload.Prompt`, sets kind `prompt` |
| `--system-event` | string | one of prompt/system-event required | optional | `Payload.SystemEvent`, sets kind `system_event` |
| `--profile` | string | optional | optional | `Job.Profile` |
| `--session-target` | string | optional, defaults to `isolated` | optional | see [Session Targets](#session-targets) |
| `--model` | string | optional | optional | `Payload.Model` override |
| `--provider` | string | optional | optional | `Payload.Provider` override |
| `--base-url` | string | optional | optional | `Payload.BaseURL` override |
| `--tool-group` | string, repeatable | optional | optional | `Payload.ToolGroups` |
| `--max-runtime` | duration | optional | optional | `Payload.MaxRuntime` |
| `--no-timeout` | bool | optional | optional | `Payload.NoTimeout`; overrides `--max-runtime` |
| `--max-iterations` | int | optional | optional | `Payload.MaxIterations` |
| `--retry-attempts` | int | optional | optional | `Payload.RetryAttempts` |
| `--retry-backoff` | duration | optional | optional | `Payload.RetryBackoff` |
| `--retry-max-delay` | duration | optional | optional | `Payload.RetryMaxDelay` |
| `--delivery` | string | optional, defaults to `none` | optional | see [Delivery](#delivery) |
| `--channel` | string | required for `gateway` | optional | `Delivery.Channel` |
| `--target` | string | required for `gateway` | optional | `Delivery.Target` |
| `--thread` | string | optional | optional | `Delivery.ThreadID` |
| `--webhook-url` | string | required for `webhook` | optional | `Delivery.WebhookURL` |
| `--best-effort` | bool | optional | optional | `Delivery.BestEffort`: don't fail the run if delivery fails |
| `--delete-after-run` | bool | optional | optional | `Job.DeleteAfterRun`: delete after the next **`ok`** run |
| `--disabled` | bool | **add-only** | not applicable | `Job.Enabled = !disabled` |

### Validation rules

- Exactly one of `--prompt` / `--system-event` must resolve to a non-empty value; the other must be empty.
- `--delivery none` or `--delivery local`: no `--channel`, `--target`, `--thread`, or `--webhook-url` allowed.
- `--delivery gateway`: `--channel` and `--target` are both required; no `--webhook-url`.
- `--delivery origin`: from the CLI, `--channel` and `--target` are effectively required too, since there's no
  active session to auto-capture them from; no `--webhook-url`.
- `--delivery webhook`: `--webhook-url` must be an absolute `http://` or `https://` URL; no `--channel`, `--target`,
  or `--thread`.
- `--max-iterations`, `--max-runtime`, `--retry-attempts`, `--retry-backoff`, `--retry-max-delay` must be
  non-negative.
- On `update`, switching `--delivery` to a new mode clears the previous mode's now-irrelevant fields (for example,
  switching from `gateway` to `webhook` clears `channel`/`target`/`thread`).
- On `update`, nested `Payload` and `Delivery` are only touched at all if at least one payload- or delivery-related
  flag is set; every other flag on the job is left exactly as it was.

:::note[A field showing empty or zero isn't necessarily "off"]
`--max-runtime`, `--retry-attempts`, `--retry-backoff`, and `--retry-max-delay` have no CLI-visible default: if
omitted, the job stores no explicit value, and the **daemon's built-in default** applies at run time instead (see
[Automation Operations](../operations/automation) for the exact values). `morph automation inspect` showing `Max runtime: 0s`
does not mean "no timeout": only `--no-timeout` means that. None of these daemon-level defaults are configurable
today; they are fixed constants.
:::

## Schedule Syntax

| Kind | Input example | Notes |
| --- | --- | --- |
| `at` | `2026-07-11T18:30:00Z` | One-shot. RFC3339(Nano) recommended; a handful of naive local-time layouts are also accepted, resolved via the timezone fallback below. No relative syntax exists. |
| `every` | `every 30m` | Recurring. Next run anchors to the *previous run's end time* (or job-creation time before the first run), not a fixed clock slot. |
| `cron` | `0 9 * * *` | Five whitespace-separated fields. Optional `TZ=<zone>` or `CRON_TZ=<zone>` prefix. |

:::note[Cron input has no `cron` keyword]
Provide only the five fields (and optional `TZ=`/`CRON_TZ=` prefix) as the `--schedule` value. `morph automation
list` *displays* cron schedules with a `cron ` prefix for readability (`cron 0 9 * * *`); that prefix is
display-only and is rejected as input (it would count as a sixth field).
:::

A bare duration without the `every` prefix (for example `--schedule "30m"`) is parsed as a **recurring** `every 30m`
schedule, not a one-shot delay.

### Timezone resolution order (cron only)

1. An explicit `TZ=`/`CRON_TZ=` prefix on the schedule string.
2. The daemon's configured default timezone.
3. The server's local system timezone.

Whichever timezone resolves the cron expression, the computed `NEXT RUN` is always stored and displayed in UTC.
`at` and `every` schedules do not use a timezone at all.

## Payload

| Kind | Value | Behavior |
| --- | --- | --- |
| `prompt` (default) | `Payload.Prompt` | A normal agent turn using the prompt text |
| `system_event` | `Payload.SystemEvent` | Reserved for a future wake/reminder flow; **currently a no-op**: the run is immediately recorded as `skipped` with no agent turn |

### Runtime-control fields

| Field | CLI flag | Default when unset |
| --- | --- | --- |
| Model override | `--model` | Profile's configured main model |
| Provider override | `--provider` | Profile's configured provider |
| Base URL override | `--base-url` | Profile's configured base URL |
| Tool group restriction | `--tool-group` (repeatable) | No restriction |
| Max runtime | `--max-runtime` | 30 minutes (daemon default) |
| No timeout | `--no-timeout` | Off (a timeout always applies unless this is set) |
| Max iterations | `--max-iterations` | Profile's configured iteration budget |
| Retry attempts | `--retry-attempts` | 1 (daemon default; i.e. no retry) |
| Retry backoff | `--retry-backoff` | 30 seconds (daemon default) |
| Retry max delay | `--retry-max-delay` | 5 minutes (daemon default) |

## Session Targets

| Value | Resolves to |
| --- | --- |
| `isolated` (default) | A brand-new session created just for this run |
| `main` | The default/main session |
| `current` | Whatever session is currently active for the profile[^origin-alias] |
| `origin` | Same as `current`[^origin-alias] |
| `session:<id>` | A specific, named session, reused on every run |

[^origin-alias]: `current` and `origin` call the exact same resolution path today and behave identically. Don't
treat them as two different behaviors.

## Delivery

| Mode | Requires | Delivery status on success |
| --- | --- | --- |
| `none` (default) | Nothing | `not_requested` |
| `local` | Nothing | `delivered` (the persisted run record itself is the delivery) |
| `origin` | `--channel` + `--target` (from the CLI); auto-captured from job metadata when created conversationally with context | `delivered` / `not_delivered` |
| `gateway` | `--channel` = `telegram` or `slack`; `--target` | `delivered` / `not_delivered` |
| `webhook` | `--webhook-url` (absolute `http`/`https`) | `delivered` / `not_delivered` |

:::note[`--channel` names the platform, not a destination]
For `origin` and `gateway` delivery, `--channel` is the literal string `telegram` or `slack`. `--target` is the
destination within that platform: a Telegram chat id, or a Slack channel/user id.
:::

### Delivery fields

| Field | CLI flag | Notes |
| --- | --- | --- |
| Mode | `--delivery` | `none`, `local`, `origin`, `gateway`, `webhook` |
| Channel | `--channel` | Platform name for `origin`/`gateway` |
| Target | `--target` | Destination id for `origin`/`gateway` |
| Thread ID | `--thread` | Optional; Telegram forum topic or Slack thread timestamp |
| Webhook URL | `--webhook-url` | Required for `webhook` |
| Best effort | `--best-effort` | Run doesn't fail when delivery fails |
| Failure target | (tool/RPC only) | Alternate destination for failure notices |
| Failure after | (tool/RPC only) | Consecutive failures before a failure notice fires |
| Failure cooldown | (tool/RPC only) | Minimum time between repeated failure notices |

The failure-notice fields have no CLI mutation flags today; they're set through the RPC/tool `delivery` object.

Execution and delivery are evaluated separately: delivery is only attempted for a run with status `ok`. A run with
status `error` instead attempts a failure notice (only if the threshold above is due); a run with status `skipped`
never attempts delivery. See [Automation](../concepts/automation#delivery-is-a-separate-outcome-from-execution).

## Statuses

There is no separate "job status" enum. A job's own lifecycle is just `Enabled` (`true`/`false`, toggled by
`pause`/`resume`); its last outcome reuses the run status below.

| Run status | Meaning |
| --- | --- |
| `running` | In progress |
| `ok` | Finished successfully |
| `error` | Failed during execution |
| `skipped` | No agent turn happened (a `system_event` payload, or a missed run skipped during startup recovery) |

| Delivery status | Meaning |
| --- | --- |
| `delivered` | Reached its destination (including `local`) |
| `not_delivered` | Delivery was attempted and failed |
| `not_requested` | No delivery was needed (`none` mode, or a non-`ok` run status) |
| `unknown` | Reserved for compatibility; not produced by current execution paths |

## IDs and Filters

| ID kind | Prefix | Example shape |
| --- | --- | --- |
| Job | `auto_` | `auto_<nanoid>` |
| Run | `autorun_` | `autorun_<nanoid>` |

| Filter flag | Commands | Behavior |
| --- | --- | --- |
| `--all` | `list`, `diagnose` | Include disabled jobs |
| `--limit` | `list`, `runs` | Maximum rows returned |
| `--job` | `runs` | Filter to one job's runs |
| `--status` | `runs` | Comma-separated run statuses, e.g. `error,skipped` |

## Diagnostics and Recovery (CLI-only)

`diagnose`, `inspect`, and `recover` are CLI/operator commands with **no RPC method and no agent-tool action**: the
CLI computes them client-side from the same `list`/`update`/`runs` calls available everywhere else. Full runbooks:
[Automation Operations](../operations/automation).

| Command | Arguments | What it does |
| --- | --- | --- |
| `automation diagnose` | `--all` | Reports invalid schedules, jobs wrongly stuck showing as running, and delivery misconfiguration |
| `automation inspect` | `<job-id>`, `--failures` (default `5`) | Full job detail plus its last run and recent failures |
| `automation recover recompute-schedules` | | Forces every enabled job to re-derive `NEXT RUN` |
| `automation recover clear-running <job-id>` | | Fixes one job that's wrongly stuck showing as still running |
| `automation recover rerun-failed <job-id>` | | Triggers a normal run, gated on the job having at least one `error` run |

## Agent Tool Actions

The owner-only `automation` tool exposes a **subset** of the CLI surface for conversational use:

```text
status, list, add, update, pause, resume, run, remove, runs
```

`diagnose`, `inspect`, and `recover` are **not** tool actions: the agent cannot self-diagnose or self-recover a
stuck job through this tool.

Schema differences from the CLI worth knowing:

- Duration fields (`max_runtime`, `retry_backoff`, `retry_max_delay`) are **nanoseconds** in the tool's JSON schema,
  not Go duration strings like the CLI's `--max-runtime 30m`.
- `capture_context` (add-only): when true, stamps the active session as `origin_session_id` metadata and defaults
  `session_target` to `origin` if it was left unset.
- `include_disabled` applies to `status` and `list` only.
- A job created or updated with `delivery.mode = origin` still needs an explicit channel and target if not already
  captured; it is not automatically inferred just because the agent is running inside a conversation.

## RPC and Trace Events

The automation gRPC service exposes 7 methods (`Status`, `List`, `Add`, `Update`, `Remove`, `Run`, `Runs`); there are
no RPC equivalents for `diagnose`/`inspect`/`recover`. See [RPC Reference](./rpc#automationservice).

Scheduler, job, and delivery lifecycle events are recorded under the `automation.*` prefix. See
[Trace Events](./trace-events#automation).

## Where To Go Next

- [Automation](../concepts/automation): the job/run/delivery model
- [Automation Guide](../guides/automation): task-oriented walkthrough with worked examples
- [Automation Operations](../operations/automation): numeric defaults, diagnostics, and recovery runbooks
- [Automation System](../development/automation-system): contributor architecture
- [RPC Reference](./rpc): gRPC services, including `AutomationService`
- [Trace Events](./trace-events): `automation.*` event names

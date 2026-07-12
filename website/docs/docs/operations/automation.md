---
title: Automation Operations
description: Reliability, diagnostics, and recovery for the automation scheduler.
---

# Automation Operations

The operator guide for keeping scheduled jobs healthy: what the scheduler does around a daemon restart, the
built-in defaults that govern timeouts and retries, and how to diagnose and recover a job that isn't behaving. For
the model, see [Automation](../concepts/automation); for everyday commands, see
[Automation Guide](../guides/automation); for every flag and status string, see
[Automation Reference](../reference/automation).

## Scheduler Lifecycle

The scheduler is not a separate process: it starts and stops with the [daemon](./daemon), the same one that owns
the agent runtime and gateways.

1. On daemon startup, the scheduler loads every job and repairs anything left inconsistent by an unclean shutdown
   (see [Startup Recovery](#startup-recovery)).
2. It sleeps until the next job is due, waking early whenever a job is added, updated, removed, or run manually.
3. On daemon shutdown, it stops taking on new scheduled work along with the rest of the daemon.

## Numeric Defaults

These are the daemon's built-in defaults today. None of them are configurable through `config.yaml` or `morph
config set`; they are fixed constants until a config surface is added for them.

| Behavior | Default |
| --- | --- |
| Maximum time between scheduler wake-ups | 1 minute |
| A job is considered wrongly stuck "running" after | 10 minutes |
| Run timeout (unless `--max-runtime` or `--no-timeout` is set) | 30 minutes |
| Delivery attempt timeout | 30 seconds |
| Retry attempts (unless `--retry-attempts` is set) | 1 (no retry) |
| Retry backoff (unless `--retry-backoff` is set) | 30 seconds, doubling per attempt |
| Retry maximum delay (unless `--retry-max-delay` is set) | 5 minutes |
| One-shot catch-up grace window after downtime | 5 minutes |
| Stagger between catch-up runs after downtime | 5 seconds per job |
| Run-history retention | 30 days |
| Run-history cleanup batch size | 500 rows per pass |
| Consecutive schedule errors before a job disables itself | 3 |
| Global concurrent run limit | Unbounded |

## Startup Recovery

When the daemon starts, it walks every job once:

- **Wrongly-stuck jobs**: if a job has been marked running for longer than the stuck-job threshold above (most
  likely because the daemon stopped mid-run), the scheduler clears that state so the job can run again.
- **Missed recurring jobs** (`every`/`cron`): a run that was due while the daemon was down is **skipped**, not
  executed late. The job's next run is computed fresh from its schedule.
- **Missed one-shot jobs** (`at`): if the missed time is still within the catch-up grace window, the job runs once,
  staggered a few seconds apart from other catch-up jobs so they don't all fire at the same instant. Outside that
  window, the one-shot job is skipped and has no further next run: to still get that reminder, add a new job.

:::note[A startup skip doesn't create a run history entry]
`morph automation runs` only shows runs the scheduler actually executed. A job skipped during startup recovery
updates that job's own last-status field to `skipped`, but does not add a row to its run history. If you're
checking what happened during downtime, look at the job itself (`morph automation inspect`), not just `runs`.
:::

## Concurrency

- A single job never runs twice at once; a second trigger (scheduled or manual) waits for the first to finish
  rather than starting in parallel.
- The daemon can also cap how many jobs run at the same time across all jobs, though this is unbounded by default
  today (see [Numeric Defaults](#numeric-defaults)).
- `morph automation run` on a job that's already running, or when the daemon is at its concurrency limit, waits for
  a free slot instead of failing.

## Retries, Backoff, and Auto-Disable

Two different failures are handled two different ways; mixing them up is the most common point of confusion:

| Kind of failure | What happens |
| --- | --- |
| The agent turn itself fails (`error` status) | The next attempt is delayed with doubling backoff (capped at the retry maximum delay), and the job **keeps trying indefinitely**. It never disables itself from run failures alone. |
| The job's schedule becomes invalid (for example, a timezone that no longer resolves) | The job disables itself after a few consecutive evaluation failures (see [Numeric Defaults](#numeric-defaults)), rather than retrying forever. |

If a job stopped running entirely rather than just slowing down, suspect a broken schedule, not a string of failed
runs. `morph automation diagnose` reports both cases separately.

## Delivery Failure Notices

Delivery failures don't retry the same way execution failures do; instead, a job can optionally send a **failure
notice** once enough runs in a row have failed. This is configured per job (not covered by a CLI flag; see
[Automation Reference](../reference/automation#delivery-fields)) with a failure count threshold, a cooldown between
repeated notices, and an optional separate destination. Without a threshold configured, failed runs are recorded but
never trigger a notice.

## Run History and Retention

Run history ages out automatically (see [Numeric Defaults](#numeric-defaults) for the retention window and cleanup
batch size); you don't need to prune it yourself. Removing a job (`morph automation remove`) does not immediately
delete its run history either; that also ages out on the normal schedule.

## Persistence and Backups

Jobs and run history live in the profile's SQLite store alongside everything else Morph persists. Back them up and
restore them the same way as the rest of profile state; see [Backups and State](./backups-and-state).

## Single-Daemon Ownership

The scheduler assumes exactly one daemon owns a profile's automation state at a time. There is no distributed
locking or exactly-once guarantee across multiple daemons pointed at the same profile; running two daemons against
one profile can produce duplicate or conflicting runs. Run one daemon per profile.

## Diagnose and Recover

Use these commands as a workflow, not manual data edits: run `diagnose` or `inspect` to see what's wrong, then the
matching `recover` action, then re-check.

| Symptom | Diagnose | Fix |
| --- | --- | --- |
| Scheduler seems unavailable | `morph doctor` (**automation** group); `morph automation status` | Start the daemon: `morph daemon` |
| Job never becomes due | `morph automation inspect <job-id>` for its next run and enabled state | If disabled, `morph automation resume <job-id>`; if the schedule looks wrong, update it |
| Schedule or timezone is invalid | `morph automation diagnose` | `morph automation update <job-id> --schedule <schedule>`, then `morph automation recover recompute-schedules` |
| Job wrongly stuck showing as running | `morph automation diagnose` or `inspect` | `morph automation recover clear-running <job-id>` |
| A run failed or hung | `morph automation inspect <job-id>` for the error and duration | `morph automation recover rerun-failed <job-id>` once the underlying cause is fixed |
| Job stopped running entirely (not just slower) | `morph automation diagnose` for an invalid-schedule finding | Fix the schedule, then `recover recompute-schedules` and `resume` if it disabled itself |
| Telegram or Slack delivery misconfigured | `morph automation diagnose` (delivery targets) | Fix gateway setup ([Telegram](../guides/gateway/telegram), [Slack](../guides/gateway/slack)) or the job's `--channel`/`--target` |
| Webhook delivery times out or returns a non-2xx status | `morph automation inspect <job-id>` for the delivery error | Fix the endpoint, or adjust `--webhook-url` |
| Run succeeded but delivery failed | `morph automation runs --job <job-id>`: `DELIVERY` column shows `not_delivered` | Same as above, by delivery mode; the run itself does not need retrying |
| One-shot job missed during downtime | `morph automation inspect <job-id>`: status `skipped`, no next run | Add a new one-shot job; the original will not retry itself |

## Where To Go Next

- [Automation](../concepts/automation): the job/run/delivery model.
- [Automation Guide](../guides/automation): everyday commands and delivery examples.
- [Automation Reference](../reference/automation): every flag, enum, and status string.
- [Automation System](../development/automation-system): contributor architecture.
- [Daemon Operations](./daemon): the process the scheduler runs inside.
- [Doctor](./doctor): the **automation** readiness group in the full check list.
- [Backups and State](./backups-and-state): back up and restore profile state, including jobs and runs.
- [Troubleshooting](../guides/troubleshooting): broader symptom-driven fixes.

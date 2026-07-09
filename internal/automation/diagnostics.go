package automation

import (
	"fmt"
	"time"
)

type DiagnosticSeverity string

const (
	DiagnosticSeverityWarn  DiagnosticSeverity = "warn"
	DiagnosticSeverityError DiagnosticSeverity = "error"
)

type DiagnosticFinding struct {
	Severity DiagnosticSeverity
	Code     string
	JobID    string
	Message  string
	Action   string
}

type DiagnosticOptions struct {
	Now               time.Time
	StaleRunningAfter time.Duration
	Location          *time.Location
	DefaultTimezone   string
}

type RunInspection struct {
	Job            Job
	LastRun        Run
	RecentFailures []Run
	DeliveryStatus DeliveryStatus
	DeliveryError  string
	SessionID      string
}

func DiagnoseJobs(jobs []Job, opts DiagnosticOptions) []DiagnosticFinding {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	staleRunningAfter := opts.StaleRunningAfter
	if staleRunningAfter <= 0 {
		staleRunningAfter = defaultStaleRunningAfter
	}

	findings := make([]DiagnosticFinding, 0)
	for _, job := range jobs {
		if job.Enabled {
			findings = append(findings, diagnoseJobSchedule(job, now, opts)...)
		}
		if !job.State.RunningAt.IsZero() && !job.State.RunningAt.Add(staleRunningAfter).After(now) {
			findings = append(findings, DiagnosticFinding{
				Severity: DiagnosticSeverityWarn,
				Code:     "stuck_running",
				JobID:    job.ID,
				Message: fmt.Sprintf(
					"job has been marked running since %s",
					job.State.RunningAt.UTC().Format(time.RFC3339),
				),
				Action: "morph automation recover clear-running " + job.ID,
			})
		}
		findings = append(findings, DiagnoseDelivery(job)...)
	}

	return findings
}

func DiagnoseDelivery(job Job) []DiagnosticFinding {
	delivery := normalizeDelivery(job.Delivery)
	switch delivery.Mode {
	case "", DeliveryNone, DeliveryLocal:
		return nil
	case DeliveryWebhook:
		if delivery.WebhookURL == "" {
			return []DiagnosticFinding{{
				Severity: DiagnosticSeverityError,
				Code:     "delivery_webhook_url_missing",
				JobID:    job.ID,
				Message:  "webhook delivery requires a webhook URL",
				Action:   "morph automation update " + job.ID + " --webhook-url <url>",
			}}
		}
	case DeliveryGateway:
		if delivery.Channel == "" || delivery.Target == "" {
			return []DiagnosticFinding{{
				Severity: DiagnosticSeverityWarn,
				Code:     "delivery_target_incomplete",
				JobID:    job.ID,
				Message:  "gateway delivery usually needs both channel and target",
				Action:   "morph automation update " + job.ID + " --channel <channel> --target <target>",
			}}
		}
	case DeliveryOrigin:
		target := getDeliveryTarget(job, delivery)
		if target.Channel == "" || target.Target == "" {
			return []DiagnosticFinding{{
				Severity: DiagnosticSeverityWarn,
				Code:     "delivery_origin_missing",
				JobID:    job.ID,
				Message:  "origin delivery has no captured channel or target",
				Action:   "morph automation update " + job.ID + " --channel <channel> --target <target>",
			}}
		}
	default:
		return []DiagnosticFinding{{
			Severity: DiagnosticSeverityError,
			Code:     "delivery_mode_unsupported",
			JobID:    job.ID,
			Message:  fmt.Sprintf("unsupported delivery mode %q", delivery.Mode),
			Action:   "morph automation update " + job.ID + " --delivery none",
		}}
	}

	return nil
}

func InspectRunHistory(job Job, runs []Run, failureLimit int) RunInspection {
	if failureLimit <= 0 {
		failureLimit = 5
	}

	inspection := RunInspection{Job: job}
	for _, run := range runs {
		if inspection.LastRun.ID == "" {
			inspection.LastRun = run
			inspection.DeliveryStatus = run.DeliveryStatus
			inspection.DeliveryError = run.DeliveryError
			inspection.SessionID = run.SessionID
		}
		if run.Status == RunStatusError && len(inspection.RecentFailures) < failureLimit {
			inspection.RecentFailures = append(inspection.RecentFailures, run)
		}
	}

	return inspection
}

func diagnoseJobSchedule(job Job, now time.Time, opts DiagnosticOptions) []DiagnosticFinding {
	_, err := EvaluateJob(job, NextRunOptions{
		Now:             now,
		LastRunAt:       job.State.LastRunAt,
		Location:        opts.Location,
		DefaultTimezone: opts.DefaultTimezone,
	})
	if err == nil {
		return nil
	}

	return []DiagnosticFinding{{
		Severity: DiagnosticSeverityError,
		Code:     "invalid_schedule",
		JobID:    job.ID,
		Message:  err.Error(),
		Action:   "morph automation update " + job.ID + " --schedule <schedule>",
	}}
}

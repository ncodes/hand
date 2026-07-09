package automation

import (
	"context"
	"errors"

	envtypes "github.com/wandxy/morph/internal/environment/types"
	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
)

type input struct {
	Action          string                `json:"action"`
	ID              string                `json:"id,omitempty"`
	Job             storage.AutomationJob `json:"job,omitempty"`
	Query           jobQueryInput         `json:"query,omitempty"`
	RunQuery        runQueryInput         `json:"run_query,omitempty"`
	CaptureContext  bool                  `json:"capture_context,omitempty"`
	IncludeDisabled bool                  `json:"include_disabled,omitempty"`
}

type jobQueryInput struct {
	IDs             []string `json:"ids,omitempty"`
	Enabled         *bool    `json:"enabled,omitempty"`
	Profile         string   `json:"profile,omitempty"`
	SessionTarget   string   `json:"session_target,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	IncludeDisabled bool     `json:"include_disabled,omitempty"`
}

type runQueryInput struct {
	JobID  string   `json:"job_id,omitempty"`
	IDs    []string `json:"ids,omitempty"`
	Status []string `json:"status,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

type output struct {
	Status string                  `json:"status"`
	Job    storage.AutomationJob   `json:"job"`
	Jobs   []storage.AutomationJob `json:"jobs,omitempty"`
	Run    storage.AutomationRun   `json:"run"`
	Runs   []storage.AutomationRun `json:"runs,omitempty"`
	Counts map[string]int          `json:"counts,omitempty"`
}

func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "automation",
		Description: "Owner-only automation control: status, list, add, update, pause, resume, run, remove, and runs.",
		Groups:      []string{"core"},
		InputSchema: common.ObjectSchema(map[string]any{
			"action": common.StringSchema(
				"Required. Action: status, list, add, update, pause, resume, run, remove, or runs. " +
					"Use status/list/runs for reads; use add/update/pause/resume/run/remove for mutations.",
			),
			"id": common.StringSchema(
				"Automation job id. Required for update, pause, resume, run, and remove. " +
					"Not used for status, list, add, or runs.",
			),
			"job": jobSchema(),
			"query": describedObjectSchema("Only used when action=list. Filters automation jobs.", map[string]any{
				"ids":              stringArraySchema("Automation job ids."),
				"enabled":          common.BooleanSchema("Filter by enabled state."),
				"profile":          common.StringSchema("Filter by profile."),
				"session_target":   common.StringSchema("Filter by session target."),
				"limit":            common.IntegerSchema("Maximum jobs to return."),
				"include_disabled": common.BooleanSchema("Include disabled jobs."),
			}),
			"run_query": describedObjectSchema("Only used when action=runs. Filters automation run history.", map[string]any{
				"job_id": common.StringSchema("Filter by automation job id. Usually set this for action=runs."),
				"ids":    stringArraySchema("Automation run ids."),
				"status": stringArraySchema("Run statuses."),
				"limit":  common.IntegerSchema("Maximum runs to return."),
			}),
			"capture_context": common.BooleanSchema(
				"Only used for action=add. When true, capture the active session as job origin metadata.",
			),
			"include_disabled": common.BooleanSchema(
				"Only used by action=status and action=list. Include disabled jobs in counts/listing.",
			),
		}, "action"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			service, err := getService(ctx, runtime)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			result, err := invoke(ctx, service, req)
			if err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			return common.EncodeOutput(result)
		}),
	}
}

func getService(ctx context.Context, runtime envtypes.Runtime) (envtypes.AutomationService, error) {
	if runtime == nil {
		return nil, errors.New("automation runtime is required")
	}
	service, ok, err := runtime.AutomationService(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || service == nil {
		return nil, errors.New("automation service is not supported")
	}

	return service, nil
}

func invoke(ctx context.Context, service envtypes.AutomationService, req input) (output, error) {
	if service == nil {
		return output{}, errors.New("automation service is required")
	}

	switch req.Action {
	case "status":
		return automationStatus(ctx, service, req)
	case "list":
		list, err := service.List(ctx, jobQueryFromInput(req.Query, req.IncludeDisabled))
		return output{Status: "ok", Jobs: list.Jobs}, err
	case "add":
		job := req.Job.Clone()
		if req.CaptureContext {
			job = captureContext(ctx, job)
		}
		if err := checkToolJob(job); err != nil {
			return output{}, err
		}
		created, err := service.Add(ctx, job)
		return output{Status: "ok", Job: created}, err
	case "update":
		patch, err := patchFromInput(req)
		if err != nil {
			return output{}, err
		}
		updated, err := service.Update(ctx, patch)
		return output{Status: "ok", Job: updated}, err
	case "pause":
		enabled := false
		updated, err := service.Update(ctx, storage.AutomationJobPatch{ID: req.ID, Enabled: &enabled})
		return output{Status: "ok", Job: updated}, err
	case "resume":
		enabled := true
		updated, err := service.Update(ctx, storage.AutomationJobPatch{ID: req.ID, Enabled: &enabled})
		return output{Status: "ok", Job: updated}, err
	case "run":
		run, err := service.Run(ctx, req.ID)
		return output{Status: "ok", Run: run}, err
	case "remove":
		return output{Status: "ok"}, service.Remove(ctx, req.ID)
	case "runs":
		list, err := service.Runs(ctx, runQueryFromInput(req.RunQuery))
		return output{Status: "ok", Runs: list.Runs}, err
	default:
		return output{}, errors.New("unsupported automation action")
	}
}

func automationStatus(ctx context.Context, service envtypes.AutomationService, req input) (output, error) {
	list, err := service.List(ctx, storage.AutomationJobQuery{IncludeDisabled: true})
	if err != nil {
		return output{}, err
	}

	counts := map[string]int{"jobs": len(list.Jobs)}
	for _, job := range list.Jobs {
		if job.Enabled {
			counts["enabled"]++
		}
		if !job.State.RunningAt.IsZero() {
			counts["running"]++
		}
	}

	return output{Status: "ok", Counts: counts}, nil
}

func captureContext(ctx context.Context, job storage.AutomationJob) storage.AutomationJob {
	sessionID := tools.SessionIDFromContext(ctx)
	if sessionID == "" {
		return job
	}
	if job.Payload.Metadata == nil {
		job.Payload.Metadata = map[string]string{}
	}
	job.Payload.Metadata["origin_session_id"] = sessionID
	if job.SessionTarget == "" {
		job.SessionTarget = "origin"
	}

	return job
}

func patchFromInput(req input) (storage.AutomationJobPatch, error) {
	patch := storage.AutomationJobPatch{
		ID:            req.ID,
		Name:          stringPtr(req.Job.Name),
		Description:   stringPtr(req.Job.Description),
		Profile:       stringPtr(req.Job.Profile),
		SessionTarget: stringPtr(req.Job.SessionTarget),
	}
	if req.Job.Schedule.Kind != "" {
		if err := checkToolSchedule(req.Job.Schedule); err != nil {
			return storage.AutomationJobPatch{}, err
		}
		patch.Schedule = &req.Job.Schedule
	}
	if req.Job.Payload.Kind != "" || req.Job.Payload.Prompt != "" || req.Job.Payload.SystemEvent != "" {
		if err := checkToolPayload(req.Job.Payload); err != nil {
			return storage.AutomationJobPatch{}, err
		}
		patch.Payload = &req.Job.Payload
	}
	if req.Job.Delivery.Mode != "" {
		patch.Delivery = &req.Job.Delivery
	}

	return patch, nil
}

func jobQueryFromInput(input jobQueryInput, includeDisabled bool) storage.AutomationJobQuery {
	return storage.AutomationJobQuery{
		IDs:             append([]string(nil), input.IDs...),
		Enabled:         input.Enabled,
		Profile:         input.Profile,
		SessionTarget:   input.SessionTarget,
		Limit:           input.Limit,
		IncludeDisabled: input.IncludeDisabled || includeDisabled,
	}
}

func runQueryFromInput(input runQueryInput) storage.AutomationRunQuery {
	statuses := make([]storage.AutomationRunStatus, 0, len(input.Status))
	for _, value := range input.Status {
		statuses = append(statuses, storage.AutomationRunStatus(value))
	}

	return storage.AutomationRunQuery{
		JobID:  input.JobID,
		IDs:    append([]string(nil), input.IDs...),
		Status: statuses,
		Limit:  input.Limit,
	}
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func checkToolJob(job storage.AutomationJob) error {
	if err := checkToolSchedule(job.Schedule); err != nil {
		return err
	}

	return checkToolPayload(job.Payload)
}

func checkToolSchedule(schedule storage.AutomationSchedule) error {
	switch schedule.Kind {
	case storage.AutomationScheduleAt:
		if schedule.At.IsZero() {
			return errors.New("automation one-shot schedule time is required")
		}
	case storage.AutomationScheduleEvery:
		if schedule.Every <= 0 {
			return errors.New("automation interval schedule must be greater than zero")
		}
	case storage.AutomationScheduleCron:
		if schedule.Cron == "" {
			return errors.New("automation cron schedule expression is required")
		}
	default:
		return errors.New("automation schedule kind is required")
	}

	return nil
}

func checkToolPayload(payload storage.AutomationPayload) error {
	switch payload.Kind {
	case "", storage.AutomationPayloadPrompt:
		if payload.Prompt == "" {
			return errors.New("automation prompt payload is required")
		}
	case storage.AutomationPayloadSystemEvent:
		if payload.SystemEvent == "" {
			return errors.New("automation system event payload is required")
		}
	default:
		return errors.New("unsupported automation payload kind")
	}

	return nil
}

func jobSchema() map[string]any {
	return common.ObjectSchema(map[string]any{
		"id":               common.StringSchema("Optional for action=add. Ignored for action=update; use top-level id there."),
		"name":             common.StringSchema("Human-readable job name."),
		"description":      common.StringSchema("Human-readable job description."),
		"enabled":          common.BooleanSchema("Whether the job is enabled."),
		"profile":          common.StringSchema("Profile to run with."),
		"session_target":   common.StringSchema("Session target: isolated, main, current, origin, or session:<id>."),
		"delete_after_run": common.BooleanSchema("Delete after a successful run."),
		"schedule": common.ObjectSchema(map[string]any{
			"kind": common.StringSchema(
				"Schedule kind. Required when setting schedule. Use at, every, or cron. " +
					"If kind=at, at is required. If kind=every, every is required. If kind=cron, cron is required.",
			),
			"at": common.StringSchema(
				"RFC3339 timestamp. Required when schedule.kind=at; ignored for every/cron schedules.",
			),
			"every": common.IntegerSchema(
				"Interval duration in nanoseconds. Required and must be greater than zero when schedule.kind=every.",
			),
			"cron": common.StringSchema(
				"Cron expression. Required when schedule.kind=cron.",
			),
			"timezone": common.StringSchema("Optional IANA timezone name for cron schedules."),
		}),
		"payload": common.ObjectSchema(map[string]any{
			"kind": common.StringSchema(
				"Payload kind. Use prompt or system_event. Defaults to prompt when omitted. " +
					"If kind=prompt or omitted, prompt is required. If kind=system_event, system_event is required.",
			),
			"prompt": common.StringSchema(
				"Prompt for agent-turn jobs. Required when payload.kind=prompt or payload.kind is omitted.",
			),
			"system_event": common.StringSchema(
				"System event value. Required when payload.kind=system_event.",
			),
			"model":           common.StringSchema("Model override."),
			"provider":        common.StringSchema("Provider override."),
			"base_url":        common.StringSchema("Provider base URL override. Use with provider/model overrides when needed."),
			"no_timeout":      common.BooleanSchema("Disable run timeout. When true, max_runtime is ignored."),
			"max_runtime":     common.IntegerSchema("Max runtime in nanoseconds. Ignored when no_timeout=true."),
			"max_iterations":  common.IntegerSchema("Max agent iterations."),
			"retry_attempts":  common.IntegerSchema("Retry attempts."),
			"retry_backoff":   common.IntegerSchema("Initial retry backoff in nanoseconds."),
			"retry_max_delay": common.IntegerSchema("Maximum retry delay in nanoseconds."),
			"tool_groups":     stringArraySchema("Allowed tool groups."),
			"metadata": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}),
		"delivery": common.ObjectSchema(map[string]any{
			"mode": common.StringSchema(
				"Delivery mode: none, local, origin, gateway, or webhook. " +
					"If mode=webhook, webhook_url is required. If mode=gateway or origin, channel/target may be required by the configured delivery sink.",
			),
			"channel":     common.StringSchema("Delivery channel. Usually required for gateway/origin delivery."),
			"target":      common.StringSchema("Delivery target. Usually required for gateway/origin delivery."),
			"thread_id":   common.StringSchema("Delivery thread id."),
			"webhook_url": common.StringSchema("Webhook URL. Required when delivery.mode=webhook."),
			"best_effort": common.BooleanSchema("Do not fail the run if delivery fails."),
			"failure_target": common.StringSchema(
				"Failure-notice target. Used only when failure_after is greater than zero.",
			),
			"failure_after": common.IntegerSchema(
				"Consecutive failures before notifying. Set greater than zero to enable failure notices.",
			),
			"failure_cooldown": common.IntegerSchema(
				"Failure notification cooldown in nanoseconds. Used only when failure_after is greater than zero.",
			),
		}),
	})
}

func describedObjectSchema(description string, properties map[string]any, required ...string) map[string]any {
	schema := common.ObjectSchema(properties, required...)
	schema["description"] = description
	return schema
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

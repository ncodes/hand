package core

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/wandxy/morph/pkg/nanoid"
	"github.com/wandxy/morph/pkg/str"
)

const (
	AutomationJobIDPrefix = "auto_"
	AutomationRunIDPrefix = "autorun_"
)

type AutomationScheduleKind string

const (
	AutomationScheduleAt    AutomationScheduleKind = "at"
	AutomationScheduleEvery AutomationScheduleKind = "every"
	AutomationScheduleCron  AutomationScheduleKind = "cron"
)

type AutomationPayloadKind string

const (
	AutomationPayloadPrompt      AutomationPayloadKind = "prompt"
	AutomationPayloadSystemEvent AutomationPayloadKind = "system_event"
)

type AutomationDeliveryMode string

const (
	AutomationDeliveryNone    AutomationDeliveryMode = "none"
	AutomationDeliveryLocal   AutomationDeliveryMode = "local"
	AutomationDeliveryOrigin  AutomationDeliveryMode = "origin"
	AutomationDeliveryGateway AutomationDeliveryMode = "gateway"
	AutomationDeliveryWebhook AutomationDeliveryMode = "webhook"
)

type AutomationRunStatus string

const (
	AutomationRunStatusRunning AutomationRunStatus = "running"
	AutomationRunStatusOK      AutomationRunStatus = "ok"
	AutomationRunStatusError   AutomationRunStatus = "error"
	AutomationRunStatusSkipped AutomationRunStatus = "skipped"
)

type AutomationDeliveryStatus string

const (
	AutomationDeliveryStatusDelivered    AutomationDeliveryStatus = "delivered"
	AutomationDeliveryStatusNotDelivered AutomationDeliveryStatus = "not_delivered"
	AutomationDeliveryStatusNotRequested AutomationDeliveryStatus = "not_requested"
	AutomationDeliveryStatusUnknown      AutomationDeliveryStatus = "unknown"
)

type AutomationSchedule struct {
	Kind     AutomationScheduleKind `json:"kind,omitempty"`
	At       time.Time              `json:"at,omitempty"`
	Every    time.Duration          `json:"every,omitempty"`
	Cron     string                 `json:"cron,omitempty"`
	Timezone string                 `json:"timezone,omitempty"`
}

type AutomationPayload struct {
	Kind          AutomationPayloadKind `json:"kind,omitempty"`
	Prompt        string                `json:"prompt,omitempty"`
	SystemEvent   string                `json:"systemEvent,omitempty"`
	Model         string                `json:"model,omitempty"`
	Provider      string                `json:"provider,omitempty"`
	BaseURL       string                `json:"baseUrl,omitempty"`
	MaxRuntime    time.Duration         `json:"maxRuntime,omitempty"`
	MaxIterations int                   `json:"maxIterations,omitempty"`
	ToolGroups    []string              `json:"toolGroups,omitempty"`
	Metadata      map[string]string     `json:"metadata,omitempty"`
}

type AutomationDelivery struct {
	Mode          AutomationDeliveryMode `json:"mode,omitempty"`
	Channel       string                 `json:"channel,omitempty"`
	Target        string                 `json:"target,omitempty"`
	ThreadID      string                 `json:"threadId,omitempty"`
	WebhookURL    string                 `json:"webhookUrl,omitempty"`
	BestEffort    bool                   `json:"bestEffort,omitempty"`
	FailureTarget string                 `json:"failureTarget,omitempty"`
}

type AutomationJobState struct {
	NextRunAt         time.Time           `json:"nextRunAt,omitempty"`
	RunningAt         time.Time           `json:"runningAt,omitempty"`
	LastRunAt         time.Time           `json:"lastRunAt,omitempty"`
	LastStatus        AutomationRunStatus `json:"lastStatus,omitempty"`
	LastError         string              `json:"lastError,omitempty"`
	LastDuration      time.Duration       `json:"lastDuration,omitempty"`
	ConsecutiveErrors int                 `json:"consecutiveErrors,omitempty"`
}

type AutomationUsage struct {
	InputTokens      int `json:"inputTokens,omitempty"`
	OutputTokens     int `json:"outputTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
}

type AutomationJob struct {
	ID             string
	Name           string
	Description    string
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Schedule       AutomationSchedule
	Payload        AutomationPayload
	Delivery       AutomationDelivery
	Profile        string
	SessionTarget  string
	DeleteAfterRun bool
	State          AutomationJobState
}

type AutomationJobPatch struct {
	ID             string
	Name           *string
	Description    *string
	Enabled        *bool
	Schedule       *AutomationSchedule
	Payload        *AutomationPayload
	Delivery       *AutomationDelivery
	Profile        *string
	SessionTarget  *string
	DeleteAfterRun *bool
	State          *AutomationJobState
}

type AutomationJobQuery struct {
	IDs             []string
	Enabled         *bool
	Profile         string
	SessionTarget   string
	Limit           int
	IncludeDisabled bool
}

type AutomationJobResult struct {
	Jobs []AutomationJob
}

type AutomationRun struct {
	ID             string
	JobID          string
	Status         AutomationRunStatus
	StartedAt      time.Time
	EndedAt        time.Time
	Duration       time.Duration
	Output         string
	Error          string
	SessionID      string
	DeliveryStatus AutomationDeliveryStatus
	DeliveryError  string
	Model          string
	Provider       string
	Usage          AutomationUsage
}

type AutomationRunPatch struct {
	ID             string
	Status         AutomationRunStatus
	EndedAt        time.Time
	Output         string
	Error          string
	SessionID      string
	DeliveryStatus AutomationDeliveryStatus
	DeliveryError  string
	Model          string
	Provider       string
	Usage          *AutomationUsage
}

type AutomationRunQuery struct {
	JobID  string
	IDs    []string
	Status []AutomationRunStatus
	Limit  int
}

type AutomationRunResult struct {
	Runs []AutomationRun
}

type AutomationStore interface {
	CreateJob(context.Context, AutomationJob) (AutomationJob, error)
	GetJob(context.Context, string) (AutomationJob, bool, error)
	ListJobs(context.Context, AutomationJobQuery) (AutomationJobResult, error)
	PatchJob(context.Context, AutomationJobPatch) (AutomationJob, error)
	DeleteJob(context.Context, string) error
	CreateRun(context.Context, AutomationRun) (AutomationRun, error)
	FinishRun(context.Context, AutomationRunPatch) (AutomationRun, error)
	ListRuns(context.Context, AutomationRunQuery) (AutomationRunResult, error)
}

func ValidateAutomationJobID(id string) error {
	jobID := str.String(id)
	trimmedID := jobID.Trim()
	if trimmedID == "" {
		return errors.New("automation job id is required")
	}
	if !strings.HasPrefix(trimmedID, AutomationJobIDPrefix) || nanoid.ValidateID(trimmedID) != nil {
		return errors.New("automation job id must be a valid auto_ nanoid")
	}
	return nil
}

func ValidateAutomationRunID(id string) error {
	runID := str.String(id)
	trimmedID := runID.Trim()
	if trimmedID == "" {
		return errors.New("automation run id is required")
	}
	if !strings.HasPrefix(trimmedID, AutomationRunIDPrefix) || nanoid.ValidateID(trimmedID) != nil {
		return errors.New("automation run id must be a valid autorun_ nanoid")
	}
	return nil
}

func (job AutomationJob) Clone() AutomationJob {
	job.Payload = job.Payload.Clone()
	return job
}

func (payload AutomationPayload) Clone() AutomationPayload {
	if len(payload.ToolGroups) > 0 {
		payload.ToolGroups = append([]string(nil), payload.ToolGroups...)
	}
	if len(payload.Metadata) > 0 {
		metadata := make(map[string]string, len(payload.Metadata))
		maps.Copy(metadata, payload.Metadata)
		payload.Metadata = metadata
	}
	return payload
}

func (run AutomationRun) Clone() AutomationRun {
	return run
}

func ApplyAutomationJobPatch(job AutomationJob, patch AutomationJobPatch, updatedAt time.Time) AutomationJob {
	if patch.Name != nil {
		job.Name = *patch.Name
	}
	if patch.Description != nil {
		job.Description = *patch.Description
	}
	if patch.Enabled != nil {
		job.Enabled = *patch.Enabled
	}
	if patch.Schedule != nil {
		job.Schedule = *patch.Schedule
	}
	if patch.Payload != nil {
		job.Payload = patch.Payload.Clone()
	}
	if patch.Delivery != nil {
		job.Delivery = *patch.Delivery
	}
	if patch.Profile != nil {
		job.Profile = *patch.Profile
	}
	if patch.SessionTarget != nil {
		job.SessionTarget = *patch.SessionTarget
	}
	if patch.DeleteAfterRun != nil {
		job.DeleteAfterRun = *patch.DeleteAfterRun
	}
	if patch.State != nil {
		job.State = *patch.State
	}
	job.UpdatedAt = updatedAt.UTC()
	return job.Clone()
}

func ApplyAutomationRunPatch(run AutomationRun, patch AutomationRunPatch, now time.Time) AutomationRun {
	if patch.Status != "" {
		run.Status = patch.Status
	}
	if patch.EndedAt.IsZero() {
		run.EndedAt = now.UTC()
	} else {
		run.EndedAt = patch.EndedAt.UTC()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = run.EndedAt
	}
	if run.Duration <= 0 && !run.EndedAt.IsZero() {
		run.Duration = run.EndedAt.Sub(run.StartedAt)
	}
	run.Output = patch.Output
	run.Error = patch.Error
	sessionID := str.String(patch.SessionID)
	run.SessionID = sessionID.Trim()
	if patch.DeliveryStatus != "" {
		run.DeliveryStatus = patch.DeliveryStatus
	}
	run.DeliveryError = patch.DeliveryError
	model := str.String(patch.Model)
	provider := str.String(patch.Provider)
	run.Model = model.Trim()
	run.Provider = provider.Trim()
	if patch.Usage != nil {
		run.Usage = *patch.Usage
	}
	return run.Clone()
}

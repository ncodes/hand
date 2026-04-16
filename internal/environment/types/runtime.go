package types

import (
	"context"

	"github.com/wandxy/hand/internal/environment/process"
	"github.com/wandxy/hand/internal/guardrails"
)

type Runtime interface {
	// Policy management
	FilePolicy() guardrails.FilesystemPolicy
	CommandPolicy() guardrails.CommandPolicy

	// Process management
	StartProcess(ctx context.Context, sessionID string, req process.StartRequest) (process.Info, error)
	GetProcess(sessionID string, processID string) (process.Info, error)
	ReadProcess(sessionID string, req process.ReadRequest) (process.Output, error)
	StopProcess(ctx context.Context, sessionID string, processID string) (process.Info, error)
	ListProcesses(sessionID string) []process.Info
	SearchSession(ctx context.Context, req SessionSearchRequest) ([]SessionSearchResult, error)

	// Plan management
	GetPlan(string) Plan
	ReplacePlan(string, Plan) (Plan, error)
	MergePlan(string, []PartialPlanStep, string, bool) (Plan, error)
	ClearPlan(string) Plan
	HydratePlan(string, Plan)
}

type SessionSearchRequest struct {
	SessionID   string `json:"session_id,omitempty"`
	Query       string `json:"query"`
	Role        string `json:"role,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
}

type SessionSearchResult struct {
	MessageID     uint      `json:"message_id"`
	Role          string    `json:"role"`
	ToolName      string    `json:"tool_name,omitempty"`
	CreatedAt     string    `json:"created_at"`
	Snippet       string    `json:"snippet"`
	FullTextBytes int       `json:"full_text_bytes"`
	MatchIndex    int       `json:"match_index"`
}

const (
	PlanStatusPending    = "pending"
	PlanStatusInProgress = "in_progress"
	PlanStatusCompleted  = "completed"
	PlanStatusCancelled  = "cancelled"
)

type PlanStep struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type PartialPlanStep struct {
	ID      string  `json:"id"`
	Content *string `json:"content,omitempty"`
	Status  *string `json:"status,omitempty"`
}

type Plan struct {
	Steps       []PlanStep `json:"steps"`
	Explanation string     `json:"explanation,omitempty"`
}

type PlanSummary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

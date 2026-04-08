package types

import "github.com/wandxy/hand/internal/guardrails"

type Runtime interface {
	FilePolicy() guardrails.FilesystemPolicy
	CommandPolicy() guardrails.CommandPolicy
	GetPlan(string) Plan
	ReplacePlan(string, Plan) (Plan, error)
	MergePlan(string, []PartialPlanStep, string, bool) (Plan, error)
	ClearPlan(string) Plan
	HydratePlan(string, Plan)
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

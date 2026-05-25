package planstore

import (
	"errors"
	"fmt"
	"strings"
)

const (
	PlanStatusPending    = "pending"
	PlanStatusInProgress = "in_progress"
	PlanStatusCompleted  = "completed"
	PlanStatusCancelled  = "cancelled"
)

// PlanStep describes one plan step.
type PlanStep struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

// PartialPlanStep describes one partial plan step.
type PartialPlanStep struct {
	ID      string  `json:"id"`
	Content *string `json:"content,omitempty"`
	Status  *string `json:"status,omitempty"`
}

// Plan describes the current plan state.
type Plan struct {
	Steps       []PlanStep `json:"steps"`
	Explanation string     `json:"explanation,omitempty"`
}

// PlanSummary summarizes plan state.
type PlanSummary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

// ValidatePlan checks that a plan has usable steps and statuses.
func ValidatePlan(plan Plan) error {
	active := 0
	for idx, step := range plan.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("step %d id is required", idx)
		}
		if strings.TrimSpace(step.Content) == "" {
			return fmt.Errorf("step %d content is required", idx)
		}
		if !ValidPlanStatus(step.Status) {
			return fmt.Errorf("step %d status is invalid", idx)
		}
		if step.Status == PlanStatusInProgress {
			active++
		}
	}
	if active > 1 {
		return errors.New("only one step may be in_progress")
	}
	if active == 0 {
		for _, step := range plan.Steps {
			if step.Status != PlanStatusCompleted && step.Status != PlanStatusCancelled {
				return errors.New("exactly one step must be in_progress while active work remains")
			}
		}
	}
	return nil
}

// ValidPlanStatus reports whether status is accepted by the plan store.
func ValidPlanStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case PlanStatusPending,
		PlanStatusInProgress,
		PlanStatusCompleted,
		PlanStatusCancelled:
		return true
	default:
		return false
	}
}

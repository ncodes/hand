package types

import (
	"errors"
	"fmt"
	"strings"
)

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

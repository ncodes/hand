package types

import envplanstore "github.com/wandxy/hand/internal/environment/planstore"

const (
	PlanStatusPending    = envplanstore.PlanStatusPending
	PlanStatusInProgress = envplanstore.PlanStatusInProgress
	PlanStatusCompleted  = envplanstore.PlanStatusCompleted
	PlanStatusCancelled  = envplanstore.PlanStatusCancelled
)

type PlanStep = envplanstore.PlanStep
type PartialPlanStep = envplanstore.PartialPlanStep
type Plan = envplanstore.Plan
type PlanSummary = envplanstore.PlanSummary

func ValidatePlan(plan Plan) error {
	return envplanstore.ValidatePlan(plan)
}

func ValidPlanStatus(status string) bool {
	return envplanstore.ValidPlanStatus(status)
}

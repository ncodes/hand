package plan

import (
	"context"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
	"github.com/wandxy/hand/internal/trace"
)

func Definition(runtime envtypes.Runtime) tools.Definition {
	type input struct {
		Steps          []map[string]any `json:"steps"`
		Merge          bool             `json:"merge"`
		Explanation    string           `json:"explanation"`
		ClearCompleted bool             `json:"clear_completed"`
	}

	return tools.Definition{
		Name: "plan_tool",
		Description: "Read or update the current session plan for multi-step work. Omit `steps` to read the current" +
			" plan without changing it. Provide `steps` to replace or merge plan items.",
		Groups: []string{"core"},
		InputSchema: common.ObjectSchema(map[string]any{
			"steps": map[string]any{
				"type": "array",
				"description": "If omitted, the tool returns the current plan without changing it. " +
					"If provided, the tool replaces or merges plan steps in the current session plan.",
				"items": common.ObjectSchema(map[string]any{
					"id":      common.StringSchema("Stable step identifier."),
					"content": common.StringSchema("Human-readable step description."),
					"status": map[string]any{
						"type":        "string",
						"description": "Plan step status.",
						"enum": []string{
							envtypes.PlanStatusPending,
							envtypes.PlanStatusInProgress,
							envtypes.PlanStatusCompleted,
							envtypes.PlanStatusCancelled,
						},
					},
				}),
			},
			"merge":           common.BooleanSchema("When true, merge step updates by id instead of replacing the full plan."),
			"explanation":     common.StringSchema("Optional explanation for why the plan changed."),
			"clear_completed": common.BooleanSchema("When true, remove completed and cancelled steps after applying the update."),
		}),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input

			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			sessionID := tools.SessionIDFromContext(ctx)
			if strings.TrimSpace(sessionID) == "" {
				sessionID = "default"
			}

			if req.Steps == nil {
				return encodePlanOutput(runtime.GetPlan(sessionID))
			}

			var (
				plan envtypes.Plan
				err  error
			)

			if req.Merge {
				updates, validationErr := decodePartialPlanSteps(req.Steps)
				if validationErr != nil {
					return common.ToolError("invalid_input", validationErr.Error()), nil
				}
				plan, err = runtime.MergePlan(sessionID, updates, req.Explanation, req.ClearCompleted)
			} else {
				steps, validationErr := decodePlanSteps(req.Steps)
				if validationErr != nil {
					return common.ToolError("invalid_input", validationErr.Error()), nil
				}

				plan = envtypes.Plan{
					Steps:       steps,
					Explanation: strings.TrimSpace(req.Explanation),
				}

				if req.ClearCompleted {
					filtered := make([]envtypes.PlanStep, 0, len(plan.Steps))
					for _, step := range plan.Steps {
						if step.Status == envtypes.PlanStatusCompleted || step.Status == envtypes.PlanStatusCancelled {
							continue
						}
						filtered = append(filtered, step)
					}
					plan.Steps = filtered
				}

				if err = envtypes.ValidatePlan(plan); err == nil {
					plan, err = runtime.ReplacePlan(sessionID, plan)
				}
			}

			if err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			recordPlanEvent(ctx, sessionID, plan)

			if req.ClearCompleted && len(plan.Steps) == 0 {
				recordPlanCleared(ctx, sessionID, plan)
			}

			return encodePlanOutput(plan)
		}),
	}
}

func decodePlanSteps(rawSteps []map[string]any) ([]envtypes.PlanStep, error) {
	seen := make(map[string]struct{}, len(rawSteps))
	steps := make([]envtypes.PlanStep, 0, len(rawSteps))

	for _, item := range rawSteps {
		id, _ := item["id"].(string)
		content, _ := item["content"].(string)
		status, _ := item["status"].(string)

		id = strings.TrimSpace(id)
		content = strings.TrimSpace(content)
		status = strings.TrimSpace(status)

		if id == "" {
			return nil, errInvalidPlan("step id is required")
		}
		if _, ok := seen[id]; ok {
			return nil, errInvalidPlan("step ids must be unique")
		}
		seen[id] = struct{}{}

		if content == "" {
			return nil, errInvalidPlan("step content is required")
		}
		if !envtypes.ValidPlanStatus(status) {
			return nil, errInvalidPlan("step status is invalid")
		}

		steps = append(steps, envtypes.PlanStep{
			ID:      id,
			Content: content,
			Status:  status,
		})
	}

	return steps, envtypes.ValidatePlan(envtypes.Plan{Steps: steps})
}

func decodePartialPlanSteps(rawSteps []map[string]any) ([]envtypes.PartialPlanStep, error) {
	seen := make(map[string]struct{}, len(rawSteps))
	steps := make([]envtypes.PartialPlanStep, 0, len(rawSteps))

	for _, item := range rawSteps {
		id, _ := item["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errInvalidPlan("step id is required")
		}
		if _, ok := seen[id]; ok {
			return nil, errInvalidPlan("step ids must be unique")
		}
		seen[id] = struct{}{}

		update := envtypes.PartialPlanStep{ID: id}

		if contentValue, ok := item["content"]; ok {
			content, contentOK := contentValue.(string)
			if !contentOK || strings.TrimSpace(content) == "" {
				return nil, errInvalidPlan("step content is required")
			}
			trimmed := strings.TrimSpace(content)
			update.Content = &trimmed
		}

		if statusValue, ok := item["status"]; ok {
			status, statusOK := statusValue.(string)
			status = strings.TrimSpace(status)
			if !statusOK || !envtypes.ValidPlanStatus(status) {
				return nil, errInvalidPlan("step status is invalid")
			}
			update.Status = &status
		}

		steps = append(steps, update)
	}

	return steps, nil
}

func encodePlanOutput(plan envtypes.Plan) (tools.Result, error) {
	summary := summarizePlan(plan)
	activeStepID := ""

	for _, step := range plan.Steps {
		if step.Status == envtypes.PlanStatusInProgress {
			activeStepID = step.ID
			break
		}
	}

	return common.EncodeOutput(map[string]any{
		"steps":          plan.Steps,
		"summary":        summary,
		"active_step_id": activeStepID,
		"explanation":    strings.TrimSpace(plan.Explanation),
	})
}

func summarizePlan(plan envtypes.Plan) envtypes.PlanSummary {
	summary := envtypes.PlanSummary{Total: len(plan.Steps)}

	for _, step := range plan.Steps {
		switch step.Status {
		case envtypes.PlanStatusPending:
			summary.Pending++
		case envtypes.PlanStatusInProgress:
			summary.InProgress++
		case envtypes.PlanStatusCompleted:
			summary.Completed++
		case envtypes.PlanStatusCancelled:
			summary.Cancelled++
		}
	}

	return summary
}

func recordPlanEvent(ctx context.Context, sessionID string, plan envtypes.Plan) {
	recorder := tools.TraceRecorderFromContext(ctx)
	if recorder == nil {
		return
	}

	recorder.Record(trace.EvtPlanUpdated, map[string]any{
		"session_id":     sessionID,
		"steps":          plan.Steps,
		"summary":        summarizePlan(plan),
		"active_step_id": activePlanStepID(plan),
		"explanation":    strings.TrimSpace(plan.Explanation),
	})
}

func recordPlanCleared(ctx context.Context, sessionID string, plan envtypes.Plan) {
	recorder := tools.TraceRecorderFromContext(ctx)
	if recorder == nil {
		return
	}

	recorder.Record(trace.EvtPlanCleared, map[string]any{
		"session_id":     sessionID,
		"steps":          plan.Steps,
		"summary":        summarizePlan(plan),
		"active_step_id": "",
		"explanation":    strings.TrimSpace(plan.Explanation),
	})
}

func activePlanStepID(plan envtypes.Plan) string {
	for _, step := range plan.Steps {
		if step.Status == envtypes.PlanStatusInProgress {
			return step.ID
		}
	}
	return ""
}

type invalidPlanError string

func (e invalidPlanError) Error() string {
	return string(e)
}

func errInvalidPlan(message string) error {
	return invalidPlanError(message)
}

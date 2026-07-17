package permissions

import (
	"context"
	"errors"
)

const (
	ErrorCodeDenied              = "permission_denied"
	ErrorCodeApprovalRequired    = "approval_required"
	ErrorCodeApprovalRateLimited = "approval_rate_limited"
)

type DecisionError struct {
	Code       string
	Evaluation Evaluation
}

func (e *DecisionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Evaluation.Reason != "" {
		return e.Evaluation.Reason
	}
	if e.Code == ErrorCodeApprovalRequired {
		return "approval required"
	}
	return "permission denied"
}

type Engine struct {
	policy Policy
}

type Checker interface {
	Check(context.Context, EvaluationInput) (Evaluation, error)
}

func NewEngine(policy Policy) Engine {
	policy.Normalize()
	return Engine{policy: policy}
}

func (e Engine) Preset(ctx context.Context) Preset {
	if preset, ok := PresetFromContext(ctx); ok {
		return preset
	}

	return e.policy.EffectivePreset()
}

func (e Engine) Evaluate(ctx context.Context, input EvaluationInput) Evaluation {
	if authorization, ok := FromContext(ctx); ok {
		input.Authorization = authorization
	} else {
		input.Authorization = AuthorizationContext{
			Actor:       Actor{Kind: ActorUnknown},
			SurfaceKind: SurfaceKindUnknown,
			Surface:     SurfaceUnknown,
		}
	}

	return e.policyFor(ctx).Evaluate(input)
}

func (e Engine) Check(ctx context.Context, input EvaluationInput) (Evaluation, error) {
	evaluation := e.Evaluate(ctx, input)
	if evaluation.Decision == DecisionAllow {
		return evaluation, nil
	}

	code := ErrorCodeDenied
	if evaluation.Decision == DecisionAsk {
		code = ErrorCodeApprovalRequired
	}

	return evaluation, &DecisionError{Code: code, Evaluation: evaluation}
}

func (e Engine) policyFor(ctx context.Context) Policy {
	policy := e.policy
	policy.Preset = e.Preset(ctx)
	return policy
}

func GetDecisionError(err error) (*DecisionError, bool) {
	var decisionErr *DecisionError
	ok := errors.As(err, &decisionErr)
	return decisionErr, ok
}

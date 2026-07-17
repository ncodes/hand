package permissions

import (
	"context"
)

type authorizationContextKey struct{}
type fullAccessContextKey struct{}
type presetContextKey struct{}
type authorizedOperationsContextKey struct{}

func WithContext(ctx context.Context, authorization AuthorizationContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	normalized, err := authorization.Normalize()
	if err != nil {
		return ctx
	}

	return context.WithValue(ctx, authorizationContextKey{}, normalized)
}

func FromContext(ctx context.Context) (AuthorizationContext, bool) {
	if ctx == nil {
		return AuthorizationContext{}, false
	}

	authorization, ok := ctx.Value(authorizationContextKey{}).(AuthorizationContext)
	if !ok {
		return AuthorizationContext{}, false
	}

	normalized, err := authorization.Normalize()
	if err != nil {
		return AuthorizationContext{}, false
	}

	return normalized, true
}

func WithFullAccess(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, fullAccessContextKey{}, true)
}

func HasFullAccess(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	enabled, _ := ctx.Value(fullAccessContextKey{}).(bool)
	return enabled
}

func WithPreset(ctx context.Context, preset Preset) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isValidPreset(preset) {
		return ctx
	}

	return context.WithValue(ctx, presetContextKey{}, preset)
}

func PresetFromContext(ctx context.Context) (Preset, bool) {
	if ctx == nil {
		return "", false
	}

	preset, ok := ctx.Value(presetContextKey{}).(Preset)
	return preset, ok && isValidPreset(preset)
}

func WithAuthorizedOperations(ctx context.Context, operations []Operation) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	normalized := make([]Operation, 0, len(operations))
	for _, operation := range operations {
		value, err := operation.Normalize()
		if err == nil {
			normalized = append(normalized, value)
		}
	}
	if len(normalized) == 0 {
		return ctx
	}

	return context.WithValue(ctx, authorizedOperationsContextKey{}, normalized)
}

func IsOperationAuthorized(ctx context.Context, operation Operation) bool {
	if ctx == nil {
		return false
	}

	operation, err := operation.Normalize()
	if err != nil {
		return false
	}

	authorized, _ := ctx.Value(authorizedOperationsContextKey{}).([]Operation)
	for _, candidate := range authorized {
		if candidate.Resource == operation.Resource &&
			candidate.Action == operation.Action &&
			candidate.Target == operation.Target &&
			candidate.TargetScope == operation.TargetScope {
			return true
		}
	}

	return false
}

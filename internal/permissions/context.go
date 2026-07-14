package permissions

import (
	"context"
)

type authorizationContextKey struct{}

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

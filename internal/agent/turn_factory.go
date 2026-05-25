package agent

import (
	"context"

	"github.com/wandxy/hand/internal/environment"
	models "github.com/wandxy/hand/internal/model"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

func (a *Agent) newTurn(
	runtimeEnv environment.Environment,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
) *Turn {
	if invokeToolFn == nil {
		invokeToolFn = a.invokeToolWithEnvironment
	}

	sessionStore := NewSessionStore(a.stateMgr)
	toolRegistry := NewToolRegistry(runtimeEnv, func(
		ctx context.Context,
		env environment.Environment,
		toolCall models.ToolCall,
	) handmsg.Message {
		return invokeToolFn(ctx, env, toolCall)
	})

	return NewTurnWithSessionStore(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		sessionStore,
		sessionStore,
		toolRegistry,
		ToolPolicyFromEnvironment(runtimeEnv),
		NewPromptProvider(runtimeEnv),
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		func(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
			return invokeToolFn(ctx, runtimeEnv, toolCall)
		},
	)
}

package agent

import (
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/host"
)

func (a *Agent) newTurn(
	runtimeEnv environment.Environment,
	invokeToolFn host.ToolInvoker,
) *Turn {
	if invokeToolFn == nil {
		invokeToolFn = a.invokeToolWithEnvironment
	}

	sessionStore := host.NewSessionStore(a.stateMgr)
	toolRegistry := host.NewToolRegistry(runtimeEnv, invokeToolFn)

	return NewTurnWithSessionStore(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		sessionStore,
		sessionStore,
		toolRegistry,
		host.ToolPolicyFromEnvironment(runtimeEnv),
		host.NewPromptProvider(runtimeEnv),
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		nil,
	)
}

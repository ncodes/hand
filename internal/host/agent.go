package host

import (
	"context"

	internalagent "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
)

type Agent struct {
	inner *internalagent.Agent
}

func NewAgent(
	ctx context.Context,
	cfg *config.Config,
	modelClient models.Client,
	optionalSummary ...models.Client,
) *Agent {
	return &Agent{inner: internalagent.NewAgent(ctx, cfg, modelClient, optionalSummary...)}
}

func (a *Agent) Start(ctx context.Context) error {
	return a.inner.Start(ctx)
}

func (a *Agent) Close() error {
	return a.inner.Close()
}

func (a *Agent) Respond(ctx context.Context, msg string, opts agentcore.RespondOptions) (string, error) {
	return a.inner.Respond(ctx, msg, opts)
}

func (a *Agent) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	return a.inner.CreateSession(ctx, id)
}

func (a *Agent) ListSessions(ctx context.Context) ([]storage.Session, error) {
	return a.inner.ListSessions(ctx)
}

func (a *Agent) UseSession(ctx context.Context, id string) error {
	return a.inner.UseSession(ctx, id)
}

func (a *Agent) CurrentSession(ctx context.Context) (storage.Session, error) {
	return a.inner.CurrentSession(ctx)
}

func (a *Agent) RecallSessionSummary(ctx context.Context, id string) (storage.SessionSummary, error) {
	return a.inner.RecallSessionSummary(ctx, id)
}

func (a *Agent) CompactSession(ctx context.Context, id string) (agentcore.CompactSessionResult, error) {
	return a.inner.CompactSession(ctx, id)
}

func (a *Agent) RepairSession(ctx context.Context, opts RepairSessionOptions) (RepairSessionResult, error) {
	return a.inner.RepairSession(ctx, opts)
}

func (a *Agent) ContextStatus(ctx context.Context, id string) (agentcore.ContextStatus, error) {
	return a.inner.ContextStatus(ctx, id)
}

func (a *Agent) GetSession(ctx context.Context, id string) (agentcore.ContextStatus, error) {
	return a.ContextStatus(ctx, id)
}

func (a *Agent) GetSessionTimeline(
	ctx context.Context,
	opts agentcore.SessionTimelineOptions,
) (agentcore.SessionTimeline, error) {
	return a.inner.GetSessionTimeline(ctx, opts)
}

func (a *Agent) TurnMessages() []handmsg.Message {
	return a.inner.TurnMessages()
}

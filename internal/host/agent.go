package host

import (
	"context"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
)

type Agent struct {
	inner *agent.Agent
}

func NewAgent(
	ctx context.Context,
	cfg *config.Config,
	modelClient models.Client,
	optionalSummary ...models.Client,
) *Agent {
	return &Agent{inner: agent.NewAgent(ctx, cfg, modelClient, optionalSummary...)}
}

func (a *Agent) Start(ctx context.Context) error {
	return a.inner.Start(ctx)
}

func (a *Agent) Close() error {
	return a.inner.Close()
}

func (a *Agent) Respond(ctx context.Context, msg string, opts RespondOptions) (string, error) {
	return a.inner.Respond(ctx, msg, agent.RespondOptions{
		Instruct:     opts.Instruct,
		SessionID:    opts.SessionID,
		Stream:       opts.Stream,
		OnTraceEvent: opts.OnTraceEvent,
		OnEvent: func(event agent.Event) {
			if opts.OnEvent == nil {
				return
			}
			opts.OnEvent(eventFromAgentEvent(event))
		},
	})
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

func (a *Agent) CompactSession(ctx context.Context, id string) (CompactSessionResult, error) {
	result, err := a.inner.CompactSession(ctx, id)
	return compactSessionResultFromAgentResult(result), err
}

func (a *Agent) RepairSession(ctx context.Context, opts RepairSessionOptions) (RepairSessionResult, error) {
	return a.inner.RepairSession(ctx, opts)
}

func (a *Agent) ContextStatus(ctx context.Context, id string) (ContextStatus, error) {
	status, err := a.inner.ContextStatus(ctx, id)
	return contextStatusFromAgentStatus(status), err
}

func (a *Agent) GetSession(ctx context.Context, id string) (ContextStatus, error) {
	return a.ContextStatus(ctx, id)
}

func (a *Agent) GetSessionTimeline(ctx context.Context, opts SessionTimelineOptions) (SessionTimeline, error) {
	timeline, err := a.inner.GetSessionTimeline(ctx, agent.SessionTimelineOptions{
		SessionID:     opts.SessionID,
		MessageOffset: opts.MessageOffset,
		MessageLimit:  opts.MessageLimit,
		TraceOffset:   opts.TraceOffset,
		TraceLimit:    opts.TraceLimit,
	})
	return sessionTimelineFromAgentTimeline(timeline), err
}

func (a *Agent) TurnMessages() []handmsg.Message {
	return a.inner.TurnMessages()
}

func eventFromAgentEvent(event agent.Event) Event {
	return Event{
		Kind:       event.Kind,
		Channel:    event.Channel,
		Text:       event.Text,
		TraceEvent: event.TraceEvent,
	}
}

func compactSessionResultFromAgentResult(result agent.CompactSessionResult) CompactSessionResult {
	return CompactSessionResult{
		SessionID:            result.SessionID,
		SourceEndOffset:      result.SourceEndOffset,
		SourceMessageCount:   result.SourceMessageCount,
		UpdatedAt:            result.UpdatedAt,
		CurrentContextLength: result.CurrentContextLength,
		TotalContextLength:   result.TotalContextLength,
	}
}

func contextStatusFromAgentStatus(status agent.ContextStatus) ContextStatus {
	return ContextStatus{
		SessionID:        status.SessionID,
		Offset:           status.Offset,
		Size:             status.Size,
		Length:           status.Length,
		Used:             status.Used,
		Remaining:        status.Remaining,
		UsedPct:          status.UsedPct,
		RemainingPct:     status.RemainingPct,
		CreatedAt:        status.CreatedAt,
		UpdatedAt:        status.UpdatedAt,
		CompactionStatus: status.CompactionStatus,
	}
}

func sessionTimelineFromAgentTimeline(timeline agent.SessionTimeline) SessionTimeline {
	return SessionTimeline{
		SessionID:             timeline.SessionID,
		Title:                 timeline.Title,
		TitleSource:           timeline.TitleSource,
		Messages:              sessionTimelineMessagesFromAgentMessages(timeline.Messages),
		TraceEvents:           sessionTimelineTraceEventsFromAgentEvents(timeline.TraceEvents),
		MessagesHasMore:       timeline.MessagesHasMore,
		TracesHasMore:         timeline.TracesHasMore,
		TracesTruncatedBefore: timeline.TracesTruncatedBefore,
		FirstTraceSequence:    timeline.FirstTraceSequence,
		LastTraceSequence:     timeline.LastTraceSequence,
	}
}

func sessionTimelineMessagesFromAgentMessages(messages []agent.SessionTimelineMessage) []SessionTimelineMessage {
	if len(messages) == 0 {
		return nil
	}

	result := make([]SessionTimelineMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, SessionTimelineMessage{
			Message: message.Message,
			Offset:  message.Offset,
		})
	}

	return result
}

func sessionTimelineTraceEventsFromAgentEvents(events []agent.SessionTimelineTraceEvent) []SessionTimelineTraceEvent {
	if len(events) == 0 {
		return nil
	}

	result := make([]SessionTimelineTraceEvent, 0, len(events))
	for _, event := range events {
		result = append(result, SessionTimelineTraceEvent{Event: event.Event})
	}

	return result
}

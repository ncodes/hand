package agent

import (
	"context"
	"errors"

	storage "github.com/wandxy/hand/internal/state/core"
)

const (
	defaultSessionTimelineLimit = 100
	maxSessionTimelineLimit     = 500
)

func (a *Agent) GetSessionTimeline(ctx context.Context, opts SessionTimelineOptions) (SessionTimeline, error) {
	if a == nil {
		return SessionTimeline{}, errors.New("agent is required")
	}
	if a.stateMgr == nil {
		return SessionTimeline{}, errors.New("state manager is required")
	}

	if err := checkTimelineOptions(opts); err != nil {
		return SessionTimeline{}, err
	}

	ctx = normalizeContext(ctx)
	session, err := a.stateMgr.Resolve(ctx, opts.SessionID)
	if err != nil {
		return SessionTimeline{}, err
	}

	messages, messagesHasMore, err := a.loadTimelineMessages(ctx, session.ID, opts)
	if err != nil {
		return SessionTimeline{}, err
	}

	traceEvents, tracesHasMore, tracesTruncatedBefore, err := a.loadTimelineTraceEvents(ctx, session.ID, opts)
	if err != nil {
		return SessionTimeline{}, err
	}

	return SessionTimeline{
		SessionID:             session.ID,
		Title:                 session.Title,
		TitleSource:           session.TitleSource,
		Messages:              messages,
		TraceEvents:           traceEvents,
		MessagesHasMore:       messagesHasMore,
		TracesHasMore:         tracesHasMore,
		TracesTruncatedBefore: tracesTruncatedBefore,
		FirstTraceSequence:    getFirstTraceSequence(traceEvents),
		LastTraceSequence:     getLastTraceSequence(traceEvents),
	}, nil
}

func (a *Agent) loadTimelineMessages(
	ctx context.Context,
	sessionID string,
	opts SessionTimelineOptions,
) ([]SessionTimelineMessage, bool, error) {
	limit := getTimelineLimit(opts.MessageLimit)
	offset := opts.MessageOffset
	hasMoreBefore := false
	if opts.MessageOffset == 0 && opts.MessageLimit <= 0 {
		count, err := a.stateMgr.CountMessages(ctx, sessionID, storage.MessageQueryOptions{})
		if err != nil {
			return nil, false, err
		}
		if count > limit {
			offset = count - limit
			hasMoreBefore = true
		}
	}

	messages, err := a.stateMgr.GetMessages(ctx, sessionID, storage.MessageQueryOptions{
		Order:  storage.MessageOrderAsc,
		Offset: offset,
		Limit:  limit + 1,
	})
	if err != nil {
		return nil, false, err
	}

	hasMore := hasMoreBefore || len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	timelineMessages := make([]SessionTimelineMessage, 0, len(messages))
	for index, message := range messages {
		timelineMessages = append(timelineMessages, SessionTimelineMessage{
			Offset:  offset + index,
			Message: message,
		})
	}

	return timelineMessages, hasMore, nil
}

func (a *Agent) loadTimelineTraceEvents(
	ctx context.Context,
	sessionID string,
	opts SessionTimelineOptions,
) ([]SessionTimelineTraceEvent, bool, bool, error) {

	limit := getTimelineLimit(opts.TraceLimit)

	query := storage.TraceQuery{
		SessionID:   sessionID,
		Limit:       limit + 1,
		MinSequence: opts.TraceOffset,
	}

	defaultRecentTail := opts.TraceOffset == 0 && opts.TraceLimit <= 0
	if defaultRecentTail {
		query.Desc = true
	}

	result, err := a.stateMgr.ListTraceEvents(ctx, query)
	if err != nil {
		if errors.Is(err, storage.ErrTraceStoreUnsupported) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}

	events := result.Events

	truncatedBefore, err := a.hasTruncatedTraceHistory(ctx, sessionID, opts.TraceOffset, events)
	if err != nil {
		return nil, false, false, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	if defaultRecentTail {
		reverseTraceEvents(events)
	}

	timelineEvents := make([]SessionTimelineTraceEvent, 0, len(events))
	for _, event := range events {
		timelineEvents = append(timelineEvents, SessionTimelineTraceEvent{
			Event: storage.CloneTraceEvent(event),
		})
	}

	return timelineEvents, hasMore, truncatedBefore, nil
}

func reverseTraceEvents(events []storage.TraceEvent) {
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
}

func (a *Agent) hasTruncatedTraceHistory(
	ctx context.Context,
	sessionID string,
	traceOffset int,
	events []storage.TraceEvent,
) (bool, error) {
	if traceOffset == 0 && len(events) > 0 {
		return events[0].Sequence > 1, nil
	}

	result, err := a.stateMgr.ListTraceEvents(ctx, storage.TraceQuery{
		SessionID: sessionID,
		Limit:     1,
	})
	if err != nil {
		if errors.Is(err, storage.ErrTraceStoreUnsupported) {
			return false, nil
		}

		return false, err
	}
	if len(result.Events) == 0 {
		return false, nil
	}

	return result.Events[0].Sequence > 1, nil
}

func checkTimelineOptions(opts SessionTimelineOptions) error {
	if opts.MessageOffset < 0 {
		return errors.New("message offset must be greater than or equal to zero")
	}
	if opts.MessageLimit < 0 {
		return errors.New("message limit must be greater than or equal to zero")
	}
	if opts.TraceOffset < 0 {
		return errors.New("trace offset must be greater than or equal to zero")
	}
	if opts.TraceLimit < 0 {
		return errors.New("trace limit must be greater than or equal to zero")
	}

	return nil
}

func getTimelineLimit(limit int) int {
	if limit <= 0 {
		return defaultSessionTimelineLimit
	}
	if limit > maxSessionTimelineLimit {
		return maxSessionTimelineLimit
	}

	return limit
}

func getFirstTraceSequence(events []SessionTimelineTraceEvent) int {
	if len(events) == 0 {
		return 0
	}

	return events[0].Event.Sequence
}

func getLastTraceSequence(events []SessionTimelineTraceEvent) int {
	if len(events) == 0 {
		return 0
	}

	return events[len(events)-1].Event.Sequence
}

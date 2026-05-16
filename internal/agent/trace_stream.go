package agent

import (
	"strings"
	"time"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/trace"
)

// fanoutTraceSession writes trace records to a primary session and mirrors
// selected sanitized events to a live callback.
type fanoutTraceSession struct {
	sessionID string
	primary   trace.Session
	redactor  guardrails.Redactor
	onEvent   func(trace.Event)
}

// newFanoutTraceSession creates a trace session that writes to a primary session
// and mirrors selected sanitized events to a live callback.
func newFanoutTraceSession(
	primary trace.Session,
	sessionID string,
	onEvent func(trace.Event),
) trace.Session {
	if onEvent == nil {
		if primary == nil {
			return trace.NoopSession()
		}
		return primary
	}
	if primary == nil {
		primary = trace.NoopSession()
	}

	if value := strings.TrimSpace(primary.ID()); value != "" {
		sessionID = value
	}

	return fanoutTraceSession{
		sessionID: strings.TrimSpace(sessionID),
		primary:   primary,
		redactor:  guardrails.NewRedactor(),
		onEvent:   onEvent,
	}
}

func (s fanoutTraceSession) ID() string {
	if s.primary != nil {
		if id := strings.TrimSpace(s.primary.ID()); id != "" {
			return id
		}
	}

	return s.sessionID
}

func (s fanoutTraceSession) Record(eventType string, payload any) {
	if s.primary != nil {
		s.primary.Record(eventType, payload)
	}
	if s.onEvent == nil || !isStreamableTraceEvent(eventType) {
		return
	}

	event := trace.Event{
		SessionID: s.ID(),
		Type:      strings.TrimSpace(eventType),
		Timestamp: time.Now().UTC(),
	}
	if payload != nil {
		event.Payload = s.redactor.Sanitize(payload)
	}

	s.onEvent(event)
}

func (s fanoutTraceSession) Close() {
	if s.primary != nil {
		s.primary.Close()
	}
}

func isStreamableTraceEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case trace.EvtToolInvocationStarted,
		trace.EvtToolInvocationCompleted,
		trace.EvtInputSafetyBlocked,
		trace.EvtOutputSafetyApplied,
		trace.EvtSessionFailed,
		trace.EvtPlanHydrated,
		trace.EvtFinalAssistantResponse:
		return true
	default:
		return false
	}
}

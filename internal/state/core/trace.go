package core

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

type TraceEvent struct {
	ID        uint
	SessionID string
	Sequence  int
	Type      string
	Timestamp time.Time
	Payload   any
}

type TraceQuery struct {
	SessionID string
	Types     []string
	Limit     int
	Offset    int
	Desc      bool
}

type TraceResult struct {
	Events []TraceEvent
}

type TraceStore interface {
	AppendTraceEvent(context.Context, TraceEvent) (TraceEvent, error)
	ListTraceEvents(context.Context, TraceQuery) (TraceResult, error)
	PruneTraceEvents(context.Context, string, int) error
}

func CloneTraceEvent(event TraceEvent) TraceEvent {
	event.Payload = cloneTracePayload(event.Payload)
	return event
}

func cloneTracePayload(payload any) any {
	if payload == nil {
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func TraceEventMatchesQuery(event TraceEvent, query TraceQuery) bool {
	if sessionID := strings.TrimSpace(query.SessionID); sessionID != "" && event.SessionID != sessionID {
		return false
	}
	if types := NormalizeTraceTypes(query.Types); len(types) > 0 && !slices.Contains(types, event.Type) {
		return false
	}
	return true
}

func NormalizeTraceTypes(types []string) []string {
	seen := make(map[string]struct{}, len(types))
	results := make([]string, 0, len(types))
	for _, eventType := range types {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		results = append(results, eventType)
	}
	return results
}

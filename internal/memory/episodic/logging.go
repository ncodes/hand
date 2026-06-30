package episodic

import (
	"maps"

	"github.com/rs/zerolog"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/stringx"
)

var extractionLog = logutils.Module("memory.extraction")

// recordFailure keeps extraction failure logging and tracing consistent across
// normalization, window loading, model calls, and persistence errors.
func recordFailure(recorder TraceRecorder, req normalizedRequest, err error) {
	recordTrace(recorder, trace.EvtMemoryExtractionFailed, getTracePayload(req, map[string]any{"error": err.Error()}))
	logExtraction("failed", req, map[string]any{"error": err.Error()})
}

func recordTrace(recorder TraceRecorder, event string, payload map[string]any) {
	if recorder == nil {
		return
	}
	typedPayload, ok := trace.DecodePayload(event, payload)
	if !ok {
		typedPayload = payload
	}
	recorder.Record(event, typedPayload)
}

// getTracePayload adds the common extraction coordinates to every event so a trace
// viewer can group logs by session and source message window.
func getTracePayload(req normalizedRequest, fields map[string]any) map[string]any {
	payload := map[string]any{
		"session_id":   stringx.String(req.SessionID).Trim(),
		"offset_start": req.OffsetStart,
		"offset_end":   req.OffsetEnd,
		"trigger":      stringx.String(req.Trigger).Trim(),
	}
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

// logExtraction mirrors trace events to debug logs. The trace has the complete
// event timeline; the log gives operators a readable live stream.
func logExtraction(event string, req normalizedRequest, fields map[string]any) {
	entry := extractionLog.Debug().
		Str("session_id", stringx.String(req.SessionID).Trim()).
		Str("trigger", stringx.String(req.Trigger).Trim()).
		Int("offset_start", req.OffsetStart).
		Int("offset_end", req.OffsetEnd)
	for key, value := range fields {
		entry = logField(entry, key, value)
	}
	entry.Msg("memory extraction " + event)
}

func recordBackgroundFailure(
	recorder TraceRecorder,
	runID string,
	sessionID string,
	messageCount int,
	reason string,
	err error,
) {
	fields := map[string]any{"error": err.Error()}
	recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundFailed, getBackgroundPayload(runID, sessionID, messageCount, reason, fields))
	logBackground("failed", runID, sessionID, messageCount, reason, fields)
}

func recordBackgroundTrace(
	recorder TraceRecorder,
	event string,
	payload map[string]any,
) {
	if recorder == nil {
		return
	}
	typedPayload, ok := trace.DecodePayload(event, payload)
	if !ok {
		typedPayload = payload
	}
	recorder.Record(event, typedPayload)
}

// getBackgroundPayload keeps background events compact. Empty session/reason fields
// are omitted so run-level events are not cluttered with meaningless values.
func getBackgroundPayload(
	runID string,
	sessionID string,
	messageCount int,
	reason string,
	fields map[string]any,
) map[string]any {
	payload := map[string]any{
		"run_id": stringx.String(runID).Trim(),
	}
	if stringx.String(sessionID).Trim() != "" {
		payload["session_id"] = stringx.String(sessionID).Trim()
	}
	if messageCount > 0 {
		payload["message_count"] = messageCount
	}
	if stringx.String(reason).Trim() != "" {
		payload["trigger_reason"] = stringx.String(reason).Trim()
	}
	maps.Copy(payload, fields)
	return payload
}

func logBackground(
	event string,
	_ string,
	sessionID string,
	messageCount int,
	reason string,
	fields map[string]any,
) {
	entry := extractionLog.Debug().
		Str("session_id", stringx.String(sessionID).Trim()).
		Str("trigger_reason", stringx.String(reason).Trim()).
		Int("message_count", messageCount)
	for key, value := range fields {
		entry = logField(entry, key, value)
	}
	entry.Msg("memory episodic background " + event)
}

// logField preserves useful scalar types instead of stringifying everything,
// which keeps downstream log filters effective.
func logField(event *zerolog.Event, key string, value any) *zerolog.Event {
	switch typed := value.(type) {
	case string:
		return event.Str(key, typed)
	case int:
		return event.Int(key, typed)
	case int64:
		return event.Int64(key, typed)
	default:
		return event.Interface(key, value)
	}
}

package episodic

import (
	"maps"

	"github.com/rs/zerolog"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(req.SessionID)
	stringValue2 := str.String(req.Trigger)
	payload := map[string]any{
		"session_id":   stringValue1.Trim(),
		"offset_start": req.OffsetStart,
		"offset_end":   req.OffsetEnd,
		"trigger":      stringValue2.Trim(),
	}
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

// logExtraction mirrors trace events to debug logs. The trace has the complete
// event timeline; the log gives operators a readable live stream.
func logExtraction(event string, req normalizedRequest, fields map[string]any) {
	stringValue3 := str.String(req.SessionID)
	stringValue4 := str.String(req.Trigger)
	entry := extractionLog.Debug().
		Str("session_id", stringValue3.Trim()).
		Str("trigger", stringValue4.Trim()).
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
	stringValue5 := str.String(runID)
	payload := map[string]any{
		"run_id": stringValue5.Trim(),
	}
	stringValue6 := str.String(sessionID)
	if stringValue6.Trim() != "" {
		stringValue8 := str.String(sessionID)
		payload["session_id"] = stringValue8.Trim()
	}
	if messageCount > 0 {
		payload["message_count"] = messageCount
	}
	stringValue7 := str.String(reason)
	if stringValue7.Trim() != "" {
		stringValue9 := str.String(reason)
		payload["trigger_reason"] = stringValue9.Trim()
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
	stringValue10 := str.String(sessionID)
	stringValue11 := str.String(reason)
	entry := extractionLog.Debug().
		Str("session_id", stringValue10.Trim()).
		Str("trigger_reason", stringValue11.Trim()).
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

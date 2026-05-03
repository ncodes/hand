package episodic

import (
	"maps"
	"strings"

	"github.com/rs/zerolog"

	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

var extractionLog = logutils.InitLogger("memory.extraction")

func recordFailure(recorder TraceRecorder, req normalizedRequest, err error) {
	recordTrace(recorder, trace.EvtMemoryExtractionFailed, tracePayload(req, map[string]any{"error": err.Error()}))
	logExtraction("failed", req, map[string]any{"error": err.Error()})
}

func recordTrace(recorder TraceRecorder, event string, payload map[string]any) {
	if recorder == nil {
		return
	}
	recorder.Record(event, payload)
}

func tracePayload(req normalizedRequest, fields map[string]any) map[string]any {
	payload := map[string]any{
		"session_id":   strings.TrimSpace(req.SessionID),
		"offset_start": req.OffsetStart,
		"offset_end":   req.OffsetEnd,
		"trigger":      strings.TrimSpace(req.Trigger),
	}
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

func logExtraction(event string, req normalizedRequest, fields map[string]any) {
	entry := extractionLog.Debug().
		Str("event", "memory extraction "+event).
		Str("session_id", strings.TrimSpace(req.SessionID)).
		Str("trigger", strings.TrimSpace(req.Trigger)).
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
	recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundFailed, backgroundPayload(runID, sessionID, messageCount, reason, fields))
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
	recorder.Record(event, payload)
}

func backgroundPayload(
	runID string,
	sessionID string,
	messageCount int,
	reason string,
	fields map[string]any,
) map[string]any {
	payload := map[string]any{
		"run_id": strings.TrimSpace(runID),
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
	}
	if messageCount > 0 {
		payload["message_count"] = messageCount
	}
	if strings.TrimSpace(reason) != "" {
		payload["trigger_reason"] = strings.TrimSpace(reason)
	}
	maps.Copy(payload, fields)
	return payload
}

func logBackground(
	event string,
	runID string,
	sessionID string,
	messageCount int,
	reason string,
	fields map[string]any,
) {
	entry := extractionLog.Debug().
		Str("event", "memory episodic background "+event).
		Str("background_run_id", strings.TrimSpace(runID)).
		Str("session_id", strings.TrimSpace(sessionID)).
		Str("trigger_reason", strings.TrimSpace(reason)).
		Int("message_count", messageCount)
	for key, value := range fields {
		entry = logField(entry, key, value)
	}
	entry.Msg("memory episodic background " + event)
}

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

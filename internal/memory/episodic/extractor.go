package episodic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/memory"
	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

const (
	DefaultWindowSize      = 20
	MaxWindowSize          = 50
	DefaultMaxWindowChars  = 6000
	MaxWindowChars         = 20000
	DefaultMaxWindowTokens = DefaultMaxWindowChars / constants.RoughTokenCharRatio
	MaxWindowTokens        = MaxWindowChars / constants.RoughTokenCharRatio
	DefaultMaxWindows      = 5
	MaxWindows             = 50
	defaultTrigger         = "command"
)

var extractionLog = logutils.InitLogger("memory.extraction")

// Request configures episodic extraction for a session or bounded message range.
type Request struct {
	SessionID       string
	OffsetStart     *int
	OffsetEnd       *int
	WindowSize      int
	MaxWindows      int
	MaxWindowChars  int
	MaxWindowTokens int
	Trigger         string
	Trace           TraceRecorder
}

// Result summarizes all windows processed by an extraction request.
type Result struct {
	SessionID      string         `json:"session_id"`
	Windows        []WindowResult `json:"windows,omitempty"`
	MessageCount   int            `json:"message_count"`
	CandidateCount int            `json:"candidate_count"`
	WriteCount     int            `json:"write_count"`
	SkipCount      int            `json:"skip_count"`
}

// WindowResult summarizes extraction work for one source message window.
type WindowResult struct {
	MemoryIDs      []string `json:"memory_ids,omitempty"`
	SkippedIDs     []string `json:"skipped_ids,omitempty"`
	OffsetStart    int      `json:"offset_start"`
	OffsetEnd      int      `json:"offset_end"`
	MessageCount   int      `json:"message_count"`
	CandidateCount int      `json:"candidate_count"`
	WriteCount     int      `json:"write_count"`
	SkipCount      int      `json:"skip_count"`
}

// Service extracts source-linked episodic memories from session message windows.
type Service struct {
	manager *statemanager.Manager
	memory  memory.Provider
	nowFunc func() time.Time
}

// TraceRecorder records extraction trace events when provided by the caller.
type TraceRecorder interface {
	Record(string, any)
}

// NewService creates an episodic extraction service with required dependencies.
func NewService(manager *statemanager.Manager, provider memory.Provider) (*Service, error) {
	if manager == nil {
		return nil, errors.New("state manager is required")
	}
	if provider == nil {
		return nil, errors.New("memory provider is required")
	}

	return &Service{manager: manager, memory: provider}, nil
}

// Extract processes the requested session range in bounded windows and records episodes.
func (s *Service) Extract(ctx context.Context, req Request) (Result, error) {
	started := s.now()
	if s == nil || s.manager == nil {
		return Result{}, errors.New("state manager is required")
	}
	if s.memory == nil {
		return Result{}, errors.New("memory provider is required")
	}

	searcher, writer, err := memoryAccess(ctx, s.memory)
	if err != nil {
		return Result{}, err
	}

	normalized, err := s.normalizeRequest(ctx, req)
	if err != nil {
		recordFailure(req.Trace, normalized, err)
		return Result{}, err
	}

	recordTrace(req.Trace, trace.EvtMemoryExtractionStarted, tracePayload(normalized, map[string]any{
		"window_size":       normalized.WindowSize,
		"max_windows":       normalized.MaxWindows,
		"max_window_chars":  normalized.MaxWindowChars,
		"max_window_tokens": normalized.MaxWindowTokens,
	}))
	logExtraction("started", normalized, map[string]any{
		"window_size":       normalized.WindowSize,
		"max_windows":       normalized.MaxWindows,
		"max_window_chars":  normalized.MaxWindowChars,
		"max_window_tokens": normalized.MaxWindowTokens,
	})

	result := Result{SessionID: normalized.SessionID}
	for start := normalized.OffsetStart; start < normalized.OffsetEnd; start += normalized.WindowSize {
		if normalized.MaxWindows > 0 && len(result.Windows) >= normalized.MaxWindows {
			break
		}

		end := min(start+normalized.WindowSize, normalized.OffsetEnd)
		window := sourceWindow{Start: start, End: end}
		windowResult, err := s.extractWindow(ctx, normalized, window, searcher, writer)
		if err != nil {
			recordFailure(req.Trace, normalized.withRange(window.Start, window.End), err)
			return Result{}, err
		}

		result.Windows = append(result.Windows, windowResult)
		result.MessageCount += windowResult.MessageCount
		result.CandidateCount += windowResult.CandidateCount
		result.WriteCount += windowResult.WriteCount
		result.SkipCount += windowResult.SkipCount
	}

	duration := s.now().Sub(started)
	recordTrace(req.Trace, trace.EvtMemoryExtractionCompleted, tracePayload(normalized, map[string]any{
		"window_count":    len(result.Windows),
		"message_count":   result.MessageCount,
		"candidate_count": result.CandidateCount,
		"write_count":     result.WriteCount,
		"skip_count":      result.SkipCount,
		"duration_ms":     duration.Milliseconds(),
	}))
	logExtraction("completed", normalized, map[string]any{
		"window_count":    len(result.Windows),
		"message_count":   result.MessageCount,
		"candidate_count": result.CandidateCount,
		"write_count":     result.WriteCount,
		"skip_count":      result.SkipCount,
		"duration_ms":     duration.Milliseconds(),
	})

	return result, nil
}

func (s Service) extractWindow(
	ctx context.Context,
	req normalizedRequest,
	window sourceWindow,
	searcher memory.SearchProvider,
	recorder memory.EpisodeProvider,
) (WindowResult, error) {
	windowReq := req.withRange(window.Start, window.End)
	messages, err := s.manager.GetMessages(ctx, req.SessionID, storage.MessageQueryOptions{
		Offset: window.Start,
		Limit:  window.End - window.Start,
	})
	if err != nil {
		return WindowResult{}, err
	}

	traceFields := map[string]any{"message_count": len(messages)}
	recordTrace(req.Trace, trace.EvtMemoryExtractionWindowLoaded, tracePayload(windowReq, traceFields))
	logExtraction("window_loaded", windowReq, traceFields)

	candidate, ok := candidateFromMessages(req, window, messages)
	candidateCount := 0
	if ok {
		candidateCount = 1
	}
	traceFields = map[string]any{"candidate_count": candidateCount}
	recordTrace(req.Trace, trace.EvtMemoryExtractionExtractorRequested, tracePayload(windowReq, traceFields))
	recordTrace(req.Trace, trace.EvtMemoryExtractionCandidates, tracePayload(windowReq, traceFields))
	traceFields = map[string]any{"message_count": len(messages), "candidate_count": candidateCount}
	logExtraction("candidates", windowReq, traceFields)

	result := WindowResult{
		OffsetStart:    window.Start,
		OffsetEnd:      window.End,
		MessageCount:   len(messages),
		CandidateCount: candidateCount,
	}
	if !ok {
		return result, nil
	}

	existing, err := searcher.Search(ctx, memory.SearchQuery{
		Kinds:    []memory.Kind{memory.KindEpisodic},
		Statuses: []memory.Status{memory.StatusActive},
		Tags:     []string{sourceRangeTag(req.SessionID, window.Start, window.End)},
		Limit:    1,
	})
	if err != nil {
		return WindowResult{}, err
	}
	if len(existing.Hits) > 0 {
		id := strings.TrimSpace(existing.Hits[0].Item.ID)
		result.SkipCount = 1
		result.SkippedIDs = append(result.SkippedIDs, id)
		traceFields := map[string]any{"memory_id": id}
		recordTrace(req.Trace, trace.EvtMemoryExtractionDuplicateSkipped, tracePayload(windowReq, traceFields))
		logExtraction("duplicate_skipped", windowReq, traceFields)
		return result, nil
	}

	item, err := recorder.RecordEpisode(ctx, memory.EpisodeRecord{Item: candidate})
	if err != nil {
		return WindowResult{}, err
	}

	result.WriteCount = 1
	result.MemoryIDs = append(result.MemoryIDs, item.ID)
	traceFields = map[string]any{"memory_id": item.ID}
	recordTrace(req.Trace, trace.EvtMemoryExtractionMemoryWritten, tracePayload(windowReq, traceFields))
	logExtraction("memory_written", windowReq, traceFields)

	return result, nil
}

func (s Service) normalizeRequest(ctx context.Context, req Request) (normalizedRequest, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		currentSessionID, err := s.manager.CurrentSession(ctx)
		if err != nil {
			return normalizedRequest{}, err
		}
		sessionID = currentSessionID
	}

	count, err := s.manager.CountMessages(ctx, sessionID, storage.MessageQueryOptions{})
	if err != nil {
		return normalizedRequest{}, err
	}

	start := 0
	if req.OffsetStart != nil {
		start = *req.OffsetStart
	}
	end := count
	if req.OffsetEnd != nil {
		end = *req.OffsetEnd
	}
	if start < 0 {
		return normalizedRequest{}, errors.New("offset_start must be greater than or equal to zero")
	}
	if end < start {
		return normalizedRequest{}, errors.New("offset_end must be greater than or equal to offset_start")
	}
	if end > count {
		end = count
	}

	windowSize := req.WindowSize
	if windowSize <= 0 {
		windowSize = DefaultWindowSize
	}
	if windowSize > MaxWindowSize {
		windowSize = MaxWindowSize
	}
	if req.MaxWindows < 0 {
		return normalizedRequest{}, errors.New("max_windows must be greater than or equal to zero")
	}

	maxChars := req.MaxWindowChars
	if maxChars <= 0 {
		maxChars = DefaultMaxWindowChars
	}
	if maxChars > MaxWindowChars {
		maxChars = MaxWindowChars
	}
	if req.MaxWindowTokens < 0 {
		return normalizedRequest{}, errors.New("max_window_tokens must be greater than or equal to zero")
	}

	maxTokens := req.MaxWindowTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxWindowTokens
	}
	if maxTokens > MaxWindowTokens {
		maxTokens = MaxWindowTokens
	}

	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = defaultTrigger
	}

	return normalizedRequest{
		SessionID:       sessionID,
		OffsetStart:     start,
		OffsetEnd:       end,
		WindowSize:      windowSize,
		MaxWindows:      req.MaxWindows,
		MaxWindowChars:  maxChars,
		MaxWindowTokens: maxTokens,
		Trigger:         trigger,
		Trace:           req.Trace,
	}, nil
}

func (s *Service) now() time.Time {
	if s != nil && s.nowFunc != nil {
		return s.nowFunc().UTC()
	}
	return time.Now().UTC()
}

// normalizedRequest is the validated internal form used while processing windows.
type normalizedRequest struct {
	SessionID       string
	OffsetStart     int
	OffsetEnd       int
	WindowSize      int
	MaxWindows      int
	MaxWindowChars  int
	MaxWindowTokens int
	Trigger         string
	Trace           TraceRecorder
}

func (r normalizedRequest) withRange(start int, end int) normalizedRequest {
	r.OffsetStart = start
	r.OffsetEnd = end
	return r
}

// sourceWindow identifies the inclusive/exclusive message offsets for one window.
type sourceWindow struct {
	Start int
	End   int
}

func candidateFromMessages(
	req normalizedRequest,
	window sourceWindow,
	messages []handmsg.Message,
) (memory.MemoryItem, bool) {
	messageIDs := make([]uint, 0, len(messages))
	offsets := make([]int, 0, len(messages))
	lines := make([]string, 0, len(messages))
	for idx, message := range messages {
		line := messageLine(message)
		if line == "" {
			continue
		}
		messageIDs = append(messageIDs, message.ID)
		offsets = append(offsets, window.Start+idx)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return memory.MemoryItem{}, false
	}

	sourceTag := sourceRangeTag(req.SessionID, window.Start, window.End)
	id := memoryID(req.SessionID, window.Start, window.End)
	text := truncateRunes(strings.Join(lines, "\n"), req.windowCharLimit())
	return memory.MemoryItem{
		ID:     id,
		Kind:   memory.KindEpisodic,
		Status: memory.StatusActive,
		Title:  fmt.Sprintf("Session %s messages %d-%d", req.SessionID, window.Start, window.End-1),
		Text:   text,
		Tags:   []string{"episodic", sourceTag},
		Metadata: map[string]string{
			"source_session_id": req.SessionID,
			"source_start":      strconv.Itoa(window.Start),
			"source_end":        strconv.Itoa(window.End),
			"trigger":           req.Trigger,
		},
		SourceLinks: []memory.SourceLink{{
			SessionID:     req.SessionID,
			MessageIDs:    messageIDs,
			Offsets:       offsets,
			CreatedBy:     req.Trigger,
			CreatedReason: "episodic_memory_extraction",
		}},
		Confidence: 0.5,
	}, true
}

func (r normalizedRequest) windowCharLimit() int {
	limit := r.MaxWindowChars
	tokenChars := r.MaxWindowTokens * constants.RoughTokenCharRatio
	if limit <= 0 || tokenChars < limit {
		limit = tokenChars
	}
	return limit
}

func messageLine(message handmsg.Message) string {
	parts := make([]string, 0, 2+len(message.ToolCalls))
	if content := strings.TrimSpace(validUTF8(message.Content)); content != "" {
		parts = append(parts, content)
	}
	for _, call := range message.ToolCalls {
		name := strings.TrimSpace(call.Name)
		input := strings.TrimSpace(validUTF8(call.Input))
		if name == "" && input == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace("tool_call "+name+": "+input))
	}
	if len(parts) == 0 {
		return ""
	}

	role := strings.TrimSpace(string(message.Role))
	if role == "" {
		role = "message"
	}
	if toolName := strings.TrimSpace(message.Name); message.Role == handmsg.RoleTool && toolName != "" {
		role += ":" + toolName
	}
	return role + ": " + strings.Join(parts, " ")
}

func memoryAccess(ctx context.Context, provider memory.Provider) (memory.SearchProvider, memory.EpisodeProvider, error) {
	caps, err := provider.Capabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	searcher, ok := provider.(memory.SearchProvider)
	if !ok || !caps.SupportsSearch {
		return nil, nil, errors.New("memory search is not supported by provider")
	}
	recorder, ok := provider.(memory.EpisodeProvider)
	if !ok || !caps.SupportsEpisodeRecording {
		return nil, nil, errors.New("memory episode recording is not supported by provider")
	}
	return searcher, recorder, nil
}

func memoryID(sessionID string, start int, end int) string {
	return "mem_episode_" + sourceRangeHash(sessionID, start, end)
}

func sourceRangeTag(sessionID string, start int, end int) string {
	return "source-range-" + sourceRangeHash(sessionID, start, end)
}

func sourceRangeHash(sessionID string, start int, end int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s:%d:%d", strings.TrimSpace(sessionID), start, end))
	return hex.EncodeToString(sum[:8])
}

func truncateRunes(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars])
}

func validUTF8(value string) string {
	if utf8.ValidString(value) {
		return value
	}
	return strings.ToValidUTF8(value, "")
}

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

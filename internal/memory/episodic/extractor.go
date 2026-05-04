package episodic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wandxy/hand/internal/constants"
	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
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

const (
	episodeKindDecision       = "decision"
	episodeKindOutcome        = "outcome"
	episodeKindToolEvent      = "tool_event"
	episodeKindBlocker        = "blocker"
	episodeKindUserCorrection = "user_correction"
)

const (
	defaultMaxTraceEventsPerWindow = 40
	maxTracePayloadChars           = 1200
)

// Service extracts source-linked episodic memories from session message windows.
type Service struct {
	// manager loads session messages and updates background extraction checkpoints.
	manager StateManager
	// memory searches for existing memories and records proposed episodic memory items.
	memory MemoryRepository
	// extractor proposes curated episodic memory items from bounded message evidence.
	extractor candidateExtractor
	// nowFunc overrides the clock in tests and background scheduling.
	nowFunc func() time.Time
}

// NewService creates a new episodic memory extraction service.
func NewService(manager StateManager, repository MemoryRepository, extractor *LLMExtractor) (*Service, error) {
	if manager == nil {
		return nil, errors.New("state manager is required")
	}
	if repository == nil {
		return nil, errors.New("memory repository is required")
	}
	if extractor == nil {
		return nil, errors.New("memory episode extractor is required")
	}

	return &Service{manager: manager, memory: repository, extractor: extractor}, nil
}

// Extract extracts curated episodic memory items from a bounded message range.
func (s *Service) Extract(ctx context.Context, req Request) (Result, error) {
	started := s.now()
	if s == nil || s.manager == nil {
		return Result{}, errors.New("state manager is required")
	}
	if s.memory == nil {
		return Result{}, errors.New("memory repository is required")
	}
	if s.extractor == nil {
		return Result{}, errors.New("memory episode extractor is required")
	}

	normalized, err := s.normalizeRequest(ctx, req)
	if err != nil {
		recordFailure(req.Trace, normalized, err)
		return Result{}, err
	}

	traceField := map[string]any{
		"window_size":       normalized.WindowSize,
		"max_windows":       normalized.MaxWindows,
		"max_window_chars":  normalized.MaxWindowChars,
		"max_window_tokens": normalized.MaxWindowTokens,
	}
	recordTrace(req.Trace, trace.EvtMemoryExtractionStarted, tracePayload(normalized, traceField))
	logExtraction("started", normalized, traceField)

	result := Result{SessionID: normalized.SessionID}
	for start := normalized.OffsetStart; start < normalized.OffsetEnd; start += normalized.WindowSize {
		if normalized.MaxWindows > 0 && len(result.Windows) >= normalized.MaxWindows {
			break
		}

		end := min(start+normalized.WindowSize, normalized.OffsetEnd)
		window := sourceWindow{Start: start, End: end}
		windowResult, err := s.extractWindow(ctx, normalized, window)
		if err != nil {
			recordFailure(req.Trace, normalized.withRange(window.Start, window.End), err)
			return Result{}, err
		}

		result.Windows = append(result.Windows, windowResult)
		result.MessageCount += windowResult.MessageCount
		result.CandidateCount += windowResult.CandidateCount
		result.WriteCount += windowResult.WriteCount
		result.SkipCount += windowResult.SkipCount

		if normalized.Trigger == backgroundTrigger {
			if err := s.manager.UpdateEpisodicCheckpoint(
				ctx,
				normalized.SessionID,
				windowResult.OffsetEnd,
			); err != nil {
				recordFailure(req.Trace, normalized.withRange(window.Start, window.End), err)
				return Result{}, err
			}
		}
	}

	duration := s.now().Sub(started)
	traceFields := map[string]any{
		"window_count":    len(result.Windows),
		"message_count":   result.MessageCount,
		"candidate_count": result.CandidateCount,
		"write_count":     result.WriteCount,
		"skip_count":      result.SkipCount,
		"duration_ms":     duration.Milliseconds(),
	}
	recordTrace(req.Trace, trace.EvtMemoryExtractionCompleted, tracePayload(normalized, traceFields))
	logExtraction("completed", normalized, traceFields)

	return result, nil
}

func (s Service) extractWindow(
	ctx context.Context,
	req normalizedRequest,
	window sourceWindow,
) (WindowResult, error) {
	windowReq := req.withRange(window.Start, window.End)
	result := WindowResult{
		OffsetStart: window.Start,
		OffsetEnd:   window.End,
	}

	candidateIDs := candidateMemoryIDs(req.SessionID, window.Start, window.End)
	existing, err := s.memory.Search(ctx, storage.MemorySearchQuery{
		IDs:      candidateIDs,
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate, storage.MemoryStatusActive},
		Limit:    len(candidateIDs),
	})
	if err != nil {
		return WindowResult{}, err
	}
	if len(existing.Hits) > 0 {
		result.SkipCount = len(existing.Hits)
		for _, hit := range existing.Hits {
			id := strings.TrimSpace(hit.Item.ID)
			if id != "" {
				result.SkippedIDs = append(result.SkippedIDs, id)
			}
		}
		traceFields := map[string]any{"memory_ids": result.SkippedIDs, "checkpoint_state": "complete"}
		recordTrace(req.Trace, trace.EvtMemoryExtractionDuplicateSkipped, tracePayload(windowReq, traceFields))
		logExtraction("duplicate_skipped", windowReq, traceFields)
		return result, nil
	}

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

	candidates, rejections, err := s.candidatesFromMessages(ctx, req, window, messages)
	if err != nil {
		return WindowResult{}, err
	}

	candidateCount := len(candidates)
	traceFields = map[string]any{"candidate_count": candidateCount}
	recordTrace(req.Trace, trace.EvtMemoryExtractionExtractorRequested, tracePayload(windowReq, traceFields))
	recordTrace(req.Trace, trace.EvtMemoryExtractionCandidates, tracePayload(windowReq, traceFields))
	traceFields = map[string]any{"message_count": len(messages), "candidate_count": candidateCount}
	logExtraction("candidates", windowReq, traceFields)

	for _, rejection := range rejections {
		traceFields = map[string]any{"candidate_kind": rejection.Kind, "rejection_reason": rejection.Reason}
		recordTrace(req.Trace, trace.EvtMemoryExtractionCandidateRejected, tracePayload(windowReq, traceFields))
		logExtraction("candidate_rejected", windowReq, traceFields)
	}

	for _, candidate := range candidates {
		traceFields = map[string]any{
			"memory_id":       candidate.ID,
			"candidate_kind":  candidate.Metadata["candidate_kind"],
			"confidence":      candidate.Confidence,
			"source_quality":  candidate.Metadata["source_quality"],
			"usefulness":      candidate.Metadata["usefulness"],
			"admission_state": candidate.Status,
		}
		recordTrace(req.Trace, trace.EvtMemoryExtractionCandidateGenerated, tracePayload(windowReq, traceFields))
		recordTrace(req.Trace, trace.EvtMemoryExtractionConfidenceScored, tracePayload(windowReq, traceFields))
		recordTrace(req.Trace, trace.EvtMemoryExtractionAdmissionHandoff, tracePayload(windowReq, traceFields))
		logExtraction("candidate_generated", windowReq, traceFields)
	}

	result.MessageCount = len(messages)
	result.CandidateCount = candidateCount

	if len(candidates) == 0 {
		return result, nil
	}

	for _, candidate := range candidates {
		item, err := s.memory.RecordEpisode(ctx, EpisodeRecord{Item: candidate})
		if err != nil {
			return WindowResult{}, err
		}

		result.WriteCount++
		result.MemoryIDs = append(result.MemoryIDs, item.ID)

		traceFields = map[string]any{
			"memory_id":       item.ID,
			"candidate_kind":  item.Metadata["candidate_kind"],
			"confidence":      item.Confidence,
			"write_status":    "candidate",
			"admission_state": item.Status,
		}
		recordTrace(req.Trace, trace.EvtMemoryExtractionMemoryWritten, tracePayload(windowReq, traceFields))
		logExtraction("memory_written", windowReq, traceFields)
	}

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

func (r normalizedRequest) withRange(start int, end int) normalizedRequest {
	r.OffsetStart = start
	r.OffsetEnd = end
	return r
}

func (s Service) candidatesFromMessages(
	ctx context.Context,
	req normalizedRequest,
	window sourceWindow,
	messages []handmsg.Message,
) ([]storage.MemoryItem, []candidateRejection, error) {
	evidence := evidenceFromMessages(window, messages)
	if len(evidence.Lines) == 0 {
		return nil, []candidateRejection{{Kind: "window", Reason: "empty_window"}}, nil
	}
	if s.extractor == nil {
		return nil, nil, errors.New("memory episode extractor is required")
	}
	traceEvidence, err := s.traceEvidence(ctx, req.SessionID)
	if err != nil {
		return nil, nil, err
	}
	evidence.TraceRefs = traceEvidenceRefs(traceEvidence)
	result, err := s.extractor.ExtractCandidates(ctx, CandidateRequest{
		SessionID:   req.SessionID,
		Start:       window.Start,
		End:         window.End,
		Messages:    evidence.Lines,
		TraceEvents: traceEvidence,
		MaxChars:    req.windowCharLimit(),
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]storage.MemoryItem, 0, len(result.Candidates))
	rejections := append([]candidateRejection(nil), result.Rejections...)
	for _, candidate := range result.Candidates {
		item, ok := memoryItemFromCandidate(req, window, evidence, candidate)
		if !ok {
			rejections = append(rejections, candidateRejection{Kind: candidate.Kind, Reason: "empty_candidate"})
			continue
		}
		items = append(items, item)
	}

	if len(items) == 0 && len(rejections) == 0 {
		rejections = append(rejections, candidateRejection{Kind: "window", Reason: "no_curated_candidate"})
	}

	return items, rejections, nil
}

func (s Service) traceEvidence(ctx context.Context, sessionID string) ([]taskTraceEvidence, error) {
	if s.manager == nil {
		return nil, nil
	}

	result, err := s.manager.ListTraceEvents(ctx, storage.TraceQuery{
		SessionID: sessionID,
		Types:     trace.EpisodicMemoryTraceEventTypes(),
		Limit:     defaultMaxTraceEventsPerWindow,
	})
	if err != nil {
		if errors.Is(err, storage.ErrTraceStoreUnsupported) {
			return nil, nil
		}
		return nil, err
	}

	traces := make([]taskTraceEvidence, 0, len(result.Events))
	for _, event := range result.Events {
		if !trace.IsEpisodicMemoryTraceEventType(event.Type) {
			continue
		}
		traces = append(traces, taskTraceEvidence{
			Ref:       traceEventRef(event),
			Type:      strings.TrimSpace(event.Type),
			Timestamp: traceEventTimestamp(event),
			Payload:   tracePayloadText(event.Payload),
		})
	}
	return traces, nil
}

func traceEventRef(event storage.TraceEvent) string {
	if event.Sequence > 0 {
		return "trace:" + strconv.Itoa(event.Sequence)
	}
	if event.ID > 0 {
		return "trace_id:" + strconv.FormatUint(uint64(event.ID), 10)
	}
	return "trace:unknown"
}

func traceEventTimestamp(event storage.TraceEvent) string {
	if event.Timestamp.IsZero() {
		return ""
	}
	return event.Timestamp.UTC().Format(time.RFC3339Nano)
}

func tracePayloadText(payload any) string {
	if payload == nil {
		return ""
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return truncateRunes(string(data), maxTracePayloadChars)
}

func traceEvidenceRefs(events []taskTraceEvidence) []string {
	refs := make([]string, 0, len(events))
	for _, event := range events {
		if ref := strings.TrimSpace(event.Ref); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func evidenceFromMessages(window sourceWindow, messages []handmsg.Message) messageEvidence {
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

	text := strings.Join(lines, "\n")
	return messageEvidence{
		MessageIDs: messageIDs,
		Offsets:    offsets,
		Lines:      lines,
		Text:       text,
		LowerText:  strings.ToLower(text),
	}
}

func memoryItemFromCandidate(
	req normalizedRequest,
	window sourceWindow,
	evidence messageEvidence,
	candidate episodeCandidate,
) (storage.MemoryItem, bool) {
	candidate.Kind = strings.TrimSpace(candidate.Kind)
	if !validCandidateKind(candidate.Kind) {
		return storage.MemoryItem{}, false
	}

	text := strings.TrimSpace(truncateRunes(candidate.Text, req.windowCharLimit()))
	title := strings.TrimSpace(candidate.Title)
	if title == "" && text == "" {
		return storage.MemoryItem{}, false
	}

	metadata := map[string]string{
		"source_session_id": req.SessionID,
		"source_start":      strconv.Itoa(window.Start),
		"source_end":        strconv.Itoa(window.End),
		"trigger":           req.Trigger,
		"candidate_kind":    candidate.Kind,
		"source_quality":    sourceQuality(evidence),
		"usefulness":        usefulness(candidate.Kind),
		"recency":           "source_window",
		"uncertainty":       uncertainty(candidate.Confidence),
	}
	if len(evidence.TraceRefs) > 0 {
		metadata["available_trace_event_refs"] = strings.Join(evidence.TraceRefs, ",")
		metadata["available_trace_event_count"] = strconv.Itoa(len(evidence.TraceRefs))
	}
	for key, value := range candidate.Metadata {
		if strings.TrimSpace(value) != "" {
			metadata[key] = strings.TrimSpace(value)
		}
	}

	sourceLinks := candidate.SourceLinks
	if len(sourceLinks) == 0 {
		sourceLinks = []storage.MemorySourceLink{{
			SessionID:     req.SessionID,
			MessageIDs:    evidence.MessageIDs,
			Offsets:       evidence.Offsets,
			CreatedBy:     req.Trigger,
			CreatedReason: "curated_episodic_memory_extraction",
		}}
	}
	sourceTag := sourceRangeTag(req.SessionID, window.Start, window.End)
	return storage.MemoryItem{
		ID:          candidateMemoryID(req.SessionID, window.Start, window.End, candidate.Kind),
		Kind:        storage.MemoryKindEpisodic,
		Status:      storage.MemoryStatusCandidate,
		Title:       title,
		Text:        text,
		Tags:        []string{"episodic", "curated", candidate.Kind, sourceTag},
		Metadata:    metadata,
		SourceLinks: sourceLinks,
		Confidence:  clampConfidence(candidate.Confidence),
	}, true
}

func validCandidateKind(kind string) bool {
	switch kind {
	case episodeKindDecision,
		episodeKindOutcome,
		episodeKindToolEvent,
		episodeKindBlocker,
		episodeKindUserCorrection:
		return true
	default:
		return false
	}
}

func sourceQuality(evidence messageEvidence) string {
	if len(evidence.MessageIDs) > 0 && len(evidence.Offsets) > 0 {
		return "high"
	}
	return "medium"
}

func usefulness(kind string) string {
	switch kind {
	case episodeKindDecision, episodeKindOutcome, episodeKindUserCorrection:
		return "high"
	case episodeKindToolEvent, episodeKindBlocker:
		return "medium"
	default:
		return "low"
	}
}

func uncertainty(confidence float64) string {
	if confidence >= 0.80 {
		return "low"
	}
	if confidence >= 0.65 {
		return "medium"
	}
	return "high"
}

func clampConfidence(confidence float64) float64 {
	if confidence < 0 {
		return 0
	}
	if confidence > 1 {
		return 1
	}
	return confidence
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

func candidateMemoryIDs(sessionID string, start int, end int) []string {
	kinds := []string{
		episodeKindDecision,
		episodeKindOutcome,
		episodeKindToolEvent,
		episodeKindBlocker,
		episodeKindUserCorrection,
	}
	ids := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		ids = append(ids, candidateMemoryID(sessionID, start, end, kind))
	}
	return ids
}

func candidateMemoryID(sessionID string, start int, end int, kind string) string {
	return "mem_episode_" + sourceRangeHash(strings.TrimSpace(kind)+":"+sessionID, start, end)
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

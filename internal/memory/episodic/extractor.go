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
	defaultMaxTraceEventsPerWindow = 40
	maxTracePayloadChars           = 1200

	episodicSimilarScoreThreshold = 0.75
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
//
// Extraction is source-window oriented. The service does not scan an entire
// transcript at once; it slices a session into bounded windows so model prompts
// stay small, provenance stays precise, and background extraction can checkpoint
// progress after each window.
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
		"plan":              "load_window_collect_trace_evidence_generate_candidates_dedupe_write_candidates",
	}
	recordTrace(req.Trace, trace.EvtMemoryExtractionStarted, tracePayload(normalized, traceField))
	logExtraction("started to extract episodic memory candidates", normalized, traceField)

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

		// Background extraction advances after every successfully processed
		// window. Foreground/tool extraction does not move this checkpoint because
		// callers may intentionally inspect arbitrary historical ranges.
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
	logExtraction("completed episodic memory candidate extraction", normalized, traceFields)

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

	existing, err := s.memory.Search(ctx, storage.MemorySearchQuery{
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate, storage.MemoryStatusActive},
		// Treat the source window as complete once any active/candidate memory exists
		// for it, even if that window produced multiple candidate IDs.
		Tags:  []string{sourceRangeTag(req.SessionID, window.Start, window.End)},
		Limit: 1,
	})
	if err != nil {
		return WindowResult{}, err
	}
	if len(existing.Hits) > 0 {
		// A source-window tag is the first duplicate guard. It prevents a
		// background job from reprocessing a window it has already turned into at
		// least one candidate.
		result.SkipCount = len(existing.Hits)
		for _, hit := range existing.Hits {
			id := strings.TrimSpace(hit.Item.ID)
			if id != "" {
				result.SkippedIDs = append(result.SkippedIDs, id)
			}
		}
		traceFields := map[string]any{"memory_ids": result.SkippedIDs, "checkpoint_state": "complete"}
		recordTrace(req.Trace, trace.EvtMemoryExtractionDuplicateSkipped, tracePayload(windowReq, traceFields))
		logExtraction("skipped completed source window", windowReq, traceFields)
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
	logExtraction("loaded source message window", windowReq, traceFields)

	candidates, rejections, err := s.candidatesFromMessages(ctx, req, window, messages)
	if err != nil {
		return WindowResult{}, err
	}

	candidateCount := len(candidates)
	traceFields = map[string]any{"candidate_count": candidateCount}
	recordTrace(req.Trace, trace.EvtMemoryExtractionExtractorRequested, tracePayload(windowReq, traceFields))
	recordTrace(req.Trace, trace.EvtMemoryExtractionCandidates, tracePayload(windowReq, traceFields))
	traceFields = map[string]any{"message_count": len(messages), "candidate_count": candidateCount}
	logExtraction("received extractor candidate set", windowReq, traceFields)

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
		logExtraction("generated episodic candidate proposal", windowReq, traceFields)
	}

	result.MessageCount = len(messages)
	result.CandidateCount = candidateCount

	if len(candidates) == 0 {
		return result, nil
	}

	for _, candidate := range candidates {
		// The extractor and admission pipeline are not the final duplicate guard.
		// Overlapping windows can still produce the same memory, so we search
		// existing episodic candidate/active memory before writing.
		rejection, err := s.episodicCandidateRejection(ctx, candidate)
		if err != nil {
			return WindowResult{}, err
		}
		if rejection != "" {
			result.SkipCount++
			traceFields = map[string]any{"candidate_kind": candidate.Metadata["candidate_kind"], "rejection_reason": rejection}
			recordTrace(req.Trace, trace.EvtMemoryExtractionCandidateRejected, tracePayload(windowReq, traceFields))
			logExtraction("rejected episodic candidate before write", windowReq, traceFields)
			continue
		}

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
		logExtraction("wrote episodic candidate memory", windowReq, traceFields)
	}

	return result, nil
}

func (s Service) episodicCandidateRejection(ctx context.Context, item storage.MemoryItem) (string, error) {
	text := episodicSearchText(item)
	if text == "" {
		return "", nil
	}

	// This dedupe pass is intentionally before RecordEpisode. It catches
	// duplicate candidates generated from overlapping windows or repeated tool
	// invocations, including candidates whose source-range tag differs.
	result, err := s.memory.Search(ctx, storage.MemorySearchQuery{
		Text:     text,
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate, storage.MemoryStatusActive},
		Limit:    5,
	})
	if err != nil {
		return "", err
	}

	for _, hit := range result.Hits {
		related := hit.Item
		if strings.TrimSpace(related.ID) == strings.TrimSpace(item.ID) {
			continue
		}
		if normalizedEpisodicText(related) == normalizedEpisodicText(item) {
			return "duplicate_episodic_memory", nil
		}
		if sameCandidateKind(related, item) && hit.Score >= episodicSimilarScoreThreshold {
			return "similar_episodic_memory", nil
		}
	}

	return "", nil
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

	// Trace evidence augments the transcript with task-level context such as
	// tool calls and outcomes. It is optional because not every store supports
	// trace queries.
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
	seen := make(map[string]struct{}, len(result.Candidates))
	for _, candidate := range result.Candidates {
		item, ok := memoryItemFromCandidate(req, window, evidence, candidate)
		if !ok {
			rejections = append(rejections, candidateRejection{Kind: candidate.Kind, Reason: "empty_candidate"})
			continue
		}
		if _, ok := seen[item.ID]; ok {
			// Candidate IDs are deterministic over source window, kind, content,
			// metadata, and source links. A repeated ID in one model response means
			// the model produced the same candidate twice.
			rejections = append(rejections, candidateRejection{Kind: candidate.Kind, Reason: "duplicate_candidate"})
			continue
		}

		seen[item.ID] = struct{}{}
		items = append(items, item)
	}
	items, rejections = admitCandidateItems(items, rejections)

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

func episodicSearchText(item storage.MemoryItem) string {
	text := strings.TrimSpace(item.Text)
	if text == "" {
		text = strings.TrimSpace(item.Title)
	}
	if len([]rune(text)) > 240 {
		text = string([]rune(text)[:240])
	}
	return text
}

func normalizedEpisodicText(item storage.MemoryItem) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(item.Title+"\n"+item.Text))), " ")
}

func sameCandidateKind(a storage.MemoryItem, b storage.MemoryItem) bool {
	return strings.TrimSpace(a.Metadata["candidate_kind"]) == strings.TrimSpace(b.Metadata["candidate_kind"])
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

	// Model metadata is allowed to enrich provider metadata but not remove
	// provenance. Empty values are ignored so storage stays compact and searchable.
	if len(evidence.TraceRefs) > 0 {
		metadata["available_trace_event_count"] = strconv.Itoa(len(evidence.TraceRefs))
	}
	for key, value := range candidate.Metadata {
		if value := normalizedMetadataValue(value); value != "" {
			metadata[key] = value
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
	// The stable ID gives repeated extraction of the same evidence the same
	// candidate identity, while source tags let background jobs skip completed
	// windows cheaply.
	return storage.MemoryItem{
		ID:          candidateMemoryID(req, window, candidate.Kind, title, text, metadata, sourceLinks),
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

func normalizedMetadataValue(value string) string {
	return strings.TrimSpace(value)
}

func sourceQuality(evidence messageEvidence) string {
	if len(evidence.MessageIDs) > 0 && len(evidence.Offsets) > 0 {
		return "high"
	}
	return "medium"
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

func candidateMemoryID(
	req normalizedRequest,
	window sourceWindow,
	kind string,
	title string,
	text string,
	metadata map[string]string,
	sourceLinks []storage.MemorySourceLink,
) string {
	return episodicMemoryIDPrefix() + sourceRangeHash(
		candidateMemoryIDSource(req.SessionID, kind, title, text, metadata, sourceLinks),
		window.Start,
		window.End,
	)
}

func episodicMemoryIDPrefix() string {
	return "mem_" + string(storage.MemoryKindEpisodic) + "_"
}

func candidateMemoryIDSource(
	sessionID string,
	kind string,
	title string,
	text string,
	metadata map[string]string,
	sourceLinks []storage.MemorySourceLink,
) string {
	parts := []string{
		strings.TrimSpace(sessionID),
		strings.TrimSpace(kind),
		normalizeMemoryIDText(title),
		normalizeMemoryIDText(text),
	}
	for _, key := range episodeIdentityMetadataKeys() {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			parts = append(parts, key+"="+normalizeMemoryIDText(value))
		}
	}
	for _, link := range sourceLinks {
		parts = append(parts, strings.TrimSpace(link.SessionID))
		parts = append(parts, uintSliceMemoryIDText(link.MessageIDs))
		parts = append(parts, intSliceMemoryIDText(link.Offsets))
	}

	return strings.Join(parts, "\n")
}

func normalizeMemoryIDText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func uintSliceMemoryIDText(values []uint) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatUint(uint64(value), 10))
	}

	return strings.Join(parts, ",")
}

func intSliceMemoryIDText(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}

	return strings.Join(parts, ",")
}

func sourceRangeTag(sessionID string, start int, end int) string {
	return "source-range-" + sourceRangeHash(sessionID, start, end)
}

func sourceRangeHash(id string, start int, end int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s:%d:%d", strings.TrimSpace(id), start, end))
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

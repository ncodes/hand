package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/agent/compaction"
	"github.com/wandxy/hand/internal/config"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
	"github.com/wandxy/hand/internal/trace"
)

const RecentSessionTail = 8

type SummaryState struct {
	SessionID          string
	SourceEndOffset    int
	SourceMessageCount int
	UpdatedAt          time.Time
	SessionSummary     string
	CurrentTask        string
	Discoveries        []string
	OpenQuestions      []string
	NextActions        []string
}

type summaryPayload struct {
	SessionSummary string   `json:"session_summary"`
	CurrentTask    string   `json:"current_task"`
	Discoveries    []string `json:"discoveries"`
	OpenQuestions  []string `json:"open_questions"`
	NextActions    []string `json:"next_actions"`
}

var summaryStructuredOutput = &models.StructuredOutput{
	Name:        "session_summary",
	Description: "Structured handoff summary for compacted conversation history.",
	Strict:      true,
	Schema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"session_summary": map[string]any{"type": "string"},
			"current_task":    map[string]any{"type": "string"},
			"discoveries": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"open_questions": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"next_actions": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{
			"session_summary",
			"current_task",
			"discoveries",
			"open_questions",
			"next_actions",
		},
	},
}

type refreshPlan struct {
	RequestedAt        time.Time
	TargetMessageCount int
	TargetOffset       int
}

func SummaryFromStorage(summary storage.SessionSummary) *SummaryState {
	if summary.SessionID == "" || summary.SessionSummary == "" {
		return nil
	}

	cloned := common.CloneSessionSummary(summary)
	return &SummaryState{
		SessionID:          cloned.SessionID,
		SourceEndOffset:    cloned.SourceEndOffset,
		SourceMessageCount: cloned.SourceMessageCount,
		UpdatedAt:          cloned.UpdatedAt,
		SessionSummary:     cloned.SessionSummary,
		CurrentTask:        cloned.CurrentTask,
		Discoveries:        cloned.Discoveries,
		OpenQuestions:      cloned.OpenQuestions,
		NextActions:        cloned.NextActions,
	}
}

func (s *Service) MaybeRefreshMemory(ctx context.Context, memory *Memory, input RefreshInput) error {
	return s.refreshMemory(ctx, memory, input, false)
}

func (s *Service) refreshMemory(ctx context.Context, memory *Memory, input RefreshInput, force bool) error {
	if memory == nil || input.TraceSession == nil {
		return nil
	}

	if s == nil {
		return errors.New("memory service is required")
	}

	if s.modelClient == nil {
		return errors.New("model client is required")
	}

	if s.store == nil {
		return errors.New("summary store is required")
	}

	if !force && !s.compactionOn {
		return nil
	}

	totalCount, err := s.store.CountMessages(ctx, input.SessionID, storage.MessageQueryOptions{})
	if err != nil {
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(
			input.SessionID,
			storage.SessionCompaction{Status: storage.CompactionStatusFailed},
			err.Error()),
		)
		return err
	}

	if totalCount <= RecentSessionTail {
		return nil
	}

	targetOffset := totalCount - RecentSessionTail

	plan := refreshPlan{
		RequestedAt:        s.currentTime(),
		TargetMessageCount: totalCount,
		TargetOffset:       targetOffset,
	}

	existingSummaryEndOffset := 0
	if memory.Summary != nil && memory.Summary.SourceEndOffset > existingSummaryEndOffset {
		existingSummaryEndOffset = memory.Summary.SourceEndOffset
	}

	if !force && memory.Summary != nil && memory.Summary.SourceEndOffset >= targetOffset {
		session, ok, err := s.store.Get(ctx, input.SessionID)
		if err != nil {
			input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
				Status:             storage.CompactionStatusFailed,
				TargetMessageCount: totalCount,
				TargetOffset:       targetOffset,
			}, err.Error()))
			return err
		}
		if !ok {
			err = errors.New("session not found")
			input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
				Status:             storage.CompactionStatusFailed,
				TargetMessageCount: totalCount,
				TargetOffset:       targetOffset,
			}, err.Error()))
			return err
		}

		return s.reconcileCompactionSucceeded(ctx, &session, plan, input.TraceSession)
	}

	if !force {
		estimate := s.evaluator.Evaluate(input.Request, input.LastPromptTokens)
		if !estimate.Triggered() {
			return nil
		}
	}

	session, ok, err := s.store.Get(ctx, input.SessionID)
	if err != nil {
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: totalCount,
			TargetOffset:       targetOffset,
		}, err.Error()))
		return err
	}
	if !ok {
		err = errors.New("session not found")
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: totalCount,
			TargetOffset:       targetOffset,
		}, err.Error()))
		return err
	}

	log.Info().
		Str("session_id", input.SessionID).
		Str("trigger_source", compactionTriggerSource(force)).
		Int("existing_summary_end_offset", existingSummaryEndOffset).
		Int("messages_to_summarize", max(plan.TargetOffset-existingSummaryEndOffset, 0)).
		Int("tail_messages_retained", RecentSessionTail).
		Int("target_offset", plan.TargetOffset).
		Int("total_messages", plan.TargetMessageCount).
		Msg("compaction plan created")

	if err := s.transitionCompactionPending(ctx, &session, plan, input.TraceSession); err != nil {
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: plan.TargetMessageCount,
			TargetOffset:       plan.TargetOffset,
		}, err.Error()))
		return err
	}

	if err := s.transitionCompactionRunning(ctx, &session, plan, input.TraceSession); err != nil {
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
			RequestedAt:        plan.RequestedAt,
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: plan.TargetMessageCount,
			TargetOffset:       plan.TargetOffset,
		}, err.Error()))
		return err
	}

	if err := s.refreshSummary(ctx, memory, input, plan); err != nil {
		if transErr := s.transitionCompactionFailed(ctx, &session, plan, err, input.TraceSession); transErr != nil {
			wrapped := fmt.Errorf("mark compaction failed: %w", transErr)
			input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
				RequestedAt:        plan.RequestedAt,
				StartedAt:          session.Compaction.StartedAt,
				Status:             storage.CompactionStatusFailed,
				TargetMessageCount: plan.TargetMessageCount,
				TargetOffset:       plan.TargetOffset,
			}, wrapped.Error()))
			return wrapped
		}
		return err
	}

	if err := s.transitionCompactionSucceeded(ctx, &session, plan, input.TraceSession); err != nil {
		input.TraceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(input.SessionID, storage.SessionCompaction{
			RequestedAt:        plan.RequestedAt,
			StartedAt:          session.Compaction.StartedAt,
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: plan.TargetMessageCount,
			TargetOffset:       plan.TargetOffset,
		}, err.Error()))
		return err
	}

	return nil
}

func (s *Service) CompactSession(
	ctx context.Context,
	session storage.Session,
	traceSession traceRecorder,
) (*SummaryState, error) {
	if s == nil {
		return nil, errors.New("memory service is required")
	}

	if s.modelClient == nil {
		return nil, errors.New("model client is required")
	}

	if s.store == nil {
		return nil, errors.New("summary store is required")
	}

	if traceSession == nil {
		return nil, errors.New("trace session is required")
	}

	memory, err := s.Load(ctx, session.ID)
	if err != nil {
		return nil, err
	}

	totalCount, err := s.store.CountMessages(ctx, session.ID, storage.MessageQueryOptions{})
	if err != nil {
		traceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(
			session.ID,
			storage.SessionCompaction{Status: storage.CompactionStatusFailed},
			err.Error()),
		)
		return nil, err
	}

	if totalCount <= RecentSessionTail {
		err = errors.New("session history is too short to compact")
		traceSession.Record(trace.EvtContextCompactionFailed, compactionTracePayload(
			session.ID,
			storage.SessionCompaction{Status: storage.CompactionStatusFailed},
			err.Error()),
		)
		return nil, err
	}

	if err := s.refreshMemory(ctx, memory, RefreshInput{
		LastPromptTokens: session.LastPromptTokens,
		SessionID:        session.ID,
		TraceSession:     traceSession,
	}, true); err != nil {
		return nil, err
	}

	return memory.Summary, nil
}

func (s *Service) refreshSummary(ctx context.Context, memory *Memory, input RefreshInput, plan refreshPlan) error {
	payload := summaryTracePayload(input.SessionID, plan.TargetOffset, plan.TargetMessageCount, plan.RequestedAt)
	input.TraceSession.Record(trace.EvtSummaryRequested, payload)

	summaryMessages := make([]handmsg.Message, 0, plan.TargetOffset)
	instructions := instruct.BuildSessionSummary()
	if summaryInstructions, ok := memory.RenderSummaryInstructions(); ok {
		instructions = instruct.New(summaryInstructions).Append(instructions...)
	}

	startOffset := 0
	if memory.Summary != nil && memory.Summary.SourceEndOffset > startOffset {
		startOffset = memory.Summary.SourceEndOffset
	}

	limit := plan.TargetOffset - startOffset
	if limit > 0 {
		messages, err := s.store.GetMessages(ctx, input.SessionID, storage.MessageQueryOptions{
			Limit:  limit,
			Offset: startOffset,
		})
		if err != nil {
			failedPayload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
			input.TraceSession.Record(trace.EvtSummaryFailed, failedPayload)
			return err
		}
		summaryMessages = append(summaryMessages, handmsg.CloneMessages(messages)...)
	}

	log.Debug().
		Str("session_id", input.SessionID).
		Int("start_offset", startOffset).
		Int("end_offset", plan.TargetOffset).
		Int("existing_summary_end_offset", startOffset).
		Int("messages_to_summarize", limit).
		Int("summary_messages", len(summaryMessages)).
		Msg("generating compaction summary")

	request := models.Request{
		Model:            s.summaryModel,
		APIMode:          s.apiMode,
		Instructions:     instructions.String(),
		Messages:         summaryMessages,
		Tools:            nil,
		StructuredOutput: summaryStructuredOutput,
		DebugRequests:    s.debugRequests,
	}
	resp, err := s.generateSummaryResponse(ctx, request)
	if err != nil {
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record(trace.EvtSummaryFailed, payload)
		return err
	}

	if resp == nil {
		err = errors.New("model response is required")
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record(trace.EvtSummaryFailed, payload)
		return err
	}

	if resp.RequiresToolCalls {
		err = errors.New("summary requested tool calls")
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record(trace.EvtSummaryFailed, payload)
		return err
	}

	summaryParsePath := "json"
	summary, err := parseSummary(
		input.SessionID,
		plan.TargetOffset,
		plan.TargetMessageCount,
		resp.OutputText,
		plan.RequestedAt,
	)
	if err != nil {
		if errors.Is(err, errSummaryResponseEmpty) {
			payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
			input.TraceSession.Record(trace.EvtSummaryFailed, payload)
			return err
		}

		summaryParsePath = "plain_text_fallback"
		log.Warn().Str("session_id", input.SessionID).Err(err).Msg("structured summary parse failed, using fallback")

		input.TraceSession.Record(trace.EvtSummaryParseFailed, mergeSummaryTracePayload(payload, map[string]any{
			"error": err.Error(),
		}))

		summary, err = fallbackSummary(
			input.SessionID,
			plan.TargetOffset,
			plan.TargetMessageCount,
			resp.OutputText,
			plan.RequestedAt,
		)
		if err != nil {
			payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
			input.TraceSession.Record(trace.EvtSummaryFailed, payload)
			return err
		}
	}

	summaryRecord := common.CloneSessionSummary(storage.SessionSummary{
		SessionID:          summary.SessionID,
		SourceEndOffset:    summary.SourceEndOffset,
		SourceMessageCount: summary.SourceMessageCount,
		UpdatedAt:          summary.UpdatedAt,
		SessionSummary:     summary.SessionSummary,
		CurrentTask:        summary.CurrentTask,
		Discoveries:        summary.Discoveries,
		OpenQuestions:      summary.OpenQuestions,
		NextActions:        summary.NextActions,
	})

	if err := s.store.SaveSummary(ctx, summaryRecord); err != nil {
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record(trace.EvtSummaryFailed, payload)
		return err
	}

	memory.Summary = summary

	log.Info().
		Str("session_id", memory.Summary.SessionID).
		Str("summary_parse_path", summaryParsePath).
		Int("summarized_from_offset", startOffset).
		Int("source_end_offset", memory.Summary.SourceEndOffset).
		Int("source_message_count", memory.Summary.SourceMessageCount).
		Int("messages_summarized", max(memory.Summary.SourceEndOffset-startOffset, 0)).
		Int("tail_messages_retained", max(memory.Summary.SourceMessageCount-memory.Summary.SourceEndOffset, 0)).
		Msg("compaction summary saved")

	input.TraceSession.Record(trace.EvtSummarySaved, summaryTracePayload(
		memory.Summary.SessionID,
		memory.Summary.SourceEndOffset,
		memory.Summary.SourceMessageCount,
		memory.Summary.UpdatedAt,
	))

	return nil
}

func (s *Service) generateSummaryResponse(ctx context.Context, request models.Request) (*models.Response, error) {
	if s == nil || s.summaryClient == nil {
		return nil, errors.New("model client is required")
	}

	resp, err := s.summaryClient.Complete(ctx, request)
	if err == nil {
		memLog.Info().
			Bool("structured_output_request", request.StructuredOutput != nil).
			Msg("compaction summary model request completed")
		return resp, nil
	}

	if request.StructuredOutput == nil {
		return nil, err
	}

	log.Warn().Err(err).Msg("structured summary request failed, retrying without structured output")

	fallback := request
	fallback.StructuredOutput = nil
	resp, err = s.summaryClient.Complete(ctx, fallback)
	if err != nil {
		return nil, err
	}

	memLog.Info().
		Bool("structured_output_request", false).
		Msg("compaction summary model request completed after unstructured retry")

	return resp, nil
}

func (s *Service) transitionCompactionPending(
	ctx context.Context,
	session *storage.Session,
	plan refreshPlan,
	recorder traceRecorder,
) error {
	if session == nil {
		return errors.New("session is required")
	}

	session.Compaction = storage.SessionCompaction{
		RequestedAt:        plan.RequestedAt,
		Status:             storage.CompactionStatusPending,
		TargetMessageCount: plan.TargetMessageCount,
		TargetOffset:       plan.TargetOffset,
	}

	if err := s.store.Save(ctx, *session); err != nil {
		return err
	}

	recorder.Record(trace.EvtContextCompactionPending, compactionTracePayload(session.ID, session.Compaction, ""))
	return nil
}

func (s *Service) transitionCompactionRunning(
	ctx context.Context,
	session *storage.Session,
	plan refreshPlan,
	recorder traceRecorder,
) error {
	if session == nil {
		return errors.New("session is required")
	}

	session.Compaction.StartedAt = s.currentTime()
	session.Compaction.Status = storage.CompactionStatusRunning
	session.Compaction.TargetMessageCount = plan.TargetMessageCount
	session.Compaction.TargetOffset = plan.TargetOffset

	if err := s.store.Save(ctx, *session); err != nil {
		return err
	}

	recorder.Record(trace.EvtContextCompactionRunning, compactionTracePayload(session.ID, session.Compaction, ""))
	log.Debug().
		Str("session_id", session.ID).
		Int("target_offset", plan.TargetOffset).
		Int("target_message_count", plan.TargetMessageCount).
		Msg("compaction running")
	return nil
}

func (s *Service) transitionCompactionSucceeded(
	ctx context.Context,
	session *storage.Session,
	plan refreshPlan,
	recorder traceRecorder,
) error {
	if session == nil {
		return errors.New("session is required")
	}

	session.Compaction.CompletedAt = s.currentTime()
	session.Compaction.FailedAt = time.Time{}
	session.Compaction.LastError = ""
	session.Compaction.Status = storage.CompactionStatusSucceeded
	session.Compaction.TargetMessageCount = plan.TargetMessageCount
	session.Compaction.TargetOffset = plan.TargetOffset

	if err := s.store.Save(ctx, *session); err != nil {
		return err
	}

	recorder.Record(trace.EvtContextCompactionSucceeded, compactionTracePayload(session.ID, session.Compaction, ""))
	log.Info().
		Str("session_id", session.ID).
		Int("target_offset", plan.TargetOffset).
		Int("target_message_count", plan.TargetMessageCount).
		Msg("compaction completed")
	return nil
}

func (s *Service) reconcileCompactionSucceeded(
	ctx context.Context,
	session *storage.Session,
	plan refreshPlan,
	recorder traceRecorder,
) error {
	if session == nil {
		return errors.New("session is required")
	}

	if session.Compaction.Status == storage.CompactionStatusSucceeded &&
		session.Compaction.TargetOffset >= plan.TargetOffset &&
		session.Compaction.TargetMessageCount >= plan.TargetMessageCount {
		return nil
	}

	if err := s.transitionCompactionSucceeded(ctx, session, plan, recorder); err != nil {
		recorder.Record(trace.EvtContextCompactionFailed, compactionTracePayload(session.ID, storage.SessionCompaction{
			RequestedAt:        session.Compaction.RequestedAt,
			StartedAt:          session.Compaction.StartedAt,
			Status:             storage.CompactionStatusFailed,
			TargetMessageCount: plan.TargetMessageCount,
			TargetOffset:       plan.TargetOffset,
		}, err.Error()))
		return err
	}

	return nil
}

func (s *Service) transitionCompactionFailed(
	ctx context.Context,
	session *storage.Session,
	plan refreshPlan,
	cause error,
	recorder traceRecorder,
) error {
	if session == nil {
		return errors.New("session is required")
	}

	session.Compaction.CompletedAt = time.Time{}
	session.Compaction.FailedAt = s.currentTime()
	session.Compaction.LastError = strings.TrimSpace(cause.Error())
	session.Compaction.Status = storage.CompactionStatusFailed
	session.Compaction.TargetMessageCount = plan.TargetMessageCount
	session.Compaction.TargetOffset = plan.TargetOffset

	if err := s.store.Save(ctx, *session); err != nil {
		return err
	}

	log.Error().Str("session_id", session.ID).Str("cause", session.Compaction.LastError).Msg("compaction failed")

	recorder.Record(trace.EvtContextCompactionFailed, compactionTracePayload(
		session.ID,
		session.Compaction,
		session.Compaction.LastError,
	))
	log.Warn().
		Str("session_id", session.ID).
		Int("target_offset", plan.TargetOffset).
		Int("target_message_count", plan.TargetMessageCount).
		Str("error", session.Compaction.LastError).
		Msg("compaction failed")

	return nil
}

func compactionTriggerSource(force bool) string {
	if force {
		return "manual"
	}

	return "preflight_threshold_exceeded"
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		now := s.now()
		if !now.IsZero() {
			return now.UTC()
		}
	}

	return time.Now().UTC()
}

func (m *Memory) RecordSummaryApplied(traceSession trace.Session) {
	if m == nil || traceSession == nil || m.Summary == nil {
		return
	}

	if strings.TrimSpace(m.Summary.SessionSummary) == "" {
		return
	}

	traceSession.Record(trace.EvtSummaryApplied, summaryTracePayload(
		m.Summary.SessionID,
		m.Summary.SourceEndOffset,
		m.Summary.SourceMessageCount,
		m.Summary.UpdatedAt,
	),
	)
}

func parseSummary(
	sessionID string,
	sourceEndOffset,
	sourceMessageCount int,
	raw string,
	updatedAt time.Time,
) (*SummaryState, error) {
	raw = normalizedSummaryText(raw)
	if raw == "" {
		return nil, errSummaryResponseEmpty
	}

	var payload summaryPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	summary := SummaryFromStorage(storage.SessionSummary{
		SessionID:          sessionID,
		SourceEndOffset:    sourceEndOffset,
		SourceMessageCount: sourceMessageCount,
		UpdatedAt:          updatedAt,
		SessionSummary:     payload.SessionSummary,
		CurrentTask:        payload.CurrentTask,
		Discoveries:        payload.Discoveries,
		OpenQuestions:      payload.OpenQuestions,
		NextActions:        payload.NextActions,
	})
	if summary == nil {
		return nil, errors.New("session summary is required")
	}

	return summary, nil
}

var errSummaryResponseEmpty = errors.New("summary response is empty")

func fallbackSummary(
	sessionID string,
	sourceEndOffset,
	sourceMessageCount int,
	raw string,
	updatedAt time.Time,
) (*SummaryState, error) {
	raw = normalizedSummaryText(raw)
	if raw == "" {
		return nil, errSummaryResponseEmpty
	}

	summary := SummaryFromStorage(storage.SessionSummary{
		SessionID:          sessionID,
		SourceEndOffset:    sourceEndOffset,
		SourceMessageCount: sourceMessageCount,
		UpdatedAt:          updatedAt,
		SessionSummary:     raw,
	})
	if summary == nil {
		return nil, errors.New("session summary is required")
	}

	return summary, nil
}

func normalizedSummaryText(raw string) string {
	return strings.TrimSpace(stripMarkdownFence(raw))
}

func stripMarkdownFence(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}

	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	return strings.TrimSpace(raw)
}

func summaryTracePayload(
	sessionID string,
	sourceEndOffset,
	sourceMessageCount int,
	updatedAt time.Time,
) map[string]any {
	return map[string]any{
		"session_id":           sessionID,
		"source_end_offset":    sourceEndOffset,
		"source_message_count": sourceMessageCount,
		"updated_at":           updatedAt,
	}
}

func mergeSummaryTracePayload(base map[string]any, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	maps.Copy(merged, base)
	maps.Copy(merged, extra)
	return merged
}

func compactionTracePayload(sessionID string, state storage.SessionCompaction, failure string) map[string]any {
	payload := map[string]any{
		"session_id":           sessionID,
		"status":               state.Status,
		"target_message_count": state.TargetMessageCount,
		"target_offset":        state.TargetOffset,
	}
	if !state.RequestedAt.IsZero() {
		payload["requested_at"] = state.RequestedAt
	}
	if !state.StartedAt.IsZero() {
		payload["started_at"] = state.StartedAt
	}
	if !state.CompletedAt.IsZero() {
		payload["completed_at"] = state.CompletedAt
	}
	if !state.FailedAt.IsZero() {
		payload["failed_at"] = state.FailedAt
	}
	if strings.TrimSpace(failure) != "" {
		payload["error"] = strings.TrimSpace(failure)
	}

	return payload
}

func renderSummaryList(title string, values []string) string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		lines = append(lines, "- "+value)
	}

	if len(lines) == 0 {
		return ""
	}

	return title + ":\n" + strings.Join(lines, "\n")
}

func summaryCompactionEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.CompactionEnabled == nil {
		return true
	}

	return *cfg.CompactionEnabled
}

func summaryCompactionEvaluator(cfg *config.Config) *compaction.Evaluator {
	if cfg == nil {
		return compaction.NewEvaluator(0, 0, 0)
	}

	return compaction.NewEvaluator(
		cfg.ContextLength,
		cfg.CompactionTriggerPercent,
		cfg.CompactionWarnPercent,
	)
}

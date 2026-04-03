package memory

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"time"

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
	if memory == nil || input.TraceSession == nil {
		return nil
	}

	if s == nil {
		return errors.New("memory service is required")
	}

	if s.modelClient == nil {
		return errors.New("model client is required")
	}

	if s.summaryStore == nil {
		return errors.New("summary store is required")
	}

	if !s.compactionOn {
		return nil
	}

	if len(input.SessionHistory) <= RecentSessionTail {
		return nil
	}

	estimate := s.evaluator.Evaluate(input.Request, input.LastPromptTokens)
	if !estimate.Triggered() {
		return nil
	}

	sourceEndOffset := len(input.SessionHistory) - RecentSessionTail
	if memory.Summary != nil && memory.Summary.SourceEndOffset >= sourceEndOffset {
		return nil
	}

	requestedAt := time.Now().UTC()
	if s.now != nil {
		requestedAt = s.now()
		if requestedAt.IsZero() {
			requestedAt = time.Now().UTC()
		} else {
			requestedAt = requestedAt.UTC()
		}
	}

	payload := summaryTracePayload(input.SessionID, sourceEndOffset, len(input.SessionHistory), requestedAt)
	input.TraceSession.Record("context.summary.requested", payload)

	summaryMessages := make([]handmsg.Message, 0, sourceEndOffset+1)

	// Include the existing summary message if it exists.
	if summaryMessage, ok := memory.RenderSummaryMessage(); ok {
		summaryMessages = append(summaryMessages, summaryMessage)
	}

	// Skip messages already covered by the existing summary.
	startOffset := 0
	if memory.Summary != nil && memory.Summary.SourceEndOffset > startOffset {
		startOffset = memory.Summary.SourceEndOffset
	}

	summaryMessages = append(
		summaryMessages,
		handmsg.CloneMessages(input.SessionHistory[startOffset:sourceEndOffset])...,
	)

	resp, err := s.modelClient.Chat(ctx, models.Request{
		Model:         s.model,
		APIMode:       s.apiMode,
		Instructions:  instruct.BuildSessionSummary().String(),
		Messages:      summaryMessages,
		Tools:         nil,
		DebugRequests: s.debugRequests,
	})
	if err != nil {
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record("context.summary.failed", payload)
		return err
	}

	if resp == nil {
		err = errors.New("model response is required")
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record("context.summary.failed", payload)
		return err
	}

	if resp.RequiresToolCalls {
		err = errors.New("summary requested tool calls")
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record("context.summary.failed", payload)
		return err
	}

	summary, err := parseSummary(
		input.SessionID,
		sourceEndOffset,
		len(input.SessionHistory),
		resp.OutputText,
		requestedAt,
	)
	if err != nil {
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record("context.summary.failed", payload)
		return err
	}

	memory.Summary = summary
	if err := s.summaryStore.SaveSummary(ctx, memory.SummaryToStorage()); err != nil {
		payload := mergeSummaryTracePayload(payload, map[string]any{"error": err.Error()})
		input.TraceSession.Record("context.summary.failed", payload)
		return err
	}

	payload = summaryTracePayload(
		memory.Summary.SessionID,
		memory.Summary.SourceEndOffset,
		memory.Summary.SourceMessageCount,
		memory.Summary.UpdatedAt,
	)
	input.TraceSession.Record("context.summary.saved", payload)
	return nil
}

func (m *Memory) RecordSummaryApplied(traceSession trace.Session) {
	if m == nil || traceSession == nil || m.Summary == nil {
		return
	}

	if strings.TrimSpace(m.Summary.SessionSummary) == "" {
		return
	}

	traceSession.Record(
		"context.summary.applied",
		summaryTracePayload(
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
	raw = strings.TrimSpace(stripMarkdownFence(raw))
	if raw == "" {
		return nil, errors.New("summary response is empty")
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

func summaryTracePayload(sessionID string, sourceEndOffset, sourceMessageCount int, updatedAt time.Time) map[string]any {
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
		cfg.ModelContextLength,
		cfg.CompactionTriggerPercent,
		cfg.CompactionWarnPercent,
	)
}

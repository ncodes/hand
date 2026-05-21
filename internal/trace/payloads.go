package trace

import (
	"encoding/json"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

type SessionFailedPayload struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type SafetyEventPayload struct {
	SessionID     string              `json:"session_id,omitempty"`
	Source        string              `json:"source,omitempty"`
	Action        string              `json:"action,omitempty"`
	ContentLength int                 `json:"content_length,omitempty"`
	Blocked       bool                `json:"blocked,omitempty"`
	Redacted      bool                `json:"redacted,omitempty"`
	Refusal       string              `json:"refusal,omitempty"`
	Findings      []map[string]string `json:"findings,omitempty"`
}

type UserMessageAcceptedPayload struct {
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`
}

type ModelReasoningCompletedPayload struct {
	DurationMS int64 `json:"duration_ms,omitempty"`
}

type FinalAssistantResponsePayload struct {
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`
}

type SummaryFallbackStartedPayload struct {
	RemainingIterations int `json:"remaining_iterations,omitempty"`
}

type ContextEventPayload struct {
	Source           string `json:"source,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	ContextLimit     int    `json:"context_limit,omitempty"`
	TriggerThreshold int    `json:"trigger_threshold,omitempty"`
	WarnThreshold    int    `json:"warn_threshold,omitempty"`
}

type SummaryEventPayload struct {
	SessionID          string    `json:"session_id,omitempty"`
	SourceEndOffset    int       `json:"source_end_offset,omitempty"`
	SourceMessageCount int       `json:"source_message_count,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
	Error              string    `json:"error,omitempty"`
}

type CompactionEventPayload struct {
	SessionID          string    `json:"session_id,omitempty"`
	Status             string    `json:"status,omitempty"`
	TargetMessageCount int       `json:"target_message_count,omitempty"`
	TargetOffset       int       `json:"target_offset,omitempty"`
	RequestedAt        time.Time `json:"requested_at,omitempty"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	FailedAt           time.Time `json:"failed_at,omitempty"`
	Error              string    `json:"error,omitempty"`
}

type WorkspaceRulesTruncatedPayload struct {
	OriginalLength   int    `json:"original_length,omitempty"`
	TruncatedLength  int    `json:"truncated_length,omitempty"`
	MaxContentLength int    `json:"max_content_length,omitempty"`
	Marker           string `json:"marker,omitempty"`
}

type PlanEventPayload struct {
	SessionID    string             `json:"session_id,omitempty"`
	Steps        []PlanStepPayload  `json:"steps,omitempty"`
	Summary      PlanSummaryPayload `json:"summary,omitempty"`
	ActiveStepID string             `json:"active_step_id,omitempty"`
	Explanation  string             `json:"explanation,omitempty"`
	Source       string             `json:"source,omitempty"`
}

type PlanStepPayload struct {
	ID      string `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

type PlanSummaryPayload struct {
	Total      int `json:"total,omitempty"`
	Pending    int `json:"pending,omitempty"`
	InProgress int `json:"in_progress,omitempty"`
	Completed  int `json:"completed,omitempty"`
	Cancelled  int `json:"cancelled,omitempty"`
}

type MemoryEventPayload struct {
	SessionID                string    `json:"session_id,omitempty"`
	MemoryID                 string    `json:"memory_id,omitempty"`
	ItemID                   string    `json:"item_id,omitempty"`
	Source                   string    `json:"source,omitempty"`
	Status                   string    `json:"status,omitempty"`
	Kind                     string    `json:"kind,omitempty"`
	Reason                   string    `json:"reason,omitempty"`
	Error                    string    `json:"error,omitempty"`
	Operation                string    `json:"operation,omitempty"`
	TriggerReason            string    `json:"trigger_reason,omitempty"`
	BackgroundRunID          string    `json:"background_run_id,omitempty"`
	CheckpointID             string    `json:"checkpoint_id,omitempty"`
	SummaryID                string    `json:"summary_id,omitempty"`
	Title                    string    `json:"title,omitempty"`
	Text                     string    `json:"text,omitempty"`
	ResultCount              int       `json:"result_count,omitempty"`
	CandidateCount           int       `json:"candidate_count,omitempty"`
	Limit                    int       `json:"limit,omitempty"`
	MessageCount             int       `json:"message_count,omitempty"`
	WindowIndex              int       `json:"window_index,omitempty"`
	WindowSize               int       `json:"window_size,omitempty"`
	WindowCount              int       `json:"window_count,omitempty"`
	WindowStartOffset        int       `json:"window_start_offset,omitempty"`
	WindowEndOffset          int       `json:"window_end_offset,omitempty"`
	SourceEndOffset          int       `json:"source_end_offset,omitempty"`
	SourceMessageCount       int       `json:"source_message_count,omitempty"`
	EpisodicCheckpointOffset int       `json:"episodic_checkpoint_offset,omitempty"`
	Attempt                  int       `json:"attempt,omitempty"`
	RetryCount               int       `json:"retry_count,omitempty"`
	WriteCount               int       `json:"write_count,omitempty"`
	SkipCount                int       `json:"skip_count,omitempty"`
	FailureCount             int       `json:"failure_count,omitempty"`
	DurationMS               int64     `json:"duration_ms,omitempty"`
	Confidence               float64   `json:"confidence,omitempty"`
	Eligible                 *bool     `json:"eligible,omitempty"`
	StartedAt                time.Time `json:"started_at,omitempty"`
	CompletedAt              time.Time `json:"completed_at,omitempty"`
}

type PlanToolOperation string

const (
	PlanToolOperationRead           PlanToolOperation = "read"
	PlanToolOperationUpdate         PlanToolOperation = "update"
	PlanToolOperationClearCompleted PlanToolOperation = "clear_completed"
)

type PlanToolState struct {
	Operation      PlanToolOperation `json:"operation,omitempty"`
	ChangedCount   int               `json:"changed_count,omitempty"`
	TotalCount     int               `json:"total_count,omitempty"`
	CompletedCount int               `json:"completed_count,omitempty"`
}

type ToolInvocationStartedPayload struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     string         `json:"input,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	PlanState *PlanToolState `json:"plan_state,omitempty"`
}

type ToolInvocationCompletedPayload struct {
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Content    string         `json:"content,omitempty"`
	Detail     string         `json:"detail,omitempty"`
	PlanState  *PlanToolState `json:"plan_state,omitempty"`
}

func DecodePayload(eventType string, payload any) (any, bool) {
	switch strings.TrimSpace(eventType) {
	case EvtChatStarted:
		return decodePayloadAs[Metadata](payload)
	case EvtSessionFailed:
		return decodePayloadAs[SessionFailedPayload](payload)
	case EvtInputSafetyBlocked,
		EvtOutputSafetyApplied,
		EvtToolOutputSafetyApplied,
		EvtLoadedContentSafetyBlocked,
		EvtMemorySafetyBlocked:
		return decodePayloadAs[SafetyEventPayload](payload)
	case EvtUserMessageAccepted:
		return decodePayloadAs[UserMessageAcceptedPayload](payload)
	case EvtModelRequest:
		return decodePayloadAs[models.Request](payload)
	case EvtModelResponse:
		return decodePayloadAs[models.Response](payload)
	case EvtModelReasoningCompleted:
		return decodePayloadAs[ModelReasoningCompletedPayload](payload)
	case EvtFinalAssistantResponse:
		return decodePayloadAs[FinalAssistantResponsePayload](payload)
	case EvtToolInvocationStarted:
		return ToolInvocationStartedPayloadFrom(payload)
	case EvtToolInvocationCompleted:
		return ToolInvocationCompletedPayloadFrom(payload)
	case EvtSummaryFallbackStarted:
		return decodePayloadAs[SummaryFallbackStartedPayload](payload)
	case EvtContextPreflight,
		EvtContextPostflightUsage,
		EvtContextCompactionTriggered,
		EvtContextCompactionWarning:
		return decodePayloadAs[ContextEventPayload](payload)
	case EvtContextCompactionPending,
		EvtContextCompactionRunning,
		EvtContextCompactionSucceeded,
		EvtContextCompactionFailed:
		return decodePayloadAs[CompactionEventPayload](payload)
	case EvtSummaryRequested,
		EvtSummarySaved,
		EvtSummaryFailed,
		EvtSummaryParseFailed,
		EvtSummaryApplied,
		EvtRecallSummaryRequested,
		EvtRecallSummarySaved,
		EvtRecallSummaryFailed:
		return decodePayloadAs[SummaryEventPayload](payload)
	case EvtMemoryRetrievalStarted,
		EvtMemoryRetrieved,
		EvtMemoryRetrievalFailed,
		EvtMemoryFlushStarted,
		EvtMemoryFlushModelRequested,
		EvtMemoryFlushWriteRequested,
		EvtMemoryFlushSkipped,
		EvtMemoryFlushFailed,
		EvtMemoryFlushTimeout,
		EvtMemoryFlushCompleted,
		EvtMemoryExtractionStarted,
		EvtMemoryExtractionWindowLoaded,
		EvtMemoryExtractionExtractorRequested,
		EvtMemoryExtractionCandidates,
		EvtMemoryExtractionCandidateGenerated,
		EvtMemoryExtractionCandidateRejected,
		EvtMemoryExtractionConfidenceScored,
		EvtMemoryExtractionAdmissionHandoff,
		EvtMemoryExtractionMemoryWritten,
		EvtMemoryExtractionDuplicateSkipped,
		EvtMemoryExtractionFailed,
		EvtMemoryExtractionCompleted,
		EvtMemoryEpisodicBackgroundScheduled,
		EvtMemoryEpisodicBackgroundEligibilityChecked,
		EvtMemoryEpisodicBackgroundWindowCheckpoint,
		EvtMemoryEpisodicBackgroundExtractionAttempt,
		EvtMemoryEpisodicBackgroundRetry,
		EvtMemoryEpisodicBackgroundFailed,
		EvtMemoryEpisodicBackgroundCompleted,
		EvtMemoryReflectionStarted,
		EvtMemoryReflectionSourceLoaded,
		EvtMemoryReflectionRelatedLoaded,
		EvtMemoryReflectionCandidateGenerated,
		EvtMemoryReflectionCandidateRejected,
		EvtMemoryReflectionMemoryWritten,
		EvtMemoryReflectionFailed,
		EvtMemoryReflectionCompleted,
		EvtMemoryPromotionStarted,
		EvtMemoryPromotionDecision,
		EvtMemoryPromotionCompleted,
		EvtMemoryPromotionFailed,
		EvtMemoryPromotionFallback:
		return decodePayloadAs[MemoryEventPayload](payload)
	case EvtWorkspaceRulesTruncated:
		return decodePayloadAs[WorkspaceRulesTruncatedPayload](payload)
	case EvtPlanUpdated,
		EvtPlanCleared,
		EvtPlanHydrated:
		return decodePayloadAs[PlanEventPayload](payload)
	default:
		return nil, false
	}
}

func DecodePayloadJSON(eventType string, payload json.RawMessage) (any, bool) {
	if len(payload) == 0 {
		return DecodePayload(eventType, nil)
	}

	return DecodePayload(eventType, payload)
}

func ToolInvocationStartedPayloadFrom(payload any) (ToolInvocationStartedPayload, bool) {
	switch value := payload.(type) {
	case ToolInvocationStartedPayload:
		return value, value.ID != "" || value.Name != ""
	case models.ToolCall:
		return ToolInvocationStartedPayload{
			ID:    strings.TrimSpace(value.ID),
			Name:  strings.TrimSpace(value.Name),
			Input: value.Input,
		}, strings.TrimSpace(value.ID) != "" || strings.TrimSpace(value.Name) != ""
	case handmsg.ToolCall:
		return ToolInvocationStartedPayload{
			ID:    strings.TrimSpace(value.ID),
			Name:  strings.TrimSpace(value.Name),
			Input: value.Input,
		}, strings.TrimSpace(value.ID) != "" || strings.TrimSpace(value.Name) != ""
	}

	fields := PayloadFields(payload)
	if len(fields) == 0 {
		return ToolInvocationStartedPayload{}, false
	}

	result := ToolInvocationStartedPayload{
		ID:        PayloadString(fields, "id", "ID", "tool_call_id", "ToolCallID"),
		Name:      PayloadString(fields, "name", "Name", "tool"),
		Input:     PayloadString(fields, "input", "Input"),
		Detail:    PayloadString(fields, "detail", "Detail"),
		PlanState: planToolStateFromAny(fields["plan_state"]),
	}

	return result, result.ID != "" || result.Name != ""
}

func ToolInvocationCompletedPayloadFrom(payload any) (ToolInvocationCompletedPayload, bool) {
	switch value := payload.(type) {
	case ToolInvocationCompletedPayload:
		return value, value.ToolCallID != "" || value.Name != ""
	case handmsg.Message:
		return ToolInvocationCompletedPayload{
			ToolCallID: strings.TrimSpace(value.ToolCallID),
			Name:       strings.TrimSpace(value.Name),
			Content:    value.Content,
		}, strings.TrimSpace(value.ToolCallID) != "" || strings.TrimSpace(value.Name) != ""
	}

	fields := PayloadFields(payload)
	if len(fields) == 0 {
		return ToolInvocationCompletedPayload{}, false
	}

	result := ToolInvocationCompletedPayload{
		ToolCallID: PayloadString(fields, "tool_call_id", "ToolCallID", "id", "ID"),
		Name:       PayloadString(fields, "name", "Name", "tool"),
		Content:    PayloadString(fields, "content", "Content"),
		Detail:     PayloadString(fields, "detail", "Detail"),
		PlanState:  planToolStateFromAny(fields["plan_state"]),
	}

	return result, result.ToolCallID != "" || result.Name != ""
}

func PayloadFields(payload any) map[string]any {
	if payload == nil {
		return nil
	}
	if fields, ok := payload.(map[string]any); ok {
		return fields
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}

	return fields
}

func PayloadString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := fields[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			return text
		}
	}

	return ""
}

func decodePayloadAs[T any](payload any) (T, bool) {
	if payload == nil {
		var empty T
		return empty, true
	}
	if value, ok := payload.(T); ok {
		return value, true
	}

	data, err := json.Marshal(payload)
	if err != nil {
		var empty T
		return empty, false
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		var empty T
		return empty, false
	}

	return result, true
}

func planToolStateFromAny(value any) *PlanToolState {
	fields := PayloadFields(value)
	if len(fields) == 0 {
		return nil
	}

	return &PlanToolState{
		Operation:      PlanToolOperation(PayloadString(fields, "operation", "Operation")),
		ChangedCount:   payloadInt(fields["changed_count"]),
		TotalCount:     payloadInt(fields["total_count"]),
		CompletedCount: payloadInt(fields["completed_count"]),
	}
}

func payloadInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}

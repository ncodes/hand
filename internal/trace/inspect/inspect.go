package inspect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	handtrace "github.com/wandxy/hand/internal/trace"
)

var (
	statPath      = os.Stat
	readDirectory = os.ReadDir
	openPath      = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	newScanner    = bufio.NewScanner
)

type Store struct {
	directory string
}

type SessionSummary struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AgentName   string    `json:"agent_name,omitempty"`
	Model       string    `json:"model,omitempty"`
	APIMode     string    `json:"api_mode,omitempty"`
	EventCount  int       `json:"event_count"`
	FinalStatus string    `json:"final_status"`
	LoadError   string    `json:"load_error,omitempty"`
}

type SessionDetail struct {
	Summary   SessionSummary     `json:"summary"`
	Timeline  []TimelineEvent    `json:"timeline"`
	Memories  *SessionMemoryView `json:"memories,omitempty"`
	Warnings  []string           `json:"warnings,omitempty"`
	LoadError string             `json:"load_error,omitempty"`
}

type SessionMemoryView struct {
	Source    string       `json:"source"`
	Items     []MemoryView `json:"items,omitempty"`
	LoadError string       `json:"load_error,omitempty"`
}

type MemoryView struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status"`
	Title       string             `json:"title,omitempty"`
	Text        string             `json:"text,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	SourceLinks []MemorySourceView `json:"source_links,omitempty"`
	Confidence  float64            `json:"confidence"`
	CreatedAt   time.Time          `json:"created_at,omitempty"`
	UpdatedAt   time.Time          `json:"updated_at,omitempty"`
}

type MemorySourceView struct {
	SessionID     string `json:"session_id,omitempty"`
	MessageIDs    []uint `json:"message_ids,omitempty"`
	Offsets       []int  `json:"offsets,omitempty"`
	SummaryID     string `json:"summary_id,omitempty"`
	CreatedBy     string `json:"created_by,omitempty"`
	CreatedReason string `json:"created_reason,omitempty"`
}

type SessionMemoryProvider interface {
	ListSessionMemories(context.Context, string) ([]storage.MemoryItem, error)
}

func memoryItemToMemoryView(item storage.MemoryItem) MemoryView {
	links := make([]MemorySourceView, 0, len(item.SourceLinks))
	for _, link := range item.SourceLinks {
		links = append(links, MemorySourceView{
			SessionID:     link.SessionID,
			MessageIDs:    append([]uint(nil), link.MessageIDs...),
			Offsets:       append([]int(nil), link.Offsets...),
			SummaryID:     link.SummaryID,
			CreatedBy:     link.CreatedBy,
			CreatedReason: link.CreatedReason,
		})
	}

	metadata := make(map[string]string, len(item.Metadata))
	for key, value := range item.Metadata {
		metadata[key] = value
	}

	return MemoryView{
		ID:          item.ID,
		Kind:        string(item.Kind),
		Status:      string(item.Status),
		Title:       item.Title,
		Text:        item.Text,
		Tags:        append([]string(nil), item.Tags...),
		Metadata:    metadata,
		SourceLinks: links,
		Confidence:  item.Confidence,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

type TimelineEvent struct {
	Index             int                  `json:"index"`
	Type              string               `json:"type"`
	Timestamp         time.Time            `json:"timestamp"`
	Raw               string               `json:"raw"`
	UserMessage       *UserMessageView     `json:"user_message,omitempty"`
	ModelRequest      *ModelRequestView    `json:"model_request,omitempty"`
	ModelResponse     *ModelResponseView   `json:"model_response,omitempty"`
	ToolInvocation    *ToolInvocationView  `json:"tool_invocation,omitempty"`
	FinalResponse     *FinalResponseView   `json:"final_response,omitempty"`
	Failure           *FailureView         `json:"failure,omitempty"`
	SummaryFallback   *SummaryFallbackView `json:"summary_fallback,omitempty"`
	StartedMetadata   *StartedMetadataView `json:"started_metadata,omitempty"`
	ContextEvent      *ContextEventView    `json:"context_event,omitempty"`
	SummaryEvent      *SummaryEventView    `json:"summary_event,omitempty"`
	CompactionEvent   *CompactionEventView `json:"compaction_event,omitempty"`
	WorkspaceRules    *WorkspaceRulesView  `json:"workspace_rules,omitempty"`
	PlanEvent         *PlanEventView       `json:"plan_event,omitempty"`
	SafetyEvent       *SafetyEventView     `json:"safety_event,omitempty"`
	GenericPayloadRaw string               `json:"generic_payload_raw,omitempty"`
}

type StartedMetadataView struct {
	AgentName string `json:"agent_name,omitempty"`
	Model     string `json:"model,omitempty"`
	APIMode   string `json:"api_mode,omitempty"`
	Source    string `json:"source,omitempty"`
	TraceDir  string `json:"trace_dir,omitempty"`
}

type UserMessageView struct {
	Message string `json:"message"`
}

type ModelRequestView struct {
	Sequence        int              `json:"sequence"`
	Model           string           `json:"model,omitempty"`
	APIMode         string           `json:"api_mode,omitempty"`
	Instructions    string           `json:"instructions,omitempty"`
	MaxOutputTokens int64            `json:"max_output_tokens"`
	Temperature     float64          `json:"temperature"`
	DebugRequests   bool             `json:"debug_requests"`
	Context         RequestMetrics   `json:"context"`
	Messages        []MessageView    `json:"messages,omitempty"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
}

type RequestMetrics struct {
	InstructionChars int `json:"instruction_chars"`
	MessageCount     int `json:"message_count"`
	MessageChars     int `json:"message_chars"`
	ToolCount        int `json:"tool_count"`
	ToolCallCount    int `json:"tool_call_count"`
}

type MessageView struct {
	Role         string         `json:"role,omitempty"`
	Name         string         `json:"name,omitempty"`
	Content      string         `json:"content,omitempty"`
	ContentChars int            `json:"content_chars"`
	CreatedAt    time.Time      `json:"created_at,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCallView `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ModelResponseView struct {
	Sequence          int            `json:"sequence"`
	ID                string         `json:"id,omitempty"`
	Model             string         `json:"model,omitempty"`
	OutputText        string         `json:"output_text,omitempty"`
	RequiresToolCalls bool           `json:"requires_tool_calls"`
	ToolCalls         []ToolCallView `json:"tool_calls,omitempty"`
}

type ToolCallView struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input string `json:"input,omitempty"`
}

type ToolInvocationView struct {
	Phase      string `json:"phase"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Input      string `json:"input,omitempty"`
	Content    string `json:"content,omitempty"`
	PairIndex  *int   `json:"pair_index,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type FinalResponseView struct {
	Message string `json:"message"`
}

type FailureView struct {
	Error string `json:"error"`
}

type SummaryFallbackView struct {
	Payload string `json:"payload,omitempty"`
}

type ContextEventView struct {
	Source           string `json:"source,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	ContextLimit     int    `json:"context_limit,omitempty"`
	TriggerThreshold int    `json:"trigger_threshold,omitempty"`
	WarnThreshold    int    `json:"warn_threshold,omitempty"`
}

type SummaryEventView struct {
	SessionID          string    `json:"session_id,omitempty"`
	SourceEndOffset    int       `json:"source_end_offset,omitempty"`
	SourceMessageCount int       `json:"source_message_count,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
	Error              string    `json:"error,omitempty"`
}

type CompactionEventView struct {
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

type WorkspaceRulesView struct {
	OriginalLength   int    `json:"original_length,omitempty"`
	TruncatedLength  int    `json:"truncated_length,omitempty"`
	MaxContentLength int    `json:"max_content_length,omitempty"`
	Marker           string `json:"marker,omitempty"`
}

type PlanEventView struct {
	SessionID    string          `json:"session_id,omitempty"`
	Steps        []PlanStepView  `json:"steps,omitempty"`
	Summary      PlanSummaryView `json:"summary"`
	ActiveStepID string          `json:"active_step_id,omitempty"`
	Explanation  string          `json:"explanation,omitempty"`
	Source       string          `json:"source,omitempty"`
}

type PlanStepView struct {
	ID      string `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

type PlanSummaryView struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

type SafetyEventView struct {
	SessionID     string              `json:"session_id,omitempty"`
	Source        string              `json:"source,omitempty"`
	Action        string              `json:"action,omitempty"`
	ContentLength int                 `json:"content_length"`
	Blocked       bool                `json:"blocked"`
	Redacted      bool                `json:"redacted"`
	Refusal       string              `json:"refusal,omitempty"`
	Findings      []map[string]string `json:"findings,omitempty"`
}

type rawEvent struct {
	SessionID string          `json:"session_id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func NewStore(directory string) *Store {
	return &Store{directory: strings.TrimSpace(directory)}
}

func (s *Store) Validate() error {
	if s == nil || strings.TrimSpace(s.directory) == "" {
		return errors.New("trace directory is required")
	}

	info, err := statPath(s.directory)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("trace directory %q does not exist", s.directory)
		}

		return fmt.Errorf("failed to access trace directory %q: %w", s.directory, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("trace directory %q is not a directory", s.directory)
	}

	return nil
}

func (s *Store) ListSessions() ([]SessionSummary, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	entries, err := readDirectory(s.directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read trace directory %q: %w", s.directory, err)
	}

	summaries := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		detail, err := LoadSessionFile(path)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, detail.Summary)
	}

	slices.SortFunc(summaries, func(a, b SessionSummary) int {
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			if a.UpdatedAt.After(b.UpdatedAt) {
				return -1
			}

			return 1
		}

		return strings.Compare(b.ID, a.ID)
	})

	return summaries, nil
}

func (s *Store) GetSession(id string) (SessionDetail, error) {
	if err := s.Validate(); err != nil {
		return SessionDetail{}, err
	}

	path, err := getSessionPath(s.directory, id)
	if err != nil {
		return SessionDetail{}, os.ErrNotExist
	}

	if _, err := statPath(path); err != nil {
		if os.IsNotExist(err) {
			return SessionDetail{}, os.ErrNotExist
		}

		return SessionDetail{}, err
	}

	return LoadSessionFile(path)
}

func getSessionPath(directory, id string) (string, error) {
	directory = strings.TrimSpace(directory)
	id = strings.TrimSpace(id)
	if directory == "" || id == "" {
		return "", os.ErrNotExist
	}

	return handtrace.ResolveTraceFilePath(directory, id)
}

func LoadSessionFile(path string) (SessionDetail, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SessionDetail{}, errors.New("trace session path is required")
	}

	fileStem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	logicalID := handtrace.SessionIDFromTraceFilename(fileStem)
	detail := SessionDetail{
		Summary: SessionSummary{
			ID:          logicalID,
			Path:        path,
			FinalStatus: "incomplete",
		},
	}

	info, err := statPath(path)
	if err != nil {
		return SessionDetail{}, err
	}
	detail.Summary.StartedAt = info.ModTime().UTC()

	file, err := openPath(path)
	if err != nil {
		return SessionDetail{}, err
	}
	defer file.Close()

	scanner := newScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 8*1024*1024)

	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var event rawEvent
		if err := json.Unmarshal(line, &event); err != nil {
			detail.LoadError = fmt.Sprintf("failed to parse line %d: %v", lineNo, err)
			detail.Summary.LoadError = detail.LoadError
			detail.Summary.FinalStatus = "load_error"
			if detail.Summary.UpdatedAt.IsZero() {
				detail.Summary.UpdatedAt = info.ModTime().UTC()
			}

			return detail, nil
		}

		detail.Summary.EventCount++
		if !event.Timestamp.IsZero() && (detail.Summary.StartedAt.IsZero() ||
			event.Timestamp.Before(detail.Summary.StartedAt)) {
			detail.Summary.StartedAt = event.Timestamp
		}
		if !event.Timestamp.IsZero() && (detail.Summary.UpdatedAt.IsZero() ||
			event.Timestamp.After(detail.Summary.UpdatedAt)) {
			detail.Summary.UpdatedAt = event.Timestamp
		}
		if event.SessionID != "" && event.SessionID != logicalID {
			detail.Warnings = append(detail.Warnings, fmt.Sprintf("event session id %q does not match file id %q",
				event.SessionID, logicalID))
		}

		timelineEvent := TimelineEvent{
			Index:     len(detail.Timeline),
			Type:      event.Type,
			Timestamp: event.Timestamp,
			Raw:       compactJSON(line),
		}
		applyEvent(&detail, &timelineEvent, event)
		detail.Timeline = append(detail.Timeline, timelineEvent)
	}

	if err := scanner.Err(); err != nil {
		return SessionDetail{}, err
	}

	if detail.Summary.UpdatedAt.IsZero() {
		detail.Summary.UpdatedAt = info.ModTime().UTC()
	}

	pairToolInvocations(detail.Timeline)
	numberInteractions(detail.Timeline)
	return detail, nil
}

func applyEvent(detail *SessionDetail, timelineEvent *TimelineEvent, event rawEvent) {
	typedPayload, payloadOK := handtrace.DecodePayloadJSON(event.Type, event.Payload)

	switch event.Type {
	case handtrace.EvtChatStarted:
		if payload, ok := typedPayload.(handtrace.Metadata); payloadOK && ok {
			detail.Summary.AgentName = payload.AgentName
			detail.Summary.Model = payload.Model
			detail.Summary.APIMode = payload.APIMode
			timelineEvent.StartedMetadata = &StartedMetadataView{
				AgentName: payload.AgentName,
				Model:     payload.Model,
				APIMode:   payload.APIMode,
				Source:    payload.Source,
				TraceDir:  payload.TraceDir,
			}

			return
		}
	case handtrace.EvtUserMessageAccepted:
		if payload, ok := typedPayload.(handtrace.UserMessageAcceptedPayload); payloadOK && ok {
			timelineEvent.UserMessage = &UserMessageView{Message: firstNonEmpty(payload.Message, payload.Text)}
			detail.Summary.FinalStatus = "in_progress"
			return
		}
	case handtrace.EvtModelRequest:
		if payload, ok := typedPayload.(models.Request); payloadOK && ok {
			timelineEvent.ModelRequest = buildRequestView(payload)
			if detail.Summary.Model == "" {
				detail.Summary.Model = payload.Model
			}
			if detail.Summary.APIMode == "" {
				detail.Summary.APIMode = payload.APIMode
			}

			return
		}
	case handtrace.EvtModelResponse:
		if payload, ok := typedPayload.(models.Response); payloadOK && ok {
			timelineEvent.ModelResponse = buildSessionMessagesResponseView(payload)
			return
		}
	case handtrace.EvtToolInvocationStarted:
		if payload, ok := typedPayload.(handtrace.ToolInvocationStartedPayload); payloadOK && ok {
			timelineEvent.ToolInvocation = &ToolInvocationView{
				Phase: "started",
				ID:    payload.ID,
				Name:  payload.Name,
				Input: payload.Input,
			}

			return
		}
	case handtrace.EvtToolInvocationCompleted:
		if payload, ok := typedPayload.(handtrace.ToolInvocationCompletedPayload); payloadOK && ok {
			timelineEvent.ToolInvocation = &ToolInvocationView{
				Phase:      "completed",
				Name:       payload.Name,
				Content:    payload.Content,
				ToolCallID: payload.ToolCallID,
				ID:         payload.ToolCallID,
			}

			return
		}
	case handtrace.EvtFinalAssistantResponse:
		if payload, ok := typedPayload.(handtrace.FinalAssistantResponsePayload); payloadOK && ok {
			timelineEvent.FinalResponse = &FinalResponseView{Message: firstNonEmpty(payload.Message, payload.Text)}
			detail.Summary.FinalStatus = "completed"
			return
		}
	case handtrace.EvtSessionFailed:
		if payload, ok := typedPayload.(handtrace.SessionFailedPayload); payloadOK && ok {
			timelineEvent.Failure = &FailureView{Error: firstNonEmpty(payload.Error, payload.Message)}
			detail.Summary.FinalStatus = "failed"
			return
		}
	case handtrace.EvtSummaryFallbackStarted:
		timelineEvent.SummaryFallback = &SummaryFallbackView{Payload: compactJSON(event.Payload)}
		return
	case handtrace.EvtContextPreflight, handtrace.EvtContextCompactionTriggered,
		handtrace.EvtContextCompactionWarning, handtrace.EvtContextPostflightUsage:
		if payload, ok := typedPayload.(handtrace.ContextEventPayload); payloadOK && ok {
			timelineEvent.ContextEvent = &ContextEventView{
				Source:           payload.Source,
				PromptTokens:     payload.PromptTokens,
				CompletionTokens: payload.CompletionTokens,
				TotalTokens:      payload.TotalTokens,
				ContextLimit:     payload.ContextLimit,
				TriggerThreshold: payload.TriggerThreshold,
				WarnThreshold:    payload.WarnThreshold,
			}

			return
		}
	case handtrace.EvtSummaryRequested, handtrace.EvtSummarySaved, handtrace.EvtSummaryFailed,
		handtrace.EvtSummaryParseFailed, handtrace.EvtSummaryApplied,
		handtrace.EvtRecallSummaryRequested, handtrace.EvtRecallSummarySaved, handtrace.EvtRecallSummaryFailed:
		if payload, ok := typedPayload.(handtrace.SummaryEventPayload); payloadOK && ok {
			timelineEvent.SummaryEvent = &SummaryEventView{
				SessionID:          payload.SessionID,
				SourceEndOffset:    payload.SourceEndOffset,
				SourceMessageCount: payload.SourceMessageCount,
				UpdatedAt:          payload.UpdatedAt,
				Error:              strings.TrimSpace(payload.Error),
			}

			return
		}
	case handtrace.EvtContextCompactionPending, handtrace.EvtContextCompactionRunning,
		handtrace.EvtContextCompactionSucceeded, handtrace.EvtContextCompactionFailed:
		if payload, ok := typedPayload.(handtrace.CompactionEventPayload); payloadOK && ok {
			timelineEvent.CompactionEvent = &CompactionEventView{
				SessionID:          payload.SessionID,
				Status:             payload.Status,
				TargetMessageCount: payload.TargetMessageCount,
				TargetOffset:       payload.TargetOffset,
				RequestedAt:        payload.RequestedAt,
				StartedAt:          payload.StartedAt,
				CompletedAt:        payload.CompletedAt,
				FailedAt:           payload.FailedAt,
				Error:              strings.TrimSpace(payload.Error),
			}

			return
		}
	case handtrace.EvtWorkspaceRulesTruncated:
		if payload, ok := typedPayload.(handtrace.WorkspaceRulesTruncatedPayload); payloadOK && ok {
			timelineEvent.WorkspaceRules = &WorkspaceRulesView{
				OriginalLength:   payload.OriginalLength,
				TruncatedLength:  payload.TruncatedLength,
				MaxContentLength: payload.MaxContentLength,
				Marker:           payload.Marker,
			}
			return
		}
	case handtrace.EvtPlanUpdated, handtrace.EvtPlanCleared, handtrace.EvtPlanHydrated:
		if payload, ok := typedPayload.(handtrace.PlanEventPayload); payloadOK && ok {
			timelineEvent.PlanEvent = &PlanEventView{
				SessionID:    payload.SessionID,
				Steps:        planStepViewsFromPayload(payload.Steps),
				Summary:      planSummaryViewFromPayload(payload.Summary),
				ActiveStepID: payload.ActiveStepID,
				Explanation:  strings.TrimSpace(payload.Explanation),
				Source:       strings.TrimSpace(payload.Source),
			}
			return
		}
	case handtrace.EvtInputSafetyBlocked, handtrace.EvtOutputSafetyApplied,
		handtrace.EvtToolOutputSafetyApplied, handtrace.EvtLoadedContentSafetyBlocked,
		handtrace.EvtMemorySafetyBlocked:
		if payload, ok := typedPayload.(handtrace.SafetyEventPayload); payloadOK && ok {
			timelineEvent.SafetyEvent = &SafetyEventView{
				SessionID:     strings.TrimSpace(payload.SessionID),
				Source:        strings.TrimSpace(payload.Source),
				Action:        strings.TrimSpace(payload.Action),
				ContentLength: payload.ContentLength,
				Blocked:       payload.Blocked,
				Redacted:      payload.Redacted,
				Refusal:       strings.TrimSpace(payload.Refusal),
				Findings:      payload.Findings,
			}
			return
		}
	}

	timelineEvent.GenericPayloadRaw = compactJSON(event.Payload)
}

func buildRequestView(payload models.Request) *ModelRequestView {
	messages := make([]MessageView, 0, len(payload.Messages))
	metrics := RequestMetrics{
		InstructionChars: len(payload.Instructions),
		MessageCount:     len(payload.Messages),
		ToolCount:        len(payload.Tools),
	}

	for _, message := range payload.Messages {
		messageView := MessageView{
			Role:         string(message.Role),
			Name:         message.Name,
			Content:      message.Content,
			ContentChars: len(message.Content),
			CreatedAt:    message.CreatedAt,
			ToolCallID:   message.ToolCallID,
			ToolCalls:    toolCallsToToolCallViews(message.ToolCalls),
		}
		metrics.MessageChars += len(message.Content)
		metrics.ToolCallCount += len(message.ToolCalls)
		messages = append(messages, messageView)
	}

	tools := make([]ToolDefinition, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		tools = append(tools, ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}

	return &ModelRequestView{
		Model:           payload.Model,
		APIMode:         payload.APIMode,
		Instructions:    payload.Instructions,
		MaxOutputTokens: payload.MaxOutputTokens,
		Temperature:     payload.Temperature,
		DebugRequests:   payload.DebugRequests,
		Context:         metrics,
		Messages:        messages,
		Tools:           tools,
	}
}

func buildSessionMessagesResponseView(payload models.Response) *ModelResponseView {
	return &ModelResponseView{
		ID:                payload.ID,
		Model:             payload.Model,
		OutputText:        payload.OutputText,
		RequiresToolCalls: payload.RequiresToolCalls,
		ToolCalls:         buildToolCallViews(payload.ToolCalls),
	}
}

func planStepViewsFromPayload(steps []handtrace.PlanStepPayload) []PlanStepView {
	if len(steps) == 0 {
		return nil
	}

	views := make([]PlanStepView, 0, len(steps))
	for _, step := range steps {
		views = append(views, PlanStepView{
			ID:      step.ID,
			Content: step.Content,
			Status:  step.Status,
		})
	}

	return views
}

func planSummaryViewFromPayload(summary handtrace.PlanSummaryPayload) PlanSummaryView {
	return PlanSummaryView{
		Total:      summary.Total,
		Pending:    summary.Pending,
		InProgress: summary.InProgress,
		Completed:  summary.Completed,
		Cancelled:  summary.Cancelled,
	}
}

func buildToolCallViews(toolCalls []models.ToolCall) []ToolCallView {
	if len(toolCalls) == 0 {
		return nil
	}

	views := make([]ToolCallView, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		views = append(views, ToolCallView{
			ID:    toolCall.ID,
			Name:  toolCall.Name,
			Input: toolCall.Input,
		})
	}

	return views
}

func toolCallsToToolCallViews(toolCalls []handmsg.ToolCall) []ToolCallView {
	if len(toolCalls) == 0 {
		return nil
	}

	views := make([]ToolCallView, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		views = append(views, ToolCallView{
			ID:    toolCall.ID,
			Name:  toolCall.Name,
			Input: toolCall.Input,
		})
	}

	return views
}

func pairToolInvocations(timeline []TimelineEvent) {
	starts := map[string]int{}
	for index := range timeline {
		toolInvocation := timeline[index].ToolInvocation
		if toolInvocation == nil || strings.TrimSpace(toolInvocation.ID) == "" {
			continue
		}
		id := strings.TrimSpace(toolInvocation.ID)
		switch toolInvocation.Phase {
		case "started":
			starts[id] = index
		case "completed":
			startIndex, ok := starts[id]
			if !ok {
				continue
			}
			timeline[index].ToolInvocation.PairIndex = new(startIndex)
			timeline[startIndex].ToolInvocation.PairIndex = new(index)
			delete(starts, id)
		}
	}
}

func numberInteractions(timeline []TimelineEvent) {
	requests := 0
	responses := 0
	for index := range timeline {
		if timeline[index].ModelRequest != nil {
			requests++
			timeline[index].ModelRequest.Sequence = requests
		}
		if timeline[index].ModelResponse != nil {
			responses++
			timeline[index].ModelResponse.Sequence = responses
		}
	}
}

func compactJSON(value []byte) string {
	if len(bytes.TrimSpace(value)) == 0 {
		return ""
	}

	var out bytes.Buffer
	if err := json.Compact(&out, value); err != nil {
		return strings.TrimSpace(string(value))
	}

	return out.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

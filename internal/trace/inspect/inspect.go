package inspect

import (
	"bufio"
	"bytes"
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
	StartedAt   time.Time `json:"started_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	AgentName   string    `json:"agent_name,omitempty"`
	Model       string    `json:"model,omitempty"`
	APIMode     string    `json:"api_mode,omitempty"`
	EventCount  int       `json:"event_count"`
	FinalStatus string    `json:"final_status"`
	LoadError   string    `json:"load_error,omitempty"`
}

type SessionDetail struct {
	Summary   SessionSummary  `json:"summary"`
	Timeline  []TimelineEvent `json:"timeline"`
	Warnings  []string        `json:"warnings,omitempty"`
	LoadError string          `json:"load_error,omitempty"`
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

type rawEvent struct {
	SessionID string          `json:"session_id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type userMessagePayload struct {
	Message string `json:"message"`
}

type contextEventPayload struct {
	Source           string `json:"source"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	ContextLimit     int    `json:"context_limit"`
	TriggerThreshold int    `json:"trigger_threshold"`
	WarnThreshold    int    `json:"warn_threshold"`
}

type summaryEventPayload struct {
	SessionID          string    `json:"session_id"`
	SourceEndOffset    int       `json:"source_end_offset"`
	SourceMessageCount int       `json:"source_message_count"`
	UpdatedAt          time.Time `json:"updated_at"`
	Error              string    `json:"error"`
}

type compactionEventPayload struct {
	SessionID          string    `json:"session_id"`
	Status             string    `json:"status"`
	TargetMessageCount int       `json:"target_message_count"`
	TargetOffset       int       `json:"target_offset"`
	RequestedAt        time.Time `json:"requested_at"`
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at"`
	FailedAt           time.Time `json:"failed_at"`
	Error              string    `json:"error"`
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

	path, err := resolveSessionPath(s.directory, id)
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

func resolveSessionPath(directory, id string) (string, error) {
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
	switch event.Type {
	case handtrace.EvtChatStarted:
		var payload handtrace.Metadata
		if json.Unmarshal(event.Payload, &payload) == nil {
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
		var payload userMessagePayload
		if json.Unmarshal(event.Payload, &payload) == nil {
			timelineEvent.UserMessage = &UserMessageView{Message: payload.Message}
			detail.Summary.FinalStatus = "in_progress"
			return
		}
	case handtrace.EvtModelRequest:
		var payload models.Request
		if json.Unmarshal(event.Payload, &payload) == nil {
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
		var payload models.Response
		if json.Unmarshal(event.Payload, &payload) == nil {
			timelineEvent.ModelResponse = buildResponseView(payload)
			return
		}
	case handtrace.EvtToolInvocationStarted:
		var payload models.ToolCall
		if json.Unmarshal(event.Payload, &payload) == nil {
			timelineEvent.ToolInvocation = &ToolInvocationView{
				Phase: "started",
				ID:    payload.ID,
				Name:  payload.Name,
				Input: payload.Input,
			}

			return
		}
	case handtrace.EvtToolInvocationCompleted:
		var payload handmsg.Message
		if json.Unmarshal(event.Payload, &payload) == nil {
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
		var payload map[string]string
		if json.Unmarshal(event.Payload, &payload) == nil {
			timelineEvent.FinalResponse = &FinalResponseView{Message: strings.TrimSpace(payload["message"])}
			detail.Summary.FinalStatus = "completed"
			return
		}
	case handtrace.EvtSessionFailed:
		var payload map[string]string
		if json.Unmarshal(event.Payload, &payload) == nil {
			timelineEvent.Failure = &FailureView{Error: strings.TrimSpace(payload["error"])}
			detail.Summary.FinalStatus = "failed"
			return
		}
	case handtrace.EvtSummaryFallbackStarted:
		timelineEvent.SummaryFallback = &SummaryFallbackView{Payload: compactJSON(event.Payload)}
		return
	case handtrace.EvtContextPreflight, handtrace.EvtContextCompactionTriggered,
		handtrace.EvtContextCompactionWarning, handtrace.EvtContextPostflightUsage:
		var payload contextEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
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
		handtrace.EvtSummaryParseFailed, handtrace.EvtSummaryApplied:
		var payload summaryEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
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
		var payload compactionEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
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
			ToolCalls:    buildToolCallViewsFromContext(message.ToolCalls),
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

func buildResponseView(payload models.Response) *ModelResponseView {
	return &ModelResponseView{
		ID:                payload.ID,
		Model:             payload.Model,
		OutputText:        payload.OutputText,
		RequiresToolCalls: payload.RequiresToolCalls,
		ToolCalls:         buildToolCallViews(payload.ToolCalls),
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

func buildToolCallViewsFromContext(toolCalls []handmsg.ToolCall) []ToolCallView {
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

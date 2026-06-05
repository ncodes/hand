package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentapi "github.com/wandxy/hand/internal/agent"
	models "github.com/wandxy/hand/internal/model"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/trace"
	agent "github.com/wandxy/hand/pkg/agent"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
)

// Client wraps a gRPC connection to the Hand RPC services.
type Client struct {
	conn        *grpc.ClientConn
	reconnector rpcReconnector
	client      handpb.HandServiceClient
	Session     *SessionService
	Model       *ModelService
}

type SessionService struct {
	client      handpb.SessionServiceClient
	reconnector rpcReconnector
}

type ModelService struct {
	client      handpb.ModelServiceClient
	reconnector rpcReconnector
}

type rpcReconnector interface {
	ResetConnectBackoff()
	Connect()
}

// RespondOptions mirrors agent response options at this package boundary.
type RespondOptions = agent.RespondOptions

// Event aliases agent.Event at this package boundary.
type Event = agent.Event

// CompactSessionResult aliases agent.CompactSessionResult at this package boundary.
type CompactSessionResult = agent.CompactSessionResult

// ContextStatus aliases agent.ContextStatus at this package boundary.
type ContextStatus = agent.ContextStatus

// SessionTimelineOptions mirrors agent timeline query options at this package boundary.
type SessionTimelineOptions = agentapi.SessionTimelineOptions

// SessionTimeline mirrors the agent timeline type at this package boundary.
type SessionTimeline = agentapi.SessionTimeline

// RepairSessionOptions aliases search.VectorRepairOptions at this package boundary.
type RepairSessionOptions = search.VectorRepairOptions

// RepairSessionResult aliases search.VectorRepairResult at this package boundary.
type RepairSessionResult = search.VectorRepairResult

type CreateSessionOptions struct {
	ID         string
	AutoSwitch *bool
}

type SessionListOptions = storage.SessionListOptions

type ModelListOptions = agentapi.ModelListOptions

type ModelSelectOptions = agentapi.ModelSelectOptions

type ProviderOption = models.ProviderOption

type ProviderList = agentapi.ProviderList

type ModelOption = models.Option

type ModelList = agentapi.ModelList

// ChatAPI is the chat surface exposed by local and RPC clients.
type ChatAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
}

// SessionAPI is the session-management surface exposed by local and RPC clients.
type SessionAPI interface {
	Create(context.Context, string) (storage.Session, error)
	CreateWithOptions(context.Context, CreateSessionOptions) (storage.Session, error)
	List(context.Context, ...SessionListOptions) ([]storage.Session, error)
	Use(context.Context, string) error
	Archive(context.Context, string) error
	Unarchive(context.Context, string) (storage.Session, error)
	Rename(context.Context, string, string) (storage.Session, error)
	Current(context.Context) (storage.Session, error)
	Compact(context.Context, string) (CompactSessionResult, error)
	Repair(context.Context, RepairSessionOptions) (RepairSessionResult, error)
	Status(context.Context, string) (ContextStatus, error)
	Timeline(context.Context, SessionTimelineOptions) (SessionTimeline, error)
}

type ModelAPI interface {
	ListProviders(context.Context) (ProviderList, error)
	ListModels(context.Context, ...ModelListOptions) (ModelList, error)
	SelectModel(context.Context, string, ...agentapi.ModelSelectOptions) (ModelOption, error)
	SetProviderAPIKey(context.Context, string, string) error
}

// ServiceAPI combines chat and session operations.
type ServiceAPI interface {
	ChatAPI
	SessionAPI() SessionAPI
	ModelAPI() ModelAPI
}

// ChatClient is a closable client that can run chat turns.
type ChatClient interface {
	ChatAPI
	Close() error
}

// ClientAPI is the complete closable RPC client surface.
type ClientAPI interface {
	ServiceAPI
	Close() error
}

// Options configures this package operation.
type Options struct {
	Address string
	Port    int
}

// NewClient returns a client configured with the supplied dependencies.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return nil, fmt.Errorf("rpc address is required")
	}

	if opts.Port <= 0 {
		return nil, fmt.Errorf("rpc port must be greater than zero")
	}

	target := fmt.Sprintf("%s:%d", address, opts.Port)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:        conn,
		reconnector: conn,
		client:      handpb.NewHandServiceClient(conn),
		Session:     newSessionService(handpb.NewSessionServiceClient(conn), conn),
		Model:       newModelService(handpb.NewModelServiceClient(conn), conn),
	}, nil
}

func NewSessionService(client handpb.SessionServiceClient) *SessionService {
	return newSessionService(client, nil)
}

func NewModelService(client handpb.ModelServiceClient) *ModelService {
	return newModelService(client, nil)
}

func newSessionService(client handpb.SessionServiceClient, reconnector rpcReconnector) *SessionService {
	return &SessionService{client: client, reconnector: reconnector}
}

func newModelService(client handpb.ModelServiceClient, reconnector rpcReconnector) *ModelService {
	return &ModelService{client: client, reconnector: reconnector}
}

func prepareRPCConnection(reconnector rpcReconnector) {
	if reconnector == nil {
		return
	}

	reconnector.ResetConnectBackoff()
	reconnector.Connect()
}

func (c *Client) Respond(ctx context.Context, message string, opts RespondOptions) (string, error) {
	req := &handpb.RespondRequest{
		Message:  message,
		Instruct: strings.TrimSpace(opts.Instruct),
		Id:       strings.TrimSpace(opts.SessionID),
	}
	if opts.Stream != nil {
		req.Stream = opts.Stream
	}

	prepareRPCConnection(c.reconnector)
	stream, err := c.client.Respond(ctx, req)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for {
		event, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				return builder.String(), errors.New("respond stream ended before done event")
			}
			return builder.String(), recvErr
		}
		switch event.GetType() {
		case handpb.RespondEvent_TEXT_DELTA:
			if event.GetChannel() != handpb.RespondEvent_REASONING {
				builder.WriteString(event.GetText())
			}
			if opts.OnEvent != nil {
				opts.OnEvent(Event{
					Kind:    agent.EventKindTextDelta,
					Channel: protoStreamChannelToAgentChannel(event.GetChannel()),
					Text:    event.GetText(),
				})
			}
		case handpb.RespondEvent_TRACE_EVENT:
			if opts.OnEvent != nil {
				traceEvent, ok := protoRespondTraceEventToTraceEvent(event)
				if ok {
					opts.OnEvent(Event{
						Kind:       agent.EventKindTrace,
						TraceEvent: &traceEvent,
					})
				}
			}
		case handpb.RespondEvent_ERROR:
			message := strings.TrimSpace(event.GetError())
			if message == "" {
				message = "respond stream failed"
			}
			return builder.String(), errors.New(message)
		case handpb.RespondEvent_DONE:
			return builder.String(), nil
		}
	}
}

func protoRespondTraceEventToTraceEvent(event *handpb.RespondEvent) (trace.Event, bool) {
	if event == nil {
		return trace.Event{}, false
	}

	eventType := strings.TrimSpace(event.GetTraceType())
	if eventType == "" {
		return trace.Event{}, false
	}

	traceEvent := trace.Event{
		SessionID: strings.TrimSpace(event.GetTraceSessionId()),
		Type:      eventType,
		Timestamp: protoTimestampToTime(event.GetTimestamp()),
	}
	if payloadJSON := strings.TrimSpace(event.GetTracePayloadJson()); payloadJSON != "" {
		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return trace.Event{}, false
		}
		traceEvent.Payload = payload
	}

	return traceEvent, true
}

func protoStreamChannelToAgentChannel(channel handpb.RespondEvent_Channel) string {
	switch channel {
	case handpb.RespondEvent_REASONING:
		return "reasoning"
	default:
		return "assistant"
	}
}

func (c *Client) SessionAPI() SessionAPI {
	if c == nil {
		return nil
	}

	return c.Session
}

func (c *Client) ModelAPI() ModelAPI {
	if c == nil {
		return nil
	}

	return c.Model
}

func (s *ModelService) ListProviders(ctx context.Context) (ProviderList, error) {
	client, err := s.getClient()
	if err != nil {
		return ProviderList{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.ListProviders(ctx, &handpb.ListProvidersRequest{})
	if err != nil {
		return ProviderList{}, err
	}

	providers := make([]ProviderOption, 0, len(resp.GetProviders()))
	for _, provider := range resp.GetProviders() {
		providers = append(providers, protoProviderOptionToProviderOption(provider))
	}

	return ProviderList{Providers: providers}, nil
}

func (s *ModelService) ListModels(ctx context.Context, opts ...ModelListOptions) (ModelList, error) {
	client, err := s.getClient()
	if err != nil {
		return ModelList{}, err
	}

	listOpts := getModelListOptions(opts...)
	prepareRPCConnection(s.reconnector)
	resp, err := client.ListModels(ctx, &handpb.ListModelsRequest{Provider: strings.TrimSpace(listOpts.Provider)})
	if err != nil {
		return ModelList{}, err
	}

	models := make([]ModelOption, 0, len(resp.GetModels()))
	for _, model := range resp.GetModels() {
		models = append(models, protoModelOptionToModelOption(model))
	}

	return ModelList{
		Provider: strings.TrimSpace(resp.GetProvider()),
		AuthType: strings.TrimSpace(resp.GetAuthType()),
		Models:   models,
	}, nil
}

func getModelListOptions(opts ...ModelListOptions) ModelListOptions {
	if len(opts) == 0 {
		return ModelListOptions{}
	}

	return opts[0]
}

func getModelSelectOptions(opts ...agentapi.ModelSelectOptions) agentapi.ModelSelectOptions {
	if len(opts) == 0 {
		return agentapi.ModelSelectOptions{}
	}

	return opts[0]
}

func (s *ModelService) SelectModel(
	ctx context.Context,
	id string,
	opts ...agentapi.ModelSelectOptions,
) (ModelOption, error) {
	client, err := s.getClient()
	if err != nil {
		return ModelOption{}, err
	}

	selectOpts := getModelSelectOptions(opts...)
	prepareRPCConnection(s.reconnector)
	resp, err := client.SelectModel(ctx, &handpb.SelectModelRequest{
		Id:       strings.TrimSpace(id),
		Provider: strings.TrimSpace(selectOpts.Provider),
	})
	if err != nil {
		return ModelOption{}, err
	}

	return protoModelOptionToModelOption(resp.GetModel()), nil
}

func (s *ModelService) SetProviderAPIKey(ctx context.Context, provider string, apiKey string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	_, err = client.SetProviderAPIKey(ctx, &handpb.SetProviderAPIKeyRequest{
		Provider: strings.TrimSpace(provider),
		ApiKey:   strings.TrimSpace(apiKey),
	})
	return err
}

func (s *SessionService) Create(ctx context.Context, id string) (storage.Session, error) {
	return s.CreateWithOptions(ctx, CreateSessionOptions{ID: id})
}

func (s *SessionService) CreateWithOptions(ctx context.Context, opts CreateSessionOptions) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	req := &handpb.CreateSessionRequest{Id: strings.TrimSpace(opts.ID)}
	if opts.AutoSwitch != nil {
		req.AutoSwitch = opts.AutoSwitch
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Create(ctx, req)
	if err != nil {
		return storage.Session{}, err
	}

	if resp.GetSession() == nil {
		return storage.Session{}, nil
	}

	return protoSessionSummaryToSession(resp.GetSession()), nil
}

func (s *SessionService) List(ctx context.Context, opts ...SessionListOptions) ([]storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	listOpts := getSessionListOptions(opts...)
	prepareRPCConnection(s.reconnector)
	resp, err := client.List(ctx, &handpb.ListSessionsRequest{Archived: listOpts.Archived})
	if err != nil {
		return nil, err
	}

	items := make([]storage.Session, 0, len(resp.GetSessions()))
	for _, session := range resp.GetSessions() {
		item := protoSessionSummaryToSession(session)
		if listOpts.Archived != nil {
			item.Archived = *listOpts.Archived
		}
		items = append(items, item)
	}

	return items, nil
}

func getSessionListOptions(opts ...SessionListOptions) SessionListOptions {
	if len(opts) == 0 {
		active := false
		return SessionListOptions{Archived: &active}
	}

	return opts[0]
}

func (s *SessionService) Use(ctx context.Context, id string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	_, err = client.Use(ctx, &handpb.UseSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (s *SessionService) Archive(ctx context.Context, id string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	_, err = client.Archive(ctx, &handpb.ArchiveSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (s *SessionService) Unarchive(ctx context.Context, id string) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Unarchive(ctx, &handpb.UnarchiveSessionRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return storage.Session{}, err
	}

	return protoSessionSummaryToSession(resp.GetSession()), nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Rename(ctx, &handpb.RenameSessionRequest{
		Id:    strings.TrimSpace(id),
		Title: strings.TrimSpace(title),
	})
	if err != nil {
		return storage.Session{}, err
	}

	return protoSessionSummaryToSession(resp.GetSession()), nil
}

func (s *SessionService) Current(ctx context.Context) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Current(ctx, &handpb.CurrentSessionRequest{})
	if err != nil {
		return storage.Session{}, err
	}

	return storage.Session{
		ID:          resp.GetId(),
		Title:       resp.GetTitle(),
		TitleSource: resp.GetTitleSource(),
	}, nil
}

func (s *SessionService) Compact(ctx context.Context, id string) (CompactSessionResult, error) {
	client, err := s.getClient()
	if err != nil {
		return CompactSessionResult{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Compact(ctx, &handpb.CompactSessionRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return CompactSessionResult{}, err
	}

	return CompactSessionResult{
		SessionID:            resp.GetId(),
		SourceEndOffset:      int(resp.GetSourceEndOffset()),
		SourceMessageCount:   int(resp.GetSourceMessageCount()),
		UpdatedAt:            protoTimestampToTime(resp.GetUpdatedAt()),
		CurrentContextLength: int(resp.GetCurrentContextLength()),
		TotalContextLength:   int(resp.GetTotalContextLength()),
	}, nil
}

func (s *SessionService) Repair(
	ctx context.Context,
	opts RepairSessionOptions,
) (RepairSessionResult, error) {
	client, err := s.getClient()
	if err != nil {
		return RepairSessionResult{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Repair(ctx, &handpb.RepairSessionRequest{
		Type: handpb.RepairSessionRequest_VECTOR,
		Vector: &handpb.VectorRepairOption{
			Id:   strings.TrimSpace(opts.SessionID),
			Full: opts.Full,
		},
	})
	if err != nil {
		return RepairSessionResult{}, err
	}
	vector := resp.GetVector()

	return RepairSessionResult{
		SessionsScanned: int(vector.GetSessionsScanned()),
		MessagesScanned: int(vector.GetMessagesScanned()),
		RowsScanned:     int(vector.GetRowsScanned()),
		MissingRows:     int(vector.GetMissingRows()),
		StaleRows:       int(vector.GetStaleRows()),
		UnchangedRows:   int(vector.GetUnchangedRows()),
		RebuiltRows:     int(vector.GetRebuiltRows()),
		DeletedSources:  int(vector.GetDeletedSources()),
		Batches:         int(vector.GetBatches()),
	}, nil
}

func (s *SessionService) Status(ctx context.Context, id string) (ContextStatus, error) {
	client, err := s.getClient()
	if err != nil {
		return ContextStatus{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Status(ctx, &handpb.GetSessionStatusRequest{
		Context: &handpb.GetSessionStatusRequestContext{Id: strings.TrimSpace(id)},
	})
	if err != nil {
		return ContextStatus{}, err
	}
	cctx := resp.GetContext()
	if cctx == nil {
		return ContextStatus{}, fmt.Errorf("hand: get session status response context is required")
	}

	return ContextStatus{
		SessionID:        resp.GetId(),
		Offset:           int(cctx.GetOffset()),
		Size:             int(resp.GetSize()),
		Length:           int(cctx.GetLength()),
		Used:             int(cctx.GetUsed()),
		Remaining:        int(cctx.GetRemaining()),
		UsedPct:          cctx.GetUsedPct(),
		RemainingPct:     cctx.GetRemainingPct(),
		CreatedAt:        protoTimestampToTime(resp.GetCreatedAt()),
		UpdatedAt:        protoTimestampToTime(resp.GetUpdatedAt()),
		CompactionStatus: resp.GetCompactionStatus(),
	}, nil
}

func (s *SessionService) Timeline(
	ctx context.Context,
	opts SessionTimelineOptions,
) (SessionTimeline, error) {
	client, err := s.getClient()
	if err != nil {
		return SessionTimeline{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Timeline(ctx, &handpb.GetSessionTimelineRequest{
		Id:            strings.TrimSpace(opts.SessionID),
		MessageOffset: int32(opts.MessageOffset),
		MessageLimit:  int32(opts.MessageLimit),
		TraceOffset:   int32(opts.TraceOffset),
		TraceLimit:    int32(opts.TraceLimit),
	})
	if err != nil {
		return SessionTimeline{}, err
	}

	return protoSessionTimelineToTimeline(resp)
}

func (s *SessionService) getClient() (handpb.SessionServiceClient, error) {
	if s != nil && s.client != nil {
		return s.client, nil
	}

	return nil, fmt.Errorf("hand: session service client is required")
}

func (s *ModelService) getClient() (handpb.ModelServiceClient, error) {
	if s != nil && s.client != nil {
		return s.client, nil
	}

	return nil, fmt.Errorf("hand: model service client is required")
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func protoSessionTimelineToTimeline(resp *handpb.GetSessionTimelineResponse) (SessionTimeline, error) {
	if resp == nil {
		return SessionTimeline{}, fmt.Errorf("hand: get session timeline response is required")
	}

	timeline := SessionTimeline{
		SessionID:             resp.GetId(),
		Title:                 resp.GetTitle(),
		TitleSource:           resp.GetTitleSource(),
		MessagesHasMore:       resp.GetMessagesHasMore(),
		TracesHasMore:         resp.GetTracesHasMore(),
		TracesTruncatedBefore: resp.GetTracesTruncatedBefore(),
		FirstTraceSequence:    int(resp.GetFirstTraceSequence()),
		LastTraceSequence:     int(resp.GetLastTraceSequence()),
		Messages:              make([]agentapi.SessionTimelineMessage, 0, len(resp.GetMessages())),
		TraceEvents:           make([]agentapi.SessionTimelineTraceEvent, 0, len(resp.GetTraceEvents())),
	}
	for _, message := range resp.GetMessages() {
		timeline.Messages = append(timeline.Messages, timelineMessageFromProto(message))
	}
	for _, event := range resp.GetTraceEvents() {
		timelineEvent, err := timelineTraceEventFromProto(event)
		if err != nil {
			return SessionTimeline{}, err
		}
		timeline.TraceEvents = append(timeline.TraceEvents, timelineEvent)
	}

	return timeline, nil
}

func timelineMessageFromProto(message *handpb.SessionTimelineMessage) agentapi.SessionTimelineMessage {
	if message == nil {
		return agentapi.SessionTimelineMessage{}
	}

	toolCalls := make([]handmsg.ToolCall, 0, len(message.GetToolCalls()))
	for _, toolCall := range message.GetToolCalls() {
		toolCalls = append(toolCalls, handmsg.ToolCall{
			ID:    strings.TrimSpace(toolCall.GetId()),
			Name:  strings.TrimSpace(toolCall.GetName()),
			Input: strings.TrimSpace(toolCall.GetInput()),
		})
	}

	return agentapi.SessionTimelineMessage{
		Offset: int(message.GetOffset()),
		Message: handmsg.Message{
			ID:         uint(message.GetId()),
			Role:       handmsg.Role(message.GetRole()),
			Name:       message.GetName(),
			ToolCallID: message.GetToolCallId(),
			Content:    message.GetContent(),
			CreatedAt:  protoTimestampToTime(message.GetCreatedAt()),
			ToolCalls:  toolCalls,
		},
	}
}

func timelineTraceEventFromProto(event *handpb.SessionTimelineTraceEvent) (agentapi.SessionTimelineTraceEvent, error) {
	if event == nil {
		return agentapi.SessionTimelineTraceEvent{}, nil
	}

	var payload any
	if payloadJSON := strings.TrimSpace(event.GetPayloadJson()); payloadJSON != "" {
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return agentapi.SessionTimelineTraceEvent{}, err
		}
	}

	return agentapi.SessionTimelineTraceEvent{
		Event: agentsession.TraceEvent{
			ID:        uint(event.GetId()),
			Sequence:  int(event.GetSequence()),
			Type:      strings.TrimSpace(event.GetType()),
			Timestamp: protoTimestampToTime(event.GetTimestamp()),
			Payload:   payload,
		},
	}, nil
}

func protoSessionSummaryToSession(summary *handpb.SessionSummary) storage.Session {
	if summary == nil {
		return storage.Session{}
	}

	return storage.Session{
		ID:          summary.GetId(),
		Title:       summary.GetTitle(),
		TitleSource: summary.GetTitleSource(),
		UpdatedAt:   time.Unix(summary.GetUpdatedAtUnix(), 0).UTC(),
	}
}

func protoProviderOptionToProviderOption(option *handpb.ProviderOption) ProviderOption {
	if option == nil {
		return ProviderOption{}
	}

	return ProviderOption{
		ID:             strings.TrimSpace(option.GetId()),
		Name:           strings.TrimSpace(option.GetName()),
		Type:           strings.TrimSpace(option.GetType()),
		ModelCount:     int(option.GetModelCount()),
		SupportsAPIKey: option.GetSupportsApiKey(),
		SupportsOAuth:  option.GetSupportsOauth(),
		AuthType:       strings.TrimSpace(option.GetAuthType()),
		Current:        option.GetCurrent(),
	}
}

func protoModelOptionToModelOption(option *handpb.ModelOption) ModelOption {
	if option == nil {
		return ModelOption{}
	}

	return ModelOption{
		ID:            strings.TrimSpace(option.GetId()),
		Name:          strings.TrimSpace(option.GetName()),
		Provider:      strings.TrimSpace(option.GetProvider()),
		API:           strings.TrimSpace(option.GetApi()),
		ContextWindow: int(option.GetContextWindow()),
		MaxTokens:     int(option.GetMaxTokens()),
		Input:         append([]string(nil), option.GetInput()...),
		Reasoning:     option.GetReasoning(),
		SupportsOAuth: option.GetSupportsOauth(),
		Current:       option.GetCurrent(),
	}
}

func protoTimestampToTime(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

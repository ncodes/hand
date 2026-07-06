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

	agentapi "github.com/wandxy/morph/internal/agent"
	models "github.com/wandxy/morph/internal/model"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/internal/state/search"
	"github.com/wandxy/morph/internal/trace"
	agent "github.com/wandxy/morph/pkg/agent"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	agentsession "github.com/wandxy/morph/pkg/agent/session"
	"github.com/wandxy/morph/pkg/str"
)

// Client wraps a gRPC connection to the Morph RPC services.
type Client struct {
	conn        *grpc.ClientConn
	reconnector rpcReconnector
	client      morphpb.MorphServiceClient
	Session     *SessionService
	Model       *ModelService
	Gateway     *GatewayService
}

type SessionService struct {
	client      morphpb.SessionServiceClient
	reconnector rpcReconnector
}

type ModelService struct {
	client      morphpb.ModelServiceClient
	reconnector rpcReconnector
}

type GatewayService struct {
	client      morphpb.GatewayServiceClient
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

type ModelRuntime struct {
	Provider      string
	API           string
	Model         string
	BaseURL       string
	ContextLength int
}

type GatewayPairingRequest struct {
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	Source      string
	SenderID    string
	DisplayName string
}

type GatewayPairedSender struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Source      string
	SenderID    string
	DisplayName string
}

type GatewayPairingList struct {
	Pending  []GatewayPairingRequest
	Approved []GatewayPairedSender
}

type GatewayStatus struct {
	State        string
	Address      string
	Port         int
	SlackMode    string
	TelegramMode string
	LastError    string
}

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
	RuntimeModel(context.Context) (ModelRuntime, error)
	ListProviders(context.Context) (ProviderList, error)
	ListModels(context.Context, ...ModelListOptions) (ModelList, error)
	SelectModel(context.Context, string, ...agentapi.ModelSelectOptions) (ModelOption, error)
	SetProviderAPIKey(context.Context, string, string) error
}

type GatewayAPI interface {
	GatewayStatus(context.Context) (GatewayStatus, error)
	Start(context.Context) (GatewayStatus, error)
	Stop(context.Context) (GatewayStatus, error)
	Restart(context.Context) (GatewayStatus, error)
	ListPairings(context.Context, string) (GatewayPairingList, error)
	ApprovePairing(context.Context, string, string) (GatewayPairedSender, bool, error)
	RevokePairing(context.Context, string, string) error
	ClearPendingPairings(context.Context, string) error
}

// ServiceAPI combines chat and session operations.
type ServiceAPI interface {
	ChatAPI
	SessionAPI() SessionAPI
	ModelAPI() ModelAPI
	GatewayAPI() GatewayAPI
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
	stringValue1 := str.String(opts.Address)
	address := stringValue1.Trim()
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
		client:      morphpb.NewMorphServiceClient(conn),
		Session:     newSessionService(morphpb.NewSessionServiceClient(conn), conn),
		Model:       newModelService(morphpb.NewModelServiceClient(conn), conn),
		Gateway:     newGatewayService(morphpb.NewGatewayServiceClient(conn), conn),
	}, nil
}

func NewSessionService(client morphpb.SessionServiceClient) *SessionService {
	return newSessionService(client, nil)
}

func NewModelService(client morphpb.ModelServiceClient) *ModelService {
	return newModelService(client, nil)
}

func NewGatewayService(client morphpb.GatewayServiceClient) *GatewayService {
	return newGatewayService(client, nil)
}

func newSessionService(client morphpb.SessionServiceClient, reconnector rpcReconnector) *SessionService {
	return &SessionService{client: client, reconnector: reconnector}
}

func newModelService(client morphpb.ModelServiceClient, reconnector rpcReconnector) *ModelService {
	return &ModelService{client: client, reconnector: reconnector}
}

func newGatewayService(client morphpb.GatewayServiceClient, reconnector rpcReconnector) *GatewayService {
	return &GatewayService{client: client, reconnector: reconnector}
}

func prepareRPCConnection(reconnector rpcReconnector) {
	if reconnector == nil {
		return
	}

	reconnector.ResetConnectBackoff()
	reconnector.Connect()
}

func (c *Client) Respond(ctx context.Context, message string, opts RespondOptions) (string, error) {
	stringValue2 := str.String(opts.Instruct)
	stringValue3 := str.String(opts.SessionID)
	req := &morphpb.RespondRequest{
		Message:  message,
		Instruct: stringValue2.Trim(),
		Id:       stringValue3.Trim(),
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
		case morphpb.RespondEvent_TEXT_DELTA:
			if event.GetChannel() != morphpb.RespondEvent_REASONING {
				builder.WriteString(event.GetText())
			}
			if opts.OnEvent != nil {
				opts.OnEvent(Event{
					Kind:    agent.EventKindTextDelta,
					Channel: protoStreamChannelToAgentChannel(event.GetChannel()),
					Text:    event.GetText(),
				})
			}
		case morphpb.RespondEvent_TRACE_EVENT:
			if opts.OnEvent != nil {
				traceEvent, ok := protoRespondTraceEventToTraceEvent(event)
				if ok {
					opts.OnEvent(Event{
						Kind:       agent.EventKindTrace,
						TraceEvent: &traceEvent,
					})
				}
			}
		case morphpb.RespondEvent_ERROR:
			eventError := str.String(event.GetError())
			message := eventError.Trim()
			if message == "" {
				message = "respond stream failed"
			}
			return builder.String(), errors.New(message)
		case morphpb.RespondEvent_DONE:
			return builder.String(), nil
		}
	}
}

func protoRespondTraceEventToTraceEvent(event *morphpb.RespondEvent) (trace.Event, bool) {
	if event == nil {
		return trace.Event{}, false
	}
	stringValue5 := str.String(event.GetTraceType())
	eventType := stringValue5.Trim()
	if eventType == "" {
		return trace.Event{}, false
	}
	stringValue6 := str.String(event.GetTraceSessionId())
	traceEvent := trace.Event{
		SessionID: stringValue6.Trim(),
		Type:      eventType,
		Timestamp: protoTimestampToTime(event.GetTimestamp()),
	}
	stringValue7 := str.String(event.GetTracePayloadJson())
	if payloadJSON := stringValue7.Trim(); payloadJSON != "" {
		var payload any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return trace.Event{}, false
		}
		traceEvent.Payload = payload
	}

	return traceEvent, true
}

func protoStreamChannelToAgentChannel(channel morphpb.RespondEvent_Channel) string {
	switch channel {
	case morphpb.RespondEvent_REASONING:
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

func (c *Client) GatewayAPI() GatewayAPI {
	if c == nil {
		return nil
	}

	return c.Gateway
}

func (s *ModelService) ListProviders(ctx context.Context) (ProviderList, error) {
	client, err := s.getClient()
	if err != nil {
		return ProviderList{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.ListProviders(ctx, &morphpb.ListProvidersRequest{})
	if err != nil {
		return ProviderList{}, err
	}

	providers := make([]ProviderOption, 0, len(resp.GetProviders()))
	for _, provider := range resp.GetProviders() {
		providers = append(providers, protoProviderOptionToProviderOption(provider))
	}

	return ProviderList{Providers: providers}, nil
}

func (s *ModelService) RuntimeModel(ctx context.Context) (ModelRuntime, error) {
	client, err := s.getClient()
	if err != nil {
		return ModelRuntime{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.RuntimeModel(ctx, &morphpb.RuntimeModelRequest{})
	if err != nil {
		return ModelRuntime{}, err
	}

	return protoRuntimeModelToModelRuntime(resp), nil
}

func (s *ModelService) ListModels(ctx context.Context, opts ...ModelListOptions) (ModelList, error) {
	client, err := s.getClient()
	if err != nil {
		return ModelList{}, err
	}

	listOpts := getModelListOptions(opts...)
	prepareRPCConnection(s.reconnector)
	stringValue8 := str.String(listOpts.Provider)
	resp, err := client.ListModels(ctx, &morphpb.ListModelsRequest{Provider: stringValue8.Trim()})
	if err != nil {
		return ModelList{}, err
	}

	models := make([]ModelOption, 0, len(resp.GetModels()))
	for _, model := range resp.GetModels() {
		models = append(models, protoModelOptionToModelOption(model))
	}
	stringValue9 := str.String(resp.GetProvider())
	stringValue10 := str.String(resp.GetAuthType())
	return ModelList{
		Provider: stringValue9.Trim(),
		AuthType: stringValue10.Trim(),
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
	stringValue11 := str.String(id)
	stringValue12 := str.String(selectOpts.Provider)
	resp, err := client.SelectModel(ctx, &morphpb.SelectModelRequest{
		Id:       stringValue11.Trim(),
		Provider: stringValue12.Trim(),
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
	stringValue13 := str.String(provider)
	stringValue14 := str.String(apiKey)
	_, err = client.SetProviderAPIKey(ctx, &morphpb.SetProviderAPIKeyRequest{
		Provider: stringValue13.Trim(),
		ApiKey:   stringValue14.Trim(),
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
	stringValue15 := str.String(opts.ID)
	req := &morphpb.CreateSessionRequest{Id: stringValue15.Trim()}
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
	resp, err := client.List(ctx, &morphpb.ListSessionsRequest{Archived: listOpts.Archived})
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
	stringValue16 := str.String(id)
	_, err = client.Use(ctx, &morphpb.UseSessionRequest{Id: stringValue16.Trim()})
	return err
}

func (s *SessionService) Archive(ctx context.Context, id string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	stringValue17 := str.String(id)
	_, err = client.Archive(ctx, &morphpb.ArchiveSessionRequest{Id: stringValue17.Trim()})
	return err
}

func (s *SessionService) Unarchive(ctx context.Context, id string) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	prepareRPCConnection(s.reconnector)
	stringValue18 := str.String(id)
	resp, err := client.Unarchive(ctx, &morphpb.UnarchiveSessionRequest{Id: stringValue18.Trim()})
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
	stringValue19 := str.String(id)
	stringValue20 := str.String(title)
	resp, err := client.Rename(ctx, &morphpb.RenameSessionRequest{
		Id:    stringValue19.Trim(),
		Title: stringValue20.Trim(),
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
	resp, err := client.Current(ctx, &morphpb.CurrentSessionRequest{})
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
	stringValue21 := str.String(id)
	resp, err := client.Compact(ctx, &morphpb.CompactSessionRequest{Id: stringValue21.Trim()})
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
	stringValue22 := str.String(opts.SessionID)
	resp, err := client.Repair(ctx, &morphpb.RepairSessionRequest{
		Type: morphpb.RepairSessionRequest_VECTOR,
		Vector: &morphpb.VectorRepairOption{
			Id:   stringValue22.Trim(),
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
	stringValue23 := str.String(id)
	resp, err := client.Status(ctx, &morphpb.GetSessionStatusRequest{
		Context: &morphpb.GetSessionStatusRequestContext{Id: stringValue23.Trim()},
	})
	if err != nil {
		return ContextStatus{}, err
	}
	cctx := resp.GetContext()
	if cctx == nil {
		return ContextStatus{}, fmt.Errorf("morph: get session status response context is required")
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
	stringValue24 := str.String(opts.SessionID)
	resp, err := client.Timeline(ctx, &morphpb.GetSessionTimelineRequest{
		Id:            stringValue24.Trim(),
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

func (s *GatewayService) ListPairings(ctx context.Context, source string) (GatewayPairingList, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayPairingList{}, err
	}

	prepareRPCConnection(s.reconnector)
	stringValue25 := str.String(source)
	resp, err := client.ListPairings(ctx, &morphpb.ListGatewayPairingsRequest{Source: stringValue25.Trim()})
	if err != nil {
		return GatewayPairingList{}, err
	}

	result := GatewayPairingList{}
	for _, pending := range resp.GetPending() {
		result.Pending = append(result.Pending, protoGatewayPairingRequestToGatewayPairingRequest(pending))
	}
	for _, approved := range resp.GetApproved() {
		result.Approved = append(result.Approved, protoGatewayPairedSenderToGatewayPairedSender(approved))
	}

	return result, nil
}

func (s *GatewayService) GatewayStatus(ctx context.Context) (GatewayStatus, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayStatus{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.GatewayStatus(ctx, &morphpb.GetGatewayStatusRequest{})
	if err != nil {
		return GatewayStatus{}, err
	}

	return gatewayStatusFromProto(resp.GetStatus()), nil
}

func (s *GatewayService) Start(ctx context.Context) (GatewayStatus, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayStatus{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Start(ctx, &morphpb.StartGatewayRequest{})
	if err != nil {
		return GatewayStatus{}, err
	}

	return gatewayStatusFromProto(resp.GetStatus()), nil
}

func (s *GatewayService) Stop(ctx context.Context) (GatewayStatus, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayStatus{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Stop(ctx, &morphpb.StopGatewayRequest{})
	if err != nil {
		return GatewayStatus{}, err
	}

	return gatewayStatusFromProto(resp.GetStatus()), nil
}

func (s *GatewayService) Restart(ctx context.Context) (GatewayStatus, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayStatus{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Restart(ctx, &morphpb.RestartGatewayRequest{})
	if err != nil {
		return GatewayStatus{}, err
	}

	return gatewayStatusFromProto(resp.GetStatus()), nil
}

func (s *GatewayService) ApprovePairing(
	ctx context.Context,
	source string,
	code string,
) (GatewayPairedSender, bool, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayPairedSender{}, false, err
	}

	prepareRPCConnection(s.reconnector)
	stringValue26 := str.String(source)
	stringValue27 := str.String(code)
	resp, err := client.ApprovePairing(ctx, &morphpb.ApproveGatewayPairingRequest{
		Source: stringValue26.Trim(),
		Code:   stringValue27.Trim(),
	})
	if err != nil {
		return GatewayPairedSender{}, false, err
	}

	return protoGatewayPairedSenderToGatewayPairedSender(resp.GetSender()), resp.GetApproved(), nil
}

func (s *GatewayService) RevokePairing(ctx context.Context, source string, senderID string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	stringValue28 := str.String(source)
	stringValue29 := str.String(senderID)
	_, err = client.RevokePairing(ctx, &morphpb.RevokeGatewayPairingRequest{
		Source:   stringValue28.Trim(),
		SenderId: stringValue29.Trim(),
	})

	return err
}

func (s *GatewayService) ClearPendingPairings(ctx context.Context, source string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	stringValue30 := str.String(source)
	_, err = client.ClearPendingPairings(ctx, &morphpb.ClearPendingGatewayPairingsRequest{
		Source: stringValue30.Trim(),
	})

	return err
}

func (s *SessionService) getClient() (morphpb.SessionServiceClient, error) {
	if s != nil && s.client != nil {
		return s.client, nil
	}

	return nil, fmt.Errorf("morph: session service client is required")
}

func (s *ModelService) getClient() (morphpb.ModelServiceClient, error) {
	if s != nil && s.client != nil {
		return s.client, nil
	}

	return nil, fmt.Errorf("morph: model service client is required")
}

func (s *GatewayService) getClient() (morphpb.GatewayServiceClient, error) {
	if s != nil && s.client != nil {
		return s.client, nil
	}

	return nil, fmt.Errorf("morph: gateway service client is required")
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func protoSessionTimelineToTimeline(resp *morphpb.GetSessionTimelineResponse) (SessionTimeline, error) {
	if resp == nil {
		return SessionTimeline{}, fmt.Errorf("morph: get session timeline response is required")
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

func timelineMessageFromProto(message *morphpb.SessionTimelineMessage) agentapi.SessionTimelineMessage {
	if message == nil {
		return agentapi.SessionTimelineMessage{}
	}

	toolCalls := make([]morphmsg.ToolCall, 0, len(message.GetToolCalls()))
	for _, toolCall := range message.GetToolCalls() {
		stringValue31 := str.String(toolCall.GetId())
		stringValue32 := str.String(toolCall.GetName())
		stringValue33 := str.String(toolCall.GetInput())
		toolCalls = append(toolCalls, morphmsg.ToolCall{
			ID:    stringValue31.Trim(),
			Name:  stringValue32.Trim(),
			Input: stringValue33.Trim(),
		})
	}

	return agentapi.SessionTimelineMessage{
		Offset: int(message.GetOffset()),
		Message: morphmsg.Message{
			ID:         uint(message.GetId()),
			Role:       morphmsg.Role(message.GetRole()),
			Name:       message.GetName(),
			ToolCallID: message.GetToolCallId(),
			Content:    message.GetContent(),
			CreatedAt:  protoTimestampToTime(message.GetCreatedAt()),
			ToolCalls:  toolCalls,
		},
	}
}

func timelineTraceEventFromProto(event *morphpb.SessionTimelineTraceEvent) (agentapi.SessionTimelineTraceEvent, error) {
	if event == nil {
		return agentapi.SessionTimelineTraceEvent{}, nil
	}

	var payload any
	stringValue34 := str.String(event.GetPayloadJson())
	if payloadJSON := stringValue34.Trim(); payloadJSON != "" {
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return agentapi.SessionTimelineTraceEvent{}, err
		}
	}
	stringValue35 := str.String(event.GetType())
	return agentapi.SessionTimelineTraceEvent{
		Event: agentsession.TraceEvent{
			ID:        uint(event.GetId()),
			Sequence:  int(event.GetSequence()),
			Type:      stringValue35.Trim(),
			Timestamp: protoTimestampToTime(event.GetTimestamp()),
			Payload:   payload,
		},
	}, nil
}

func protoSessionSummaryToSession(summary *morphpb.SessionSummary) storage.Session {
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

func gatewayStatusFromProto(status *morphpb.GatewayStatus) GatewayStatus {
	if status == nil {
		return GatewayStatus{}
	}
	stringValue36 := str.String(status.GetState())
	stringValue37 := str.String(status.GetAddress())
	stringValue38 := str.String(status.GetSlackMode())
	stringValue39 := str.String(status.GetTelegramMode())
	stringValue40 := str.String(status.GetLastError())
	return GatewayStatus{
		State:        stringValue36.Trim(),
		Address:      stringValue37.Trim(),
		Port:         int(status.GetPort()),
		SlackMode:    stringValue38.Trim(),
		TelegramMode: stringValue39.Trim(),
		LastError:    stringValue40.Trim(),
	}
}

func protoGatewayPairingRequestToGatewayPairingRequest(
	request *morphpb.GatewayPairingRequest,
) GatewayPairingRequest {
	if request == nil {
		return GatewayPairingRequest{}
	}
	stringValue41 := str.String(request.GetSource())
	stringValue42 := str.String(request.GetSenderId())
	stringValue43 := str.String(request.GetDisplayName())
	return GatewayPairingRequest{
		Source:      stringValue41.Trim(),
		SenderID:    stringValue42.Trim(),
		DisplayName: stringValue43.Trim(),
		CreatedAt:   protoTimestampToTime(request.GetCreatedAt()),
		LastSeenAt:  protoTimestampToTime(request.GetLastSeenAt()),
		ExpiresAt:   protoTimestampToTime(request.GetExpiresAt()),
	}
}

func protoGatewayPairedSenderToGatewayPairedSender(sender *morphpb.GatewayPairedSender) GatewayPairedSender {
	if sender == nil {
		return GatewayPairedSender{}
	}
	stringValue44 := str.String(sender.GetSource())
	stringValue45 := str.String(sender.GetSenderId())
	stringValue46 := str.String(sender.GetDisplayName())
	return GatewayPairedSender{
		Source:      stringValue44.Trim(),
		SenderID:    stringValue45.Trim(),
		DisplayName: stringValue46.Trim(),
		CreatedAt:   protoTimestampToTime(sender.GetCreatedAt()),
		UpdatedAt:   protoTimestampToTime(sender.GetUpdatedAt()),
	}
}

func protoProviderOptionToProviderOption(option *morphpb.ProviderOption) ProviderOption {
	if option == nil {
		return ProviderOption{}
	}
	stringValue47 := str.String(option.GetId())
	stringValue48 := str.String(option.GetName())
	stringValue49 := str.String(option.GetType())
	stringValue50 := str.String(option.GetAuthType())
	return ProviderOption{
		ID:             stringValue47.Trim(),
		Name:           stringValue48.Trim(),
		Type:           stringValue49.Trim(),
		ModelCount:     int(option.GetModelCount()),
		SupportsAPIKey: option.GetSupportsApiKey(),
		SupportsOAuth:  option.GetSupportsOauth(),
		AuthType:       stringValue50.Trim(),
		Current:        option.GetCurrent(),
	}
}

func protoModelOptionToModelOption(option *morphpb.ModelOption) ModelOption {
	if option == nil {
		return ModelOption{}
	}
	stringValue51 := str.String(option.GetId())
	stringValue52 := str.String(option.GetName())
	stringValue53 := str.String(option.GetProvider())
	stringValue54 := str.String(option.GetApi())
	return ModelOption{
		ID:            stringValue51.Trim(),
		Name:          stringValue52.Trim(),
		Provider:      stringValue53.Trim(),
		API:           stringValue54.Trim(),
		ContextWindow: int(option.GetContextWindow()),
		MaxTokens:     int(option.GetMaxTokens()),
		Input:         append([]string(nil), option.GetInput()...),
		Reasoning:     option.GetReasoning(),
		SupportsOAuth: option.GetSupportsOauth(),
		Current:       option.GetCurrent(),
	}
}

func protoRuntimeModelToModelRuntime(runtime *morphpb.RuntimeModelResponse) ModelRuntime {
	if runtime == nil {
		return ModelRuntime{}
	}
	stringValue55 := str.String(runtime.GetProvider())
	stringValue56 := str.String(runtime.GetApi())
	stringValue57 := str.String(runtime.GetModel())
	stringValue58 := str.String(runtime.GetBaseUrl())
	return ModelRuntime{
		Provider:      stringValue55.Trim(),
		API:           stringValue56.Trim(),
		Model:         stringValue57.Trim(),
		BaseURL:       strings.TrimRight(stringValue58.Trim(), "/"),
		ContextLength: int(runtime.GetContextLength()),
	}
}

func protoTimestampToTime(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

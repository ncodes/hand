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
	req := &morphpb.RespondRequest{
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
			message := strings.TrimSpace(event.GetError())
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
	resp, err := client.ListModels(ctx, &morphpb.ListModelsRequest{Provider: strings.TrimSpace(listOpts.Provider)})
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
	resp, err := client.SelectModel(ctx, &morphpb.SelectModelRequest{
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
	_, err = client.SetProviderAPIKey(ctx, &morphpb.SetProviderAPIKeyRequest{
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

	req := &morphpb.CreateSessionRequest{Id: strings.TrimSpace(opts.ID)}
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
	_, err = client.Use(ctx, &morphpb.UseSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (s *SessionService) Archive(ctx context.Context, id string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	_, err = client.Archive(ctx, &morphpb.ArchiveSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (s *SessionService) Unarchive(ctx context.Context, id string) (storage.Session, error) {
	client, err := s.getClient()
	if err != nil {
		return storage.Session{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.Unarchive(ctx, &morphpb.UnarchiveSessionRequest{Id: strings.TrimSpace(id)})
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
	resp, err := client.Rename(ctx, &morphpb.RenameSessionRequest{
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
	resp, err := client.Compact(ctx, &morphpb.CompactSessionRequest{Id: strings.TrimSpace(id)})
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
	resp, err := client.Repair(ctx, &morphpb.RepairSessionRequest{
		Type: morphpb.RepairSessionRequest_VECTOR,
		Vector: &morphpb.VectorRepairOption{
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
	resp, err := client.Status(ctx, &morphpb.GetSessionStatusRequest{
		Context: &morphpb.GetSessionStatusRequestContext{Id: strings.TrimSpace(id)},
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
	resp, err := client.Timeline(ctx, &morphpb.GetSessionTimelineRequest{
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

func (s *GatewayService) ListPairings(ctx context.Context, source string) (GatewayPairingList, error) {
	client, err := s.getClient()
	if err != nil {
		return GatewayPairingList{}, err
	}

	prepareRPCConnection(s.reconnector)
	resp, err := client.ListPairings(ctx, &morphpb.ListGatewayPairingsRequest{Source: strings.TrimSpace(source)})
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
	resp, err := client.ApprovePairing(ctx, &morphpb.ApproveGatewayPairingRequest{
		Source: strings.TrimSpace(source),
		Code:   strings.TrimSpace(code),
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
	_, err = client.RevokePairing(ctx, &morphpb.RevokeGatewayPairingRequest{
		Source:   strings.TrimSpace(source),
		SenderId: strings.TrimSpace(senderID),
	})

	return err
}

func (s *GatewayService) ClearPendingPairings(ctx context.Context, source string) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	prepareRPCConnection(s.reconnector)
	_, err = client.ClearPendingPairings(ctx, &morphpb.ClearPendingGatewayPairingsRequest{
		Source: strings.TrimSpace(source),
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
		toolCalls = append(toolCalls, morphmsg.ToolCall{
			ID:    strings.TrimSpace(toolCall.GetId()),
			Name:  strings.TrimSpace(toolCall.GetName()),
			Input: strings.TrimSpace(toolCall.GetInput()),
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

	return GatewayStatus{
		State:        strings.TrimSpace(status.GetState()),
		Address:      strings.TrimSpace(status.GetAddress()),
		Port:         int(status.GetPort()),
		SlackMode:    strings.TrimSpace(status.GetSlackMode()),
		TelegramMode: strings.TrimSpace(status.GetTelegramMode()),
		LastError:    strings.TrimSpace(status.GetLastError()),
	}
}

func protoGatewayPairingRequestToGatewayPairingRequest(
	request *morphpb.GatewayPairingRequest,
) GatewayPairingRequest {
	if request == nil {
		return GatewayPairingRequest{}
	}

	return GatewayPairingRequest{
		Source:      strings.TrimSpace(request.GetSource()),
		SenderID:    strings.TrimSpace(request.GetSenderId()),
		DisplayName: strings.TrimSpace(request.GetDisplayName()),
		CreatedAt:   protoTimestampToTime(request.GetCreatedAt()),
		LastSeenAt:  protoTimestampToTime(request.GetLastSeenAt()),
		ExpiresAt:   protoTimestampToTime(request.GetExpiresAt()),
	}
}

func protoGatewayPairedSenderToGatewayPairedSender(sender *morphpb.GatewayPairedSender) GatewayPairedSender {
	if sender == nil {
		return GatewayPairedSender{}
	}

	return GatewayPairedSender{
		Source:      strings.TrimSpace(sender.GetSource()),
		SenderID:    strings.TrimSpace(sender.GetSenderId()),
		DisplayName: strings.TrimSpace(sender.GetDisplayName()),
		CreatedAt:   protoTimestampToTime(sender.GetCreatedAt()),
		UpdatedAt:   protoTimestampToTime(sender.GetUpdatedAt()),
	}
}

func protoProviderOptionToProviderOption(option *morphpb.ProviderOption) ProviderOption {
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

func protoModelOptionToModelOption(option *morphpb.ModelOption) ModelOption {
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

func protoRuntimeModelToModelRuntime(runtime *morphpb.RuntimeModelResponse) ModelRuntime {
	if runtime == nil {
		return ModelRuntime{}
	}

	return ModelRuntime{
		Provider:      strings.TrimSpace(runtime.GetProvider()),
		API:           strings.TrimSpace(runtime.GetApi()),
		Model:         strings.TrimSpace(runtime.GetModel()),
		BaseURL:       strings.TrimRight(strings.TrimSpace(runtime.GetBaseUrl()), "/"),
		ContextLength: int(runtime.GetContextLength()),
	}
}

func protoTimestampToTime(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

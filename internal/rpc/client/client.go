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

	handmsg "github.com/wandxy/hand/internal/messages"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/trace"
	agent "github.com/wandxy/hand/pkg/agent"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
)

type Client struct {
	conn   *grpc.ClientConn
	client handpb.HandServiceClient
}

type RespondOptions = agent.RespondOptions
type Event = agent.Event

type CompactSessionResult = agent.CompactSessionResult

type ContextStatus = agent.ContextStatus

type SessionTimelineOptions = agent.SessionTimelineOptions

type SessionTimeline = agent.SessionTimeline

type RepairSessionOptions = search.VectorRepairOptions

type RepairSessionResult = search.VectorRepairResult

type ChatAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
}

type SessionAPI interface {
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (storage.Session, error)
	CompactSession(context.Context, string) (CompactSessionResult, error)
	RepairSession(context.Context, RepairSessionOptions) (RepairSessionResult, error)
	GetSession(context.Context, string) (ContextStatus, error)
	GetSessionTimeline(context.Context, SessionTimelineOptions) (SessionTimeline, error)
}

type ServiceAPI interface {
	ChatAPI
	SessionAPI
}

type ChatClient interface {
	ChatAPI
	Close() error
}

type SessionClient interface {
	SessionAPI
	Close() error
}

type ClientAPI interface {
	ServiceAPI
	Close() error
}

type Options struct {
	Address string
	Port    int
}

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
		conn:   conn,
		client: handpb.NewHandServiceClient(conn),
	}, nil
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

	stream, err := c.client.Respond(ctx, req)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	done := false
	for {
		event, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				if done {
					break
				}
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
				opts.OnEvent(agent.Event{
					Kind:    agent.EventKindTextDelta,
					Channel: protoStreamChannelToAgentChannel(event.GetChannel()),
					Text:    event.GetText(),
				})
			}
		case handpb.RespondEvent_TRACE_EVENT:
			if opts.OnEvent != nil {
				traceEvent, ok := protoRespondTraceEventToTraceEvent(event)
				if ok {
					opts.OnEvent(agent.Event{
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
			done = true
			return builder.String(), nil
		}
	}

	return builder.String(), nil
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

func (c *Client) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	resp, err := c.client.CreateSession(ctx, &handpb.CreateSessionRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return storage.Session{}, err
	}

	if resp.GetSession() == nil {
		return storage.Session{}, nil
	}

	return protoSessionSummaryToSession(resp.GetSession()), nil
}

func (c *Client) ListSessions(ctx context.Context) ([]storage.Session, error) {
	resp, err := c.client.ListSessions(ctx, &handpb.ListSessionsRequest{})
	if err != nil {
		return nil, err
	}

	sessions := make([]storage.Session, 0, len(resp.GetSessions()))
	for _, session := range resp.GetSessions() {
		sessions = append(sessions, protoSessionSummaryToSession(session))
	}

	return sessions, nil
}

func (c *Client) UseSession(ctx context.Context, id string) error {
	_, err := c.client.UseSession(ctx, &handpb.UseSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (c *Client) CurrentSession(ctx context.Context) (storage.Session, error) {
	resp, err := c.client.CurrentSession(ctx, &handpb.CurrentSessionRequest{})
	if err != nil {
		return storage.Session{}, err
	}

	return storage.Session{
		ID:          resp.GetId(),
		Title:       resp.GetTitle(),
		TitleSource: resp.GetTitleSource(),
	}, nil
}

func (c *Client) CompactSession(ctx context.Context, id string) (CompactSessionResult, error) {
	resp, err := c.client.CompactSession(ctx, &handpb.CompactSessionRequest{Id: strings.TrimSpace(id)})
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

func (c *Client) RepairSession(
	ctx context.Context,
	opts RepairSessionOptions,
) (RepairSessionResult, error) {
	resp, err := c.client.RepairSession(ctx, &handpb.RepairSessionRequest{
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

func (c *Client) GetSession(ctx context.Context, id string) (ContextStatus, error) {
	resp, err := c.client.GetSession(ctx, &handpb.GetSessionRequest{
		Context: &handpb.GetSessionRequestContext{Id: strings.TrimSpace(id)},
	})
	if err != nil {
		return ContextStatus{}, err
	}
	cctx := resp.GetContext()
	if cctx == nil {
		return ContextStatus{}, fmt.Errorf("hand: get session response context is required")
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

func (c *Client) GetSessionTimeline(
	ctx context.Context,
	opts SessionTimelineOptions,
) (SessionTimeline, error) {
	resp, err := c.client.GetSessionTimeline(ctx, &handpb.GetSessionTimelineRequest{
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
		Messages:              make([]agent.SessionTimelineMessage, 0, len(resp.GetMessages())),
		TraceEvents:           make([]agent.SessionTimelineTraceEvent, 0, len(resp.GetTraceEvents())),
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

func timelineMessageFromProto(message *handpb.SessionTimelineMessage) agent.SessionTimelineMessage {
	if message == nil {
		return agent.SessionTimelineMessage{}
	}

	toolCalls := make([]handmsg.ToolCall, 0, len(message.GetToolCalls()))
	for _, toolCall := range message.GetToolCalls() {
		toolCalls = append(toolCalls, handmsg.ToolCall{
			ID:    strings.TrimSpace(toolCall.GetId()),
			Name:  strings.TrimSpace(toolCall.GetName()),
			Input: strings.TrimSpace(toolCall.GetInput()),
		})
	}

	return agent.SessionTimelineMessage{
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

func timelineTraceEventFromProto(event *handpb.SessionTimelineTraceEvent) (agent.SessionTimelineTraceEvent, error) {
	if event == nil {
		return agent.SessionTimelineTraceEvent{}, nil
	}

	var payload any
	if payloadJSON := strings.TrimSpace(event.GetPayloadJson()); payloadJSON != "" {
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return agent.SessionTimelineTraceEvent{}, err
		}
	}

	return agent.SessionTimelineTraceEvent{
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

func protoTimestampToTime(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

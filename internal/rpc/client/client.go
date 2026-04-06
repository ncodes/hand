package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"github.com/wandxy/hand/internal/storage"
)

type Client struct {
	conn   *grpc.ClientConn
	client handpb.HandServiceClient
}

type RespondOptions = agent.RespondOptions
type Event = agent.Event

type CompactSessionResult = agent.CompactSessionResult

type SessionContextStatus = agent.SessionContextStatus

type ChatAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
}

type SessionAPI interface {
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (string, error)
	CompactSession(context.Context, string) (CompactSessionResult, error)
	GetSession(context.Context, string) (SessionContextStatus, error)
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
					Channel: streamChannelToAgent(event.GetChannel()),
					Text:    event.GetText(),
				})
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

func streamChannelToAgent(channel handpb.RespondEvent_Channel) string {
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

	return fromSessionSummary(resp.GetSession()), nil
}

func (c *Client) ListSessions(ctx context.Context) ([]storage.Session, error) {
	resp, err := c.client.ListSessions(ctx, &handpb.ListSessionsRequest{})
	if err != nil {
		return nil, err
	}

	sessions := make([]storage.Session, 0, len(resp.GetSessions()))
	for _, session := range resp.GetSessions() {
		sessions = append(sessions, fromSessionSummary(session))
	}

	return sessions, nil
}

func (c *Client) UseSession(ctx context.Context, id string) error {
	_, err := c.client.UseSession(ctx, &handpb.UseSessionRequest{Id: strings.TrimSpace(id)})
	return err
}

func (c *Client) CurrentSession(ctx context.Context) (string, error) {
	resp, err := c.client.CurrentSession(ctx, &handpb.CurrentSessionRequest{})
	if err != nil {
		return "", err
	}

	return resp.GetId(), nil
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
		UpdatedAt:            fromTimestamp(resp.GetUpdatedAt()),
		CurrentContextLength: int(resp.GetCurrentContextLength()),
		TotalContextLength:   int(resp.GetTotalContextLength()),
	}, nil
}

func (c *Client) GetSession(ctx context.Context, id string) (SessionContextStatus, error) {
	resp, err := c.client.GetSession(ctx, &handpb.GetSessionRequest{
		Context: &handpb.GetSessionRequestContext{Id: strings.TrimSpace(id)},
	})
	if err != nil {
		return SessionContextStatus{}, err
	}
	cctx := resp.GetContext()
	if cctx == nil {
		return SessionContextStatus{}, fmt.Errorf("hand: get session response context is required")
	}

	return SessionContextStatus{
		SessionID:        resp.GetId(),
		Offset:           int(cctx.GetOffset()),
		Size:             int(resp.GetSize()),
		Length:           int(cctx.GetLength()),
		Used:             int(cctx.GetUsed()),
		Remaining:        int(cctx.GetRemaining()),
		UsedPct:          cctx.GetUsedPct(),
		RemainingPct:     cctx.GetRemainingPct(),
		CreatedAt:        fromTimestamp(resp.GetCreatedAt()),
		UpdatedAt:        fromTimestamp(resp.GetUpdatedAt()),
		CompactionStatus: resp.GetCompactionStatus(),
	}, nil
}

func (c *Client) SessionContextStatus(ctx context.Context, id string) (SessionContextStatus, error) {
	return c.GetSession(ctx, id)
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func fromSessionSummary(summary *handpb.SessionSummary) storage.Session {
	if summary == nil {
		return storage.Session{}
	}

	return storage.Session{
		ID:        summary.GetId(),
		UpdatedAt: time.Unix(summary.GetUpdatedAtUnix(), 0).UTC(),
	}
}

func fromTimestamp(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

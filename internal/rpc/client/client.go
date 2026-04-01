package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
	sessionstore "github.com/wandxy/hand/internal/session"
)

type Client struct {
	conn   *grpc.ClientConn
	client handpb.HandServiceClient
}

type RespondOptions struct {
	Instruct  string
	SessionID string
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
	resp, err := c.client.Chat(ctx, &handpb.ChatRequest{
		Message:   message,
		Instruct:  strings.TrimSpace(opts.Instruct),
		SessionId: strings.TrimSpace(opts.SessionID),
	})
	if err != nil {
		return "", err
	}

	return resp.Message, nil
}

func (c *Client) CreateSession(ctx context.Context, id string) (sessionstore.Session, error) {
	resp, err := c.client.CreateSession(ctx, &handpb.CreateSessionRequest{SessionId: strings.TrimSpace(id)})
	if err != nil {
		return sessionstore.Session{}, err
	}

	if resp.GetSession() == nil {
		return sessionstore.Session{}, nil
	}

	return fromSessionSummary(resp.GetSession()), nil
}

func (c *Client) ListSessions(ctx context.Context) ([]sessionstore.Session, error) {
	resp, err := c.client.ListSessions(ctx, &handpb.ListSessionsRequest{})
	if err != nil {
		return nil, err
	}

	sessions := make([]sessionstore.Session, 0, len(resp.GetSessions()))
	for _, session := range resp.GetSessions() {
		sessions = append(sessions, fromSessionSummary(session))
	}

	return sessions, nil
}

func (c *Client) UseSession(ctx context.Context, id string) error {
	_, err := c.client.UseSession(ctx, &handpb.UseSessionRequest{SessionId: strings.TrimSpace(id)})
	return err
}

func (c *Client) CurrentSession(ctx context.Context) (string, error) {
	resp, err := c.client.CurrentSession(ctx, &handpb.CurrentSessionRequest{})
	if err != nil {
		return "", err
	}

	return resp.GetSessionId(), nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func fromSessionSummary(summary *handpb.SessionSummary) sessionstore.Session {
	if summary == nil {
		return sessionstore.Session{}
	}

	return sessionstore.Session{
		ID:        summary.GetSessionId(),
		UpdatedAt: time.Unix(summary.GetUpdatedAtUnix(), 0).UTC(),
	}
}

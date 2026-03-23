package rpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wandxy/hand/internal/config"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type Client struct {
	conn   *grpc.ClientConn
	client handpb.HandServiceClient
}

// NewClient creates a new RPC client.
func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	target := fmt.Sprintf("%s:%d", cfg.RPCAddress, cfg.RPCPort)
	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		client: handpb.NewHandServiceClient(conn),
	}, nil
}

// Chat sends a chat message to the RPC server and returns the response.
func (c *Client) Chat(ctx context.Context, message string) (string, error) {
	resp, err := c.client.Chat(ctx, &handpb.ChatRequest{Message: message})
	if err != nil {
		return "", err
	}
	return resp.Message, nil
}

// Close closes the RPC client.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

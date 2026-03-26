package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type chatterStub struct {
	message  string
	instruct string
	reply    string
	err      error
}

func (s *chatterStub) Chat(_ context.Context, message string, opts agent.ChatOptions) (string, error) {
	s.message = message
	s.instruct = opts.Instruct
	return s.reply, s.err
}

func TestNewService_ReturnsService(t *testing.T) {
	require.NotNil(t, NewService(nil))
}

func TestService_ChatReturnsMessage(t *testing.T) {
	stub := &chatterStub{reply: "hello back"}
	svc := NewService(stub)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello", Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello", stub.message)
	require.Equal(t, "be terse", stub.instruct)
	require.Equal(t, "hello back", resp.Message)
}

func TestService_ChatReturnsHandlerError(t *testing.T) {
	stub := &chatterStub{err: errors.New("boom")}
	svc := NewService(stub)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.EqualError(t, err, "boom")
	require.Nil(t, resp)
}

func TestService_ChatRejectsNilRequest(t *testing.T) {
	svc := NewService(&chatterStub{})

	resp, err := svc.Chat(context.Background(), nil)

	require.EqualError(t, err, "chat request is required")
	require.Nil(t, resp)
}

func TestService_ChatRejectsMissingHandler(t *testing.T) {
	svc := NewService(nil)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.EqualError(t, err, "chat handler is required")
	require.Nil(t, resp)
}

func TestService_ChatRejectsNilReceiver(t *testing.T) {
	var svc *Service

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.EqualError(t, err, "service is required")
	require.Nil(t, resp)
}

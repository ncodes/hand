package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type handServiceClientStub struct {
	req  *handpb.ChatRequest
	resp *handpb.ChatResponse
	err  error
}

func (s *handServiceClientStub) Chat(_ context.Context, req *handpb.ChatRequest, _ ...grpc.CallOption) (*handpb.ChatResponse, error) {
	s.req = req
	return s.resp, s.err
}

func TestClient_ChatSendsInstruct(t *testing.T) {
	stub := &handServiceClientStub{resp: &handpb.ChatResponse{Message: "hello back"}}
	client := &Client{client: stub}

	reply, err := client.Chat(context.Background(), "hello", ChatOptions{Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, &handpb.ChatRequest{Message: "hello", Instruct: "be terse"}, stub.req)
}

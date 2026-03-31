package client

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type handServiceClientStub struct {
	req         *handpb.ChatRequest
	resp        *handpb.ChatResponse
	err         error
	createResp  *handpb.CreateSessionResponse
	createReq   *handpb.CreateSessionRequest
	listResp    *handpb.ListSessionsResponse
	listReq     *handpb.ListSessionsRequest
	useReq      *handpb.UseSessionRequest
	currentResp *handpb.CurrentSessionResponse
}

func (s *handServiceClientStub) Chat(_ context.Context, req *handpb.ChatRequest, _ ...grpc.CallOption) (*handpb.ChatResponse, error) {
	s.req = req
	return s.resp, s.err
}

func (s *handServiceClientStub) CreateSession(_ context.Context, req *handpb.CreateSessionRequest, _ ...grpc.CallOption) (*handpb.CreateSessionResponse, error) {
	s.createReq = req
	return s.createResp, s.err
}

func (s *handServiceClientStub) ListSessions(_ context.Context, req *handpb.ListSessionsRequest, _ ...grpc.CallOption) (*handpb.ListSessionsResponse, error) {
	s.listReq = req
	return s.listResp, s.err
}

func (s *handServiceClientStub) UseSession(_ context.Context, req *handpb.UseSessionRequest, _ ...grpc.CallOption) (*handpb.UseSessionResponse, error) {
	s.useReq = req
	return &handpb.UseSessionResponse{SessionId: req.SessionId}, s.err
}

func (s *handServiceClientStub) CurrentSession(context.Context, *handpb.CurrentSessionRequest, ...grpc.CallOption) (*handpb.CurrentSessionResponse, error) {
	return s.currentResp, s.err
}

func TestClient_ChatSendsInstruct(t *testing.T) {
	stub := &handServiceClientStub{resp: &handpb.ChatResponse{Message: "hello back"}}
	client := &Client{client: stub}

	reply, err := client.Chat(context.Background(), "hello", ChatOptions{Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, &handpb.ChatRequest{Message: "hello", Instruct: "be terse"}, stub.req)
}

func TestClient_ChatSendsSessionID(t *testing.T) {
	stub := &handServiceClientStub{resp: &handpb.ChatResponse{Message: "hello back"}}
	client := &Client{client: stub}

	_, err := client.Chat(context.Background(), "hello", ChatOptions{SessionID: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.req.GetSessionId())
}

func TestClient_CreateSessionReturnsSummary(t *testing.T) {
	stub := &handServiceClientStub{
		createResp: &handpb.CreateSessionResponse{
			Session: &handpb.SessionSummary{
				SessionId:     "project-a",
				UpdatedAtUnix: time.Unix(10, 0).UTC().Unix(),
			},
		},
	}
	client := &Client{client: stub}

	session, err := client.CreateSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", session.ID)
	require.Equal(t, "project-a", stub.createReq.GetSessionId())
}

func TestClient_ListSessionsReturnsItems(t *testing.T) {
	stub := &handServiceClientStub{
		listResp: &handpb.ListSessionsResponse{
			Sessions: []*handpb.SessionSummary{
				{SessionId: "default", UpdatedAtUnix: 10},
				{SessionId: "project-a", UpdatedAtUnix: 20},
			},
		},
	}
	client := &Client{client: stub}

	sessions, err := client.ListSessions(context.Background())

	require.NoError(t, err)
	require.NotNil(t, stub.listReq)
	require.Len(t, sessions, 2)
	require.Equal(t, "default", sessions[0].ID)
	require.Equal(t, "project-a", sessions[1].ID)
}

func TestClient_UseSessionSendsSessionID(t *testing.T) {
	stub := &handServiceClientStub{}
	client := &Client{client: stub}

	err := client.UseSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.useReq.GetSessionId())
}

func TestClient_CurrentSessionReturnsValue(t *testing.T) {
	stub := &handServiceClientStub{currentResp: &handpb.CurrentSessionResponse{SessionId: "project-a"}}
	client := &Client{client: stub}

	id, err := client.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, "project-a", id)
}

func TestNewClient_ValidatesOptions(t *testing.T) {
	_, err := NewClient(context.Background(), Options{})
	require.EqualError(t, err, "rpc address is required")

	_, err = NewClient(context.Background(), Options{Address: "127.0.0.1"})
	require.EqualError(t, err, "rpc port must be greater than zero")
}

func TestNewClient_CreatesConnection(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	client, err := NewClient(context.Background(), Options{
		Address: "127.0.0.1",
		Port:    lis.Addr().(*net.TCPAddr).Port,
	})
	require.NoError(t, err)
	require.NoError(t, client.Close())
}

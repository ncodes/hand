package client

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	protomock "github.com/wandxy/hand/internal/mocks/proto"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

func TestClient_RespondSendsInstruct(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Resp: &handpb.RespondResponse{Message: "hello back"}}
	client := &Client{client: stub}

	reply, err := client.Respond(context.Background(), "hello", RespondOptions{Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, &handpb.RespondRequest{Message: "hello", Instruct: "be terse"}, stub.Req)
}

func TestClient_RespondSendsSessionID(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Resp: &handpb.RespondResponse{Message: "hello back"}}
	client := &Client{client: stub}

	_, err := client.Respond(context.Background(), "hello", RespondOptions{SessionID: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.Req.GetId())
}

func TestClient_CreateSessionReturnsSummary(t *testing.T) {
	stub := &protomock.HandServiceClientStub{
		CreateResp: &handpb.CreateSessionResponse{
			Session: &handpb.SessionSummary{
				Id:            "project-a",
				UpdatedAtUnix: time.Unix(10, 0).UTC().Unix(),
			},
		},
	}
	client := &Client{client: stub}

	session, err := client.CreateSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", session.ID)
	require.Equal(t, "project-a", stub.CreateReq.GetId())
}

func TestClient_ListSessionsReturnsItems(t *testing.T) {
	stub := &protomock.HandServiceClientStub{
		ListResp: &handpb.ListSessionsResponse{
			Sessions: []*handpb.SessionSummary{
				{Id: "default", UpdatedAtUnix: 10},
				{Id: "project-a", UpdatedAtUnix: 20},
			},
		},
	}
	client := &Client{client: stub}

	sessions, err := client.ListSessions(context.Background())

	require.NoError(t, err)
	require.NotNil(t, stub.ListReq)
	require.Len(t, sessions, 2)
	require.Equal(t, "default", sessions[0].ID)
	require.Equal(t, "project-a", sessions[1].ID)
}

func TestClient_UseSessionSendsSessionID(t *testing.T) {
	stub := &protomock.HandServiceClientStub{}
	client := &Client{client: stub}

	err := client.UseSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.UseReq.GetId())
}

func TestClient_CurrentSessionReturnsValue(t *testing.T) {
	stub := &protomock.HandServiceClientStub{CurrentResp: &handpb.CurrentSessionResponse{Id: "project-a"}}
	client := &Client{client: stub}

	id, err := client.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, "project-a", id)
}

func TestClient_CompactSessionReturnsResult(t *testing.T) {
	now := time.Unix(123, 0).UTC()
	stub := &protomock.HandServiceClientStub{CompactResp: &handpb.CompactSessionResponse{
		Id:                   "project-a",
		SourceEndOffset:      12,
		SourceMessageCount:   20,
		UpdatedAt:            timestamppb.New(now),
		CurrentContextLength: 4000,
		TotalContextLength:   128000,
	}}
	client := &Client{client: stub}

	result, err := client.CompactSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.CompactReq.GetId())
	require.Equal(t, "project-a", result.SessionID)
	require.Equal(t, 12, result.SourceEndOffset)
	require.Equal(t, 20, result.SourceMessageCount)
	require.Equal(t, now, result.UpdatedAt)
	require.Equal(t, 4000, result.CurrentContextLength)
	require.Equal(t, 128000, result.TotalContextLength)
}

func TestClient_GetSessionReturnsResult(t *testing.T) {
	created := time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 4, 2, 11, 0, 0, 0, time.UTC)
	stub := &protomock.HandServiceClientStub{StatusResp: &handpb.GetSessionResponse{
		Id:               "project-a",
		Size:             20,
		CreatedAt:        timestamppb.New(created),
		UpdatedAt:        timestamppb.New(updated),
		CompactionStatus: "pending",
		Context: &handpb.GetSessionResponse_Context{
			Offset:       12,
			Length:       128000,
			Used:         64000,
			Remaining:    64000,
			UsedPct:      0.5,
			RemainingPct: 0.5,
		},
	}}
	client := &Client{client: stub}

	result, err := client.GetSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.StatusReq.GetContext().GetId())
	require.Equal(t, "project-a", result.SessionID)
	require.Equal(t, 12, result.Offset)
	require.Equal(t, 20, result.Size)
	require.Equal(t, 128000, result.Length)
	require.Equal(t, 64000, result.Used)
	require.Equal(t, 64000, result.Remaining)
	require.Equal(t, 0.5, result.UsedPct)
	require.Equal(t, 0.5, result.RemainingPct)
	require.True(t, created.Equal(result.CreatedAt))
	require.True(t, updated.Equal(result.UpdatedAt))
	require.Equal(t, "pending", result.CompactionStatus)
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

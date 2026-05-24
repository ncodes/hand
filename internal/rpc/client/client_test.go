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
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
	agent "github.com/wandxy/hand/pkg/agent"
)

func TestClient_RespondSendsInstruct(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "hello back", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	reply, err := client.Respond(context.Background(), "hello", RespondOptions{Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, &handpb.RespondRequest{Message: "hello", Instruct: "be terse"}, stub.Req)
}

func TestClient_RespondSendsSessionID(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "hello back", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	_, err := client.Respond(context.Background(), "hello", RespondOptions{SessionID: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.Req.GetId())
}

func TestClient_RespondStreamsTextDeltas(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "hello ", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "back", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	var events []Event
	reply, err := client.Respond(context.Background(), "hello", RespondOptions{
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, []Event{
		{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "hello "},
		{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "back"},
	}, events)
}

func TestClient_RespondRejectsStreamThatEndsBeforeDone(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "partial", Channel: handpb.RespondEvent_ASSISTANT},
	}}
	client := &Client{client: stub}

	reply, err := client.Respond(context.Background(), "hello", RespondOptions{})

	require.Equal(t, "partial", reply)
	require.EqualError(t, err, "respond stream ended before done event")
}

func TestClient_RespondIgnoresReasoningForFinalReplyAndExposesEvents(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "thinking", Channel: handpb.RespondEvent_REASONING},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "answer", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	var events []Event
	reply, err := client.Respond(context.Background(), "hello", RespondOptions{
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "answer", reply)
	require.Equal(t, []Event{
		{Kind: agent.EventKindTextDelta, Channel: "reasoning", Text: "thinking"},
		{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "answer"},
	}, events)
}

func TestClient_RespondExposesTraceEvents(t *testing.T) {
	timestamp := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{
			Type:             handpb.RespondEvent_TRACE_EVENT,
			TraceSessionId:   "default",
			TraceType:        trace.EvtInputSafetyBlocked,
			TracePayloadJson: `{"blocked":true,"findings":[{"id":"prompt_exfiltration"}]}`,
			Timestamp:        timestamppb.New(timestamp),
		},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "safe", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	var events []Event
	reply, err := client.Respond(context.Background(), "hello", RespondOptions{
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "safe", reply)
	require.Len(t, events, 2)
	require.Equal(t, agent.EventKindTrace, events[0].Kind)
	traceEvent, ok := events[0].TraceEvent.(*trace.Event)
	require.True(t, ok)
	require.NotNil(t, traceEvent)
	require.Equal(t, "default", traceEvent.SessionID)
	require.Equal(t, trace.EvtInputSafetyBlocked, traceEvent.Type)
	require.Equal(t, timestamp, traceEvent.Timestamp)
	require.Equal(t, map[string]any{
		"blocked": true,
		"findings": []any{
			map[string]any{"id": "prompt_exfiltration"},
		},
	}, traceEvent.Payload)
	require.Equal(t, Event{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "safe"}, events[1])
}

func TestClient_RespondIgnoresMalformedTraceEvents(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Events: []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TRACE_EVENT, TraceType: trace.EvtSessionFailed, TracePayloadJson: `{`},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "safe", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}}
	client := &Client{client: stub}

	var events []Event
	reply, err := client.Respond(context.Background(), "hello", RespondOptions{
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "safe", reply)
	require.Equal(t, []Event{{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "safe"}}, events)
}

func TestClient_CreateSessionReturnsSummary(t *testing.T) {
	stub := &protomock.HandServiceClientStub{
		CreateResp: &handpb.CreateSessionResponse{
			Session: &handpb.SessionSummary{
				Id:            "project-a",
				Title:         "Project Planning",
				TitleSource:   storage.SessionTitleSourceGenerated,
				UpdatedAtUnix: time.Unix(10, 0).UTC().Unix(),
			},
		},
	}
	client := &Client{client: stub}

	session, err := client.CreateSession(context.Background(), "project-a")

	require.NoError(t, err)
	require.Equal(t, "project-a", session.ID)
	require.Equal(t, "Project Planning", session.Title)
	require.Equal(t, storage.SessionTitleSourceGenerated, session.TitleSource)
	require.Equal(t, "project-a", stub.CreateReq.GetId())
}

func TestClient_ListSessionsReturnsItems(t *testing.T) {
	stub := &protomock.HandServiceClientStub{
		ListResp: &handpb.ListSessionsResponse{
			Sessions: []*handpb.SessionSummary{
				{Id: "default", Title: "Daily Planning", TitleSource: storage.SessionTitleSourceGenerated, UpdatedAtUnix: 10},
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
	require.Equal(t, "Daily Planning", sessions[0].Title)
	require.Equal(t, storage.SessionTitleSourceGenerated, sessions[0].TitleSource)
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
	stub := &protomock.HandServiceClientStub{CurrentResp: &handpb.CurrentSessionResponse{
		Id:          "project-a",
		Title:       "Project Planning",
		TitleSource: storage.SessionTitleSourceGenerated,
	}}
	client := &Client{client: stub}

	session, err := client.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, "project-a", session.ID)
	require.Equal(t, "Project Planning", session.Title)
	require.Equal(t, storage.SessionTitleSourceGenerated, session.TitleSource)
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

func TestClient_RepairSessionReturnsResult(t *testing.T) {
	stub := &protomock.HandServiceClientStub{RepairResp: &handpb.RepairSessionResponse{
		Type: handpb.RepairSessionRequest_VECTOR,
		Vector: &handpb.VectorRepairResponse{
			SessionsScanned: 2,
			MessagesScanned: 3,
			RowsScanned:     4,
			MissingRows:     5,
			StaleRows:       6,
			UnchangedRows:   7,
			RebuiltRows:     8,
			DeletedSources:  9,
			Batches:         10,
		},
	}}
	client := &Client{client: stub}

	result, err := client.RepairSession(context.Background(), RepairSessionOptions{
		SessionID: " project-a ",
		Full:      true,
	})

	require.NoError(t, err)
	require.Equal(t, handpb.RepairSessionRequest_VECTOR, stub.RepairReq.GetType())
	require.Equal(t, "project-a", stub.RepairReq.GetVector().GetId())
	require.True(t, stub.RepairReq.GetVector().GetFull())
	require.Equal(t, 2, result.SessionsScanned)
	require.Equal(t, 3, result.MessagesScanned)
	require.Equal(t, 4, result.RowsScanned)
	require.Equal(t, 5, result.MissingRows)
	require.Equal(t, 6, result.StaleRows)
	require.Equal(t, 7, result.UnchangedRows)
	require.Equal(t, 8, result.RebuiltRows)
	require.Equal(t, 9, result.DeletedSources)
	require.Equal(t, 10, result.Batches)
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

func TestClient_GetSessionTimelineReturnsResult(t *testing.T) {
	messageAt := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	traceAt := time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC)
	stub := &protomock.HandServiceClientStub{TimelineResp: &handpb.GetSessionTimelineResponse{
		Id:          "default",
		Title:       "Daily Planning",
		TitleSource: storage.SessionTitleSourceGenerated,
		Messages: []*handpb.SessionTimelineMessage{{
			Offset:     2,
			Id:         7,
			Role:       "tool",
			Name:       "read_file",
			ToolCallId: "call_1",
			Content:    "file content",
			CreatedAt:  timestamppb.New(messageAt),
			ToolCalls: []*handpb.SessionTimelineToolCall{{
				Id:    "call_2",
				Name:  "search",
				Input: `{"query":"hello"}`,
			}},
		}},
		TraceEvents: []*handpb.SessionTimelineTraceEvent{{
			Id:          9,
			Sequence:    3,
			Type:        trace.EvtInputSafetyBlocked,
			Timestamp:   timestamppb.New(traceAt),
			PayloadJson: `{"blocked":true}`,
		}},
		MessagesHasMore:       true,
		TracesHasMore:         true,
		TracesTruncatedBefore: true,
		FirstTraceSequence:    3,
		LastTraceSequence:     3,
	}}
	client := &Client{client: stub}

	result, err := client.GetSessionTimeline(context.Background(), agent.SessionTimelineOptions{
		SessionID:     " default ",
		MessageOffset: 2,
		MessageLimit:  1,
		TraceOffset:   3,
		TraceLimit:    4,
	})

	require.NoError(t, err)
	require.Equal(t, "default", stub.TimelineReq.GetId())
	require.EqualValues(t, 2, stub.TimelineReq.GetMessageOffset())
	require.EqualValues(t, 1, stub.TimelineReq.GetMessageLimit())
	require.EqualValues(t, 3, stub.TimelineReq.GetTraceOffset())
	require.EqualValues(t, 4, stub.TimelineReq.GetTraceLimit())
	require.Equal(t, "default", result.SessionID)
	require.Equal(t, "Daily Planning", result.Title)
	require.Equal(t, storage.SessionTitleSourceGenerated, result.TitleSource)
	require.True(t, result.MessagesHasMore)
	require.True(t, result.TracesHasMore)
	require.True(t, result.TracesTruncatedBefore)
	require.Equal(t, 3, result.FirstTraceSequence)
	require.Equal(t, 3, result.LastTraceSequence)
	require.Len(t, result.Messages, 1)
	require.Equal(t, 2, result.Messages[0].Offset)
	require.EqualValues(t, 7, result.Messages[0].Message.ID)
	require.Equal(t, "tool", string(result.Messages[0].Message.Role))
	require.Equal(t, "read_file", result.Messages[0].Message.Name)
	require.Equal(t, "call_1", result.Messages[0].Message.ToolCallID)
	require.Equal(t, "file content", result.Messages[0].Message.Content)
	require.Equal(t, messageAt, result.Messages[0].Message.CreatedAt)
	require.Len(t, result.Messages[0].Message.ToolCalls, 1)
	require.Equal(t, "call_2", result.Messages[0].Message.ToolCalls[0].ID)
	require.Equal(t, "search", result.Messages[0].Message.ToolCalls[0].Name)
	require.Equal(t, `{"query":"hello"}`, result.Messages[0].Message.ToolCalls[0].Input)
	require.Len(t, result.TraceEvents, 1)
	require.EqualValues(t, 9, result.TraceEvents[0].Event.ID)
	require.Equal(t, 3, result.TraceEvents[0].Event.Sequence)
	require.Equal(t, trace.EvtInputSafetyBlocked, result.TraceEvents[0].Event.Type)
	require.Equal(t, traceAt, result.TraceEvents[0].Event.Timestamp)
	require.Equal(t, map[string]any{"blocked": true}, result.TraceEvents[0].Event.Payload)
}

func TestClient_GetSessionTimelineReturnsDecodeErrors(t *testing.T) {
	client := &Client{client: &protomock.HandServiceClientStub{}}

	_, err := client.GetSessionTimeline(context.Background(), agent.SessionTimelineOptions{})
	require.EqualError(t, err, "hand: get session timeline response is required")

	client = &Client{client: &protomock.HandServiceClientStub{TimelineResp: &handpb.GetSessionTimelineResponse{
		TraceEvents: []*handpb.SessionTimelineTraceEvent{{PayloadJson: "{"}},
	}}}
	_, err = client.GetSessionTimeline(context.Background(), agent.SessionTimelineOptions{})
	require.Error(t, err)
}

func TestClient_GetSessionTimelineReturnsRPCError(t *testing.T) {
	stub := &protomock.HandServiceClientStub{Err: context.Canceled}
	client := &Client{client: stub}

	_, err := client.GetSessionTimeline(context.Background(), agent.SessionTimelineOptions{})

	require.ErrorIs(t, err, context.Canceled)
}

func TestTimelineProtoAdaptersHandleNilRecords(t *testing.T) {
	message := timelineMessageFromProto(nil)
	require.Zero(t, message)

	event, err := timelineTraceEventFromProto(nil)
	require.NoError(t, err)
	require.Zero(t, event)
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

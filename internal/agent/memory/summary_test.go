package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/storage"
	storagemock "github.com/wandxy/hand/internal/storage/mock"
	"github.com/wandxy/hand/internal/trace"
)

func TestMemory_MaybeRefreshSummary_ReturnsWhenMemoryOrTraceIsNil(t *testing.T) {
	var mem *Memory
	require.NoError(t, mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		TraceSession: &mocks.TraceSessionStub{},
	}))

	mem = &Memory{}
	require.NoError(t, mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{}))
}

func TestMemory_MaybeRefreshSummary_SkipsWhenCompactionIsDisabled(t *testing.T) {
	client := &mocks.ModelClientStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(false),
		ModelClient:    client,
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestMemory_MaybeRefreshSummary_SkipsWhenHistoryIsTooShort(t *testing.T) {
	client := &mocks.ModelClientStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    client,
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(RecentSessionTail),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestMemory_MaybeRefreshSummary_SkipsWhenEstimateDoesNotTrigger(t *testing.T) {
	client := &mocks.ModelClientStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    client,
		Request:        models.Request{Instructions: "small"},
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestMemory_MaybeRefreshSummary_SkipsWhenSummaryAlreadyCoversHistory(t *testing.T) {
	client := &mocks.ModelClientStub{}
	mem := &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 2,
		SessionSummary:  "covered",
	}}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    client,
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestMemory_MaybeRefreshSummary_SkipsWhenExistingSummaryAlreadyCoversTargetOffset(t *testing.T) {
	client := &mocks.ModelClientStub{}
	mem := &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 99,
	}}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    client,
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestMemory_MaybeRefreshSummary_RecordsFailureWhenModelCallFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    &mocks.ModelClientStub{Err: errors.New("summary failed")},
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   traceSession,
	})
	require.EqualError(t, err, "summary failed")

	requireSummaryEvent(t, traceSession.Events, "context.summary.failed")
}

func TestMemory_MaybeRefreshSummary_RecordsFailureWhenModelReturnsNil(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    &mocks.ModelClientStub{Responses: []*models.Response{nil}},
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   traceSession,
	})
	require.EqualError(t, err, "model response is required")

	requireSummaryEvent(t, traceSession.Events, "context.summary.failed")
}

func TestMemory_MaybeRefreshSummary_RecordsFailureWhenSummaryRequestsTools(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    &mocks.ModelClientStub{Responses: []*models.Response{{RequiresToolCalls: true}}},
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   traceSession,
	})
	require.EqualError(t, err, "summary requested tool calls")

	requireSummaryEvent(t, traceSession.Events, "context.summary.failed")
}

func TestMemory_MaybeRefreshSummary_RecordsFailureWhenModelReturnsInvalidSummary(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "{"}}},
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore:   &storagemock.SessionStore{},
		TraceSession:   traceSession,
	})
	require.Error(t, err)

	require.Nil(t, mem.Summary)
	requireSummaryEvent(t, traceSession.Events, "context.summary.failed")
}

func TestMemory_MaybeRefreshSummary_RecordsFailureWhenSavingSummaryFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config: summaryTestConfig(true),
		ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: `
        {"session_summary":"Older work","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`}}},
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore: &storagemock.SessionStore{
			SaveSummaryFunc: func(context.Context, storage.SessionSummary) error {
				return errors.New("save summary failed")
			},
		},
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "save summary failed")

	requireSummaryEvent(t, traceSession.Events, "context.summary.failed")
}

func TestMemory_MaybeRefreshSummary_SavesSummaryAndRecordsTrace(t *testing.T) {
	requestedAt := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{
			OutputText: `{"session_summary":"Older work","current_task":"Fix tests",
            "discoveries":["one"],"open_questions":["two"],"next_actions":["three"]}`,
		}},
	}
	mem := &Memory{
		Summary: &SummaryState{
			SessionID:          storage.DefaultSessionID,
			SourceEndOffset:    1,
			SourceMessageCount: 9,
			UpdatedAt:          requestedAt.Add(-time.Hour),
			SessionSummary:     "Earlier work",
			CurrentTask:        "Continue",
			Discoveries:        []string{"prior"},
		},
	}

	var saved storage.SessionSummary
	err := mem.MaybeRefreshSummary(context.Background(), SummaryRefreshInput{
		Config:         summaryTestConfig(true),
		ModelClient:    client,
		Now:            func() time.Time { return requestedAt },
		Request:        summaryTriggerRequest(),
		SessionHistory: summaryTestHistory(10),
		SessionID:      storage.DefaultSessionID,
		SummaryStore: &storagemock.SessionStore{
			SaveSummaryFunc: func(_ context.Context, summary storage.SessionSummary) error {
				saved = summary
				return nil
			},
		},
		TraceSession: traceSession,
	})
	require.NoError(t, err)

	require.Equal(t, 1, client.CallCount)
	require.Len(t, client.Requests, 1)
	require.Equal(t, instruct.BuildSessionSummary().String(), client.Requests[0].Instructions)
	require.Len(t, client.Requests[0].Messages, 2)
	require.Equal(t, handmsg.RoleDeveloper, client.Requests[0].Messages[0].Role)
	require.Contains(t, client.Requests[0].Messages[0].Content, "Session Summary:\nEarlier work")
	require.Equal(t, "history", client.Requests[0].Messages[1].Content)

	require.Equal(t, storage.DefaultSessionID, saved.SessionID)
	require.Equal(t, 2, saved.SourceEndOffset)
	require.Equal(t, 10, saved.SourceMessageCount)
	require.Equal(t, requestedAt, saved.UpdatedAt)
	require.Equal(t, "Older work", saved.SessionSummary)
	require.Equal(t, "Fix tests", saved.CurrentTask)
	require.Equal(t, []string{"one"}, saved.Discoveries)
	require.Equal(t, []string{"two"}, saved.OpenQuestions)
	require.Equal(t, []string{"three"}, saved.NextActions)

	require.NotNil(t, mem.Summary)
	require.Equal(t, saved.SessionSummary, mem.Summary.SessionSummary)
	requireSummaryEvent(t, traceSession.Events, "context.summary.requested")
	requireSummaryEvent(t, traceSession.Events, "context.summary.saved")
}

func TestMemory_RecordSummaryApplied_ReturnsWhenUnavailable(t *testing.T) {
	(&Memory{}).RecordSummaryApplied(nil)
	(&Memory{}).RecordSummaryApplied(&mocks.TraceSessionStub{})
	(*Memory)(nil).RecordSummaryApplied(&mocks.TraceSessionStub{})
}

func TestMemory_RecordSummaryApplied_SkipsBlankSummary(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{Summary: &SummaryState{SessionID: storage.DefaultSessionID, SessionSummary: "   "}}

	mem.RecordSummaryApplied(traceSession)
	require.Empty(t, traceSession.Events)
}

func TestMemory_RecordSummaryApplied_RecordsEvent(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	updatedAt := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	mem := &Memory{Summary: &SummaryState{
		SessionID:          storage.DefaultSessionID,
		SourceEndOffset:    2,
		SourceMessageCount: 10,
		UpdatedAt:          updatedAt,
		SessionSummary:     "Older work",
	}}

	mem.RecordSummaryApplied(traceSession)

	require.Len(t, traceSession.Events, 1)
	require.Equal(t, "context.summary.applied", traceSession.Events[0].Type)
	require.Equal(t, map[string]any{
		"session_id":           storage.DefaultSessionID,
		"source_end_offset":    2,
		"source_message_count": 10,
		"updated_at":           updatedAt,
	}, traceSession.Events[0].Payload)
}

func TestSummaryFromStorage_ReturnsNilWhenRequiredFieldsMissing(t *testing.T) {
	require.Nil(t, SummaryFromStorage(storage.SessionSummary{}))
	require.Nil(t, SummaryFromStorage(storage.SessionSummary{SessionID: "ses_test", SessionSummary: ""}))
}

func TestSummaryFromStorage_ClonesStorageSummary(t *testing.T) {
	stored := storage.SessionSummary{
		SessionID:          "ses_test",
		SourceEndOffset:    2,
		SourceMessageCount: 5,
		UpdatedAt:          time.Now().UTC(),
		SessionSummary:     "Older work",
		CurrentTask:        "Fix tests",
		Discoveries:        []string{"one"},
		OpenQuestions:      []string{"two"},
		NextActions:        []string{"three"},
	}

	loaded := SummaryFromStorage(stored)
	require.NotNil(t, loaded)
	require.Equal(t, stored.SessionID, loaded.SessionID)
	require.Equal(t, stored.SessionSummary, loaded.SessionSummary)

	stored.Discoveries[0] = "changed"
	require.Equal(t, "one", loaded.Discoveries[0])
}

func TestParseSummary_RejectsEmptyRaw(t *testing.T) {
	summary, err := parseSummary(storage.DefaultSessionID, 1, 2, "   ", time.Now().UTC())
	require.Nil(t, summary)
	require.EqualError(t, err, "summary response is empty")
}

func TestParseSummary_RejectsInvalidJSON(t *testing.T) {
	summary, err := parseSummary(storage.DefaultSessionID, 1, 2, "{", time.Now().UTC())
	require.Nil(t, summary)
	require.Error(t, err)
}

func TestParseSummary_RejectsMissingSessionSummary(t *testing.T) {
	summary, err := parseSummary(
		storage.DefaultSessionID,
		1,
		2,
		`{"session_summary":"","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`,
		time.Now().UTC(),
	)
	require.Nil(t, summary)
	require.EqualError(t, err, "session summary is required")
}

func TestParseSummary_StripsMarkdownFenceBeforeParsing(t *testing.T) {
	summary, err := parseSummary(
		storage.DefaultSessionID,
		1,
		2,
		"```json\n{\"session_summary\":\"done\",\"current_task\":\"next\",\"discoveries\":[\"one\"],\"open_questions\":[\"two\"],\"next_actions\":[\"three\"]}\n```",
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, "done", summary.SessionSummary)
	require.Equal(t, "next", summary.CurrentTask)
}

func TestStripMarkdownFence_HandlesFenceVariants(t *testing.T) {
	require.Equal(t, `{"a":1}`, stripMarkdownFence("```json\n{\"a\":1}\n```"))
	require.Equal(t, `{"a":1}`, stripMarkdownFence("```JSON\n{\"a\":1}\n```"))
	require.Equal(t, `{"a":1}`, stripMarkdownFence("```\n{\"a\":1}\n```"))
	require.Equal(t, `plain`, stripMarkdownFence("plain"))
}

func TestRenderSummaryList_TrimsEmptyValues(t *testing.T) {
	require.Equal(t, "", renderSummaryList("Discoveries", nil))
	require.Equal(t, "", renderSummaryList("Discoveries", []string{" ", "\t"}))
	require.Equal(t, "Discoveries:\n- one\n- two", renderSummaryList("Discoveries", []string{" one ", "", "two"}))
}

func TestSummaryCompactionEnabled_DefaultsAndUsesConfiguredValue(t *testing.T) {
	require.True(t, summaryCompactionEnabled(nil))
	require.True(t, summaryCompactionEnabled(&config.Config{}))

	require.False(t, summaryCompactionEnabled(&config.Config{CompactionEnabled: new(false)}))
}

func TestSummaryCompactionEvaluator_UsesConfigValues(t *testing.T) {
	require.NotNil(t, summaryCompactionEvaluator(nil))
	require.NotNil(t, summaryCompactionEvaluator(&config.Config{
		ModelContextLength:       100,
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	}))
}

func summaryTestConfig(enabled bool) *config.Config {
	return &config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ModelContextLength:       100,
		CompactionEnabled:        &enabled,
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	}
}

func summaryTriggerRequest() models.Request {
	return models.Request{
		Instructions: "summary",
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
		}},
	}
}

func summaryTestHistory(count int) []handmsg.Message {
	history := make([]handmsg.Message, 0, count)
	for i := 0; i < count; i++ {
		history = append(history, handmsg.Message{
			Role:      handmsg.RoleUser,
			Content:   "history",
			CreatedAt: time.Now().UTC(),
		})
	}

	return history
}

func requireSummaryEvent(t *testing.T, events []trace.Event, eventType string) {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return
		}
	}

	require.Fail(t, "missing trace event", eventType)
}

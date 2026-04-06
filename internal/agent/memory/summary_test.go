package memory

import (
	"context"
	"errors"
	"io"
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
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestService_MaybeRefreshMemory_ReturnsWhenMemoryOrTraceIsNil(t *testing.T) {
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
	var mem *Memory
	require.NoError(t, service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		TraceSession: &mocks.TraceSessionStub{},
	}))

	mem = &Memory{}
	require.NoError(t, service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{}))
}

func TestService_MaybeRefreshMemory_ReturnsErrorWhenServiceDependenciesMissing(t *testing.T) {
	mem := &Memory{}
	require.EqualError(t, (*Service)(nil).MaybeRefreshMemory(context.Background(), mem, RefreshInput{TraceSession: &mocks.TraceSessionStub{}}), "memory service is required")
	require.EqualError(t, (&Service{store: &storagemock.SessionStore{}}).MaybeRefreshMemory(context.Background(), mem, RefreshInput{TraceSession: &mocks.TraceSessionStub{}}), "model client is required")
	require.EqualError(t, (&Service{modelClient: &mocks.ModelClientStub{}}).MaybeRefreshMemory(context.Background(), mem, RefreshInput{TraceSession: &mocks.TraceSessionStub{}}), "summary store is required")
}

func TestService_Load(t *testing.T) {
	store := &storagemock.SessionStore{
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{
				SessionID:      storage.DefaultSessionID,
				SessionSummary: "Older work",
			}, true, nil
		},
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)

	mem, err := service.Load(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.NotNil(t, mem)
	require.NotNil(t, mem.Summary)
	require.Equal(t, "Older work", mem.Summary.SessionSummary)
}

func TestService_Load_ReturnsErrors(t *testing.T) {
	_, err := (*Service)(nil).Load(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "memory service is required")

	_, err = (&Service{}).Load(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "summary store is required")

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{}, false, errors.New("load failed")
		},
	})
	_, err = service.Load(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "load failed")
}

func TestService_MaybeRefreshMemory_SkipsWhenCompactionIsDisabled(t *testing.T) {
	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(false), client, summaryTestStore(summaryTestHistory(10)))
	mem := &Memory{}
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestService_MaybeRefreshMemory_DoesNotTransitionCompactionWhenRefreshIsNotNeeded(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *config.Config
		history []handmsg.Message
		memory  *Memory
		request models.Request
	}{
		{
			name:    "history too short",
			cfg:     summaryTestConfig(true),
			history: summaryTestHistory(RecentSessionTail),
			memory:  &Memory{},
			request: summaryTriggerRequest(),
		},
		{
			name:    "estimate does not trigger",
			cfg:     summaryTestConfig(true),
			history: summaryTestHistory(10),
			memory:  &Memory{},
			request: models.Request{Instructions: "small"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := summaryTestStore(tc.history)
			service := summaryTestService(tc.cfg, &mocks.ModelClientStub{}, store)

			err := service.MaybeRefreshMemory(context.Background(), tc.memory, RefreshInput{
				Request:      tc.request,
				SessionID:    storage.DefaultSessionID,
				TraceSession: &mocks.TraceSessionStub{},
			})
			require.NoError(t, err)

			session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, storage.SessionCompaction{}, session.Compaction)
		})
	}
}

func TestService_MaybeRefreshMemory_ReturnsCountMessagesError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 0, errors.New("count failed")
		},
	})

	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "count failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenLoadingSessionFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	store.GetFunc = func(context.Context, string) (storage.Session, bool, error) {
		return storage.Session{}, false, errors.New("get session failed")
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)

	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "get session failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenSessionIsMissing(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	store.GetFunc = func(context.Context, string) (storage.Session, bool, error) {
		return storage.Session{}, false, nil
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)

	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "session not found")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_SkipsWhenHistoryIsTooShort(t *testing.T) {
	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(true), client, summaryTestStore(summaryTestHistory(RecentSessionTail)))
	mem := &Memory{}
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestService_MaybeRefreshMemory_SkipsWhenEstimateDoesNotTrigger(t *testing.T) {
	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(true), client, summaryTestStore(summaryTestHistory(10)))
	mem := &Memory{}
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      models.Request{Instructions: "small"},
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestService_MaybeRefreshMemory_SkipsWhenSummaryAlreadyCoversHistory(t *testing.T) {
	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(true), client, &storagemock.SessionStore{})
	mem := &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 2,
		SessionSummary:  "covered",
	}}
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestService_MaybeRefreshMemory_SkipsWhenExistingSummaryAlreadyCoversTargetOffset(t *testing.T) {
	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(true), client, summaryTestStore(summaryTestHistory(10)))
	mem := &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 99,
	}}
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)
}

func TestService_MaybeRefreshMemory_ReconcilesStaleRunningStateWhenSummaryAlreadyCoversTarget(t *testing.T) {
	store := summaryTestStore(summaryTestHistory(10))
	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	session.Compaction = storage.SessionCompaction{
		RequestedAt:        time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		StartedAt:          time.Date(2026, 4, 3, 9, 1, 0, 0, time.UTC),
		Status:             storage.CompactionStatusRunning,
		TargetMessageCount: 10,
		TargetOffset:       2,
	}
	require.NoError(t, store.Save(context.Background(), session))

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)
	mem := &Memory{Summary: &SummaryState{
		SessionID:          storage.DefaultSessionID,
		SourceEndOffset:    99,
		SourceMessageCount: 99,
		SessionSummary:     "covered",
	}}
	traceSession := &mocks.TraceSessionStub{}

	err = service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)

	session, ok, err = store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, session.Compaction.Status)
	require.Equal(t, 2, session.Compaction.TargetOffset)
	require.Equal(t, 10, session.Compaction.TargetMessageCount)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionSucceeded)
}

func TestService_MaybeRefreshMemory_ReconcilesCoveredSummaryWithoutTriggeringEstimate(t *testing.T) {
	store := summaryTestStore(summaryTestHistory(10))
	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	session.Compaction = storage.SessionCompaction{
		RequestedAt:        time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		StartedAt:          time.Date(2026, 4, 3, 9, 1, 0, 0, time.UTC),
		Status:             storage.CompactionStatusRunning,
		TargetMessageCount: 10,
		TargetOffset:       2,
	}
	require.NoError(t, store.Save(context.Background(), session))

	client := &mocks.ModelClientStub{}
	service := summaryTestService(summaryTestConfig(true), client, store)
	mem := &Memory{Summary: &SummaryState{
		SessionID:          storage.DefaultSessionID,
		SourceEndOffset:    99,
		SourceMessageCount: 99,
		SessionSummary:     "covered",
	}}
	traceSession := &mocks.TraceSessionStub{}

	err = service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      models.Request{Instructions: "small"},
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)
	require.Zero(t, client.CallCount)

	session, ok, err = store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, session.Compaction.Status)
	require.Equal(t, 2, session.Compaction.TargetOffset)
	require.Equal(t, 10, session.Compaction.TargetMessageCount)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionSucceeded)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenCoveredSummarySessionLoadFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 10, nil
		},
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("get covered session failed")
		},
	})

	err := service.MaybeRefreshMemory(context.Background(), &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 99,
		SessionSummary:  "covered",
	}}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "get covered session failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenCoveredSummarySessionIsMissing(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 10, nil
		},
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, nil
		},
	})

	err := service.MaybeRefreshMemory(context.Background(), &Memory{Summary: &SummaryState{
		SessionID:       storage.DefaultSessionID,
		SourceEndOffset: 99,
		SessionSummary:  "covered",
	}}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "session not found")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenModelCallFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Err: errors.New("summary failed")}, summaryTestStore(summaryTestHistory(10)))
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "summary failed")

	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenLoadingSummaryMessagesFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	store.GetMessagesFunc = func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
		return nil, errors.New("get messages failed")
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)

	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "get messages failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenModelReturnsNil(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Responses: []*models.Response{nil}}, summaryTestStore(summaryTestHistory(10)))
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "model response is required")

	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenSummaryRequestsTools(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Responses: []*models.Response{{RequiresToolCalls: true}}}, summaryTestStore(summaryTestHistory(10)))
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "summary requested tool calls")

	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_FallsBackWhenStructuredOutputRequestFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	client := &mocks.ModelClientStub{
		Errors: []error{errors.New("structured outputs unsupported")},
		Responses: []*models.Response{
			nil,
			{
				OutputText: `{
				"session_summary": "Older work",
				"current_task": "Fix tests",
				"discoveries": ["one"],
				"open_questions": ["two"],
				"next_actions": ["three"]
			}`,
			}},
	}
	service := summaryTestService(summaryTestConfig(true), client, summaryTestStore(summaryTestHistory(10)))

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)
	require.Len(t, client.Requests, 2)
	require.NotNil(t, client.Requests[0].StructuredOutput)
	require.Nil(t, client.Requests[1].StructuredOutput)
	require.NotNil(t, mem.Summary)
	require.Equal(t, "Older work", mem.Summary.SessionSummary)
}

func TestService_MaybeRefreshMemory_UsesSummaryModelWhenConfigured(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	client := &mocks.ModelClientStub{
		Errors: []error{errors.New("structured outputs unsupported")},
		Responses: []*models.Response{
			nil,
			{
				OutputText: `{
				"session_summary": "Older work",
				"current_task": "Fix tests",
				"discoveries": ["one"],
				"open_questions": ["two"],
				"next_actions": ["three"]
			}`,
			},
		},
	}
	cfg := summaryTestConfig(true)
	cfg.Model = "openai/gpt-4o-mini"
	cfg.SummaryModel = "anthropic/claude-3.5-haiku"
	service := NewService(cfg, client, summaryTestStore(summaryTestHistory(10)))

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)
	require.Len(t, client.Requests, 2)
	require.Equal(t, "anthropic/claude-3.5-haiku", client.Requests[0].Model)
	require.Equal(t, "anthropic/claude-3.5-haiku", client.Requests[1].Model)
}

func TestService_generateSummaryResponse_ValidationAndFallbackPaths(t *testing.T) {
	t.Run("nil_service", func(t *testing.T) {
		resp, err := (*Service)(nil).generateSummaryResponse(context.Background(), models.Request{})
		require.Nil(t, resp)
		require.EqualError(t, err, "model client is required")
	})

	t.Run("nil_client", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), nil, summaryTestStore(summaryTestHistory(10)))
		resp, err := service.generateSummaryResponse(context.Background(), models.Request{})
		require.Nil(t, resp)
		require.EqualError(t, err, "model client is required")
	})

	t.Run("returns_original_error_without_structured_output", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Err: errors.New("chat failed")}, summaryTestStore(summaryTestHistory(10)))
		resp, err := service.generateSummaryResponse(context.Background(), models.Request{})
		require.Nil(t, resp)
		require.EqualError(t, err, "chat failed")
	})
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenModelReturnsInvalidSummary(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "{"}}}, summaryTestStore(summaryTestHistory(10)))
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)

	require.NotNil(t, mem.Summary)
	require.Equal(t, "{", mem.Summary.SessionSummary)
	require.Empty(t, mem.Summary.CurrentTask)
	require.Nil(t, mem.Summary.Discoveries)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryParseFailed)
	requireSummaryEventAbsent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenModelReturnsEmptySummary(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "```json\n \n```"}},
	}, summaryTestStore(summaryTestHistory(10)))

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "summary response is empty")
	require.Nil(t, mem.Summary)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
	requireSummaryEventAbsent(t, traceSession.Events, trace.EvtSummaryParseFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenFallbackSummaryConstructionFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "not-json"}},
	}, summaryTestStore(summaryTestHistory(10)))

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    "", // empty session ID should be rejected
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "session summary is required")
	require.Nil(t, mem.Summary)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryParseFailed)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsFailureWhenSavingSummaryFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{
		Summary: &SummaryState{
			SessionID:          storage.DefaultSessionID,
			SourceEndOffset:    1,
			SourceMessageCount: 9,
			UpdatedAt:          time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC),
			SessionSummary:     "Earlier work",
		},
	}
	store := summaryTestStore(summaryTestHistory(10))
	store.SaveSummaryFunc = func(context.Context, storage.SessionSummary) error {
		return errors.New("save summary failed")
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: `
		{"session_summary":"Older work","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`}}}, store)
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "save summary failed")

	require.NotNil(t, mem.Summary)
	require.Equal(t, "Earlier work", mem.Summary.SessionSummary)
	require.Equal(t, 1, mem.Summary.SourceEndOffset)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
}

func TestService_MaybeRefreshMemory_RecordsParseFailureBeforeFallbackSaveFailure(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	mem := &Memory{
		Summary: &SummaryState{
			SessionID:          storage.DefaultSessionID,
			SourceEndOffset:    1,
			SourceMessageCount: 9,
			UpdatedAt:          time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC),
			SessionSummary:     "Earlier work",
		},
	}
	store := summaryTestStore(summaryTestHistory(10))
	store.SaveSummaryFunc = func(context.Context, storage.SessionSummary) error {
		return errors.New("save summary failed")
	}
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "## Summary\nKeep moving"}},
	}, store)

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})

	require.EqualError(t, err, "save summary failed")
	require.NotNil(t, mem.Summary)
	require.Equal(t, "Earlier work", mem.Summary.SessionSummary)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryParseFailed)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryFailed)
	requireSummaryEventAbsent(t, traceSession.Events, trace.EvtSummarySaved)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenPendingTransitionFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	store.SaveFunc = func(context.Context, storage.Session) error {
		return errors.New("save pending failed")
	}

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)
	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "save pending failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenLifecycleSaveFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	saveCalls := 0
	store.SaveFunc = func(_ context.Context, saved storage.Session) error {
		saveCalls++
		if saveCalls == 2 {
			return errors.New("save running failed")
		}
		return nil
	}

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)
	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "save running failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionPending)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_ReturnsWrappedErrorWhenMarkingCompactionFailedFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	saveCalls := 0
	store.SaveFunc = func(_ context.Context, saved storage.Session) error {
		saveCalls++
		if saveCalls == 3 {
			return errors.New("save failed state failed")
		}
		return nil
	}

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Err: errors.New("summary failed")}, store)
	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "mark compaction failed: save failed state failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_RecordsCompactionFailureWhenSucceededTransitionFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	saveCalls := 0
	store.SaveFunc = func(_ context.Context, saved storage.Session) error {
		saveCalls++
		if saveCalls == 3 {
			return errors.New("save succeeded failed")
		}
		return nil
	}

	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{
		Responses: []*models.Response{{
			OutputText: `{"session_summary":"Older work","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`,
		}},
	}, store)
	err := service.MaybeRefreshMemory(context.Background(), &Memory{}, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "save succeeded failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_SavesSummaryAndRecordsTrace(t *testing.T) {
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
	store := summaryTestStore(summaryTestHistory(10))
	store.SaveSummaryFunc = func(_ context.Context, summary storage.SessionSummary) error {
		saved = summary
		return nil
	}
	service := summaryTestService(summaryTestConfig(true), client, store)
	service.now = func() time.Time { return requestedAt }
	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.NoError(t, err)

	require.Equal(t, 1, client.CallCount)
	require.Len(t, client.Requests, 1)
	require.Contains(t, client.Requests[0].Instructions, instruct.BuildSessionSummary().String())
	require.Contains(t, client.Requests[0].Instructions, "Session Summary:\nEarlier work")
	require.Len(t, client.Requests[0].Messages, 1)
	require.Equal(t, "history", client.Requests[0].Messages[0].Content)

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
	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, session.Compaction.Status)
	require.Equal(t, 2, session.Compaction.TargetOffset)
	require.Equal(t, 10, session.Compaction.TargetMessageCount)
	require.Empty(t, session.Compaction.LastError)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryRequested)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummarySaved)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionPending)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionRunning)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionSucceeded)
}

func TestService_MaybeRefreshMemory_MarksCompactionFailedAndRetries(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	client := &mocks.ModelClientStub{Err: errors.New("summary failed")}
	service := summaryTestService(summaryTestConfig(true), client, store)
	mem := &Memory{}

	err := service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: traceSession,
	})
	require.EqualError(t, err, "summary failed")

	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusFailed, session.Compaction.Status)
	require.Equal(t, "summary failed", session.Compaction.LastError)
	require.Equal(t, 2, session.Compaction.TargetOffset)
	require.Equal(t, 10, session.Compaction.TargetMessageCount)

	client.Err = nil
	client.Responses = []*models.Response{{
		OutputText: `{"session_summary":"Older work","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`,
	}}

	err = service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	})
	require.NoError(t, err)

	session, ok, err = store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, session.Compaction.Status)
	require.Empty(t, session.Compaction.LastError)
	require.False(t, session.Compaction.CompletedAt.IsZero())

	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionPending)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionRunning)
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_MaybeRefreshMemory_FallsBackWhenClockReturnsZero(t *testing.T) {
	mem := &Memory{}
	store := summaryTestStore(summaryTestHistory(10))
	service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{
		Responses: []*models.Response{{
			OutputText: `{"session_summary":"Older work","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`,
		}},
	}, store)
	service.now = func() time.Time { return time.Time{} }

	require.NoError(t, service.MaybeRefreshMemory(context.Background(), mem, RefreshInput{
		Request:      summaryTriggerRequest(),
		SessionID:    storage.DefaultSessionID,
		TraceSession: &mocks.TraceSessionStub{},
	}))

	require.NotNil(t, mem.Summary)
	require.False(t, mem.Summary.UpdatedAt.IsZero())
}

func TestService_CompactSession_ReturnsValidationErrors(t *testing.T) {
	sess := storage.Session{ID: storage.DefaultSessionID}
	traceSession := &mocks.TraceSessionStub{}

	t.Run("nil_service", func(t *testing.T) {
		_, err := (*Service)(nil).CompactSession(context.Background(), sess, traceSession)
		require.EqualError(t, err, "memory service is required")
	})

	t.Run("nil_model_client", func(t *testing.T) {
		svc := NewService(summaryTestConfig(true), nil, &storagemock.SessionStore{})
		_, err := svc.CompactSession(context.Background(), sess, traceSession)
		require.EqualError(t, err, "model client is required")
	})

	t.Run("nil_summary_store", func(t *testing.T) {
		svc := NewService(summaryTestConfig(true), &mocks.ModelClientStub{}, nil)
		_, err := svc.CompactSession(context.Background(), sess, traceSession)
		require.EqualError(t, err, "summary store is required")
	})

	t.Run("nil_trace_session", func(t *testing.T) {
		svc := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, summaryTestStore(summaryTestHistory(10)))
		_, err := svc.CompactSession(context.Background(), sess, nil)
		require.EqualError(t, err, "trace session is required")
	})
}

func TestService_CompactSession_ReturnsLoadError(t *testing.T) {
	store := summaryTestStore(summaryTestHistory(10))
	store.GetSummaryFunc = func(context.Context, string) (storage.SessionSummary, bool, error) {
		return storage.SessionSummary{}, false, errors.New("load summary failed")
	}
	svc := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)
	traceSession := &mocks.TraceSessionStub{}

	_, err := svc.CompactSession(context.Background(), storage.Session{ID: storage.DefaultSessionID}, traceSession)
	require.EqualError(t, err, "load summary failed")
}

func TestService_CompactSession_ReturnsCountMessagesError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	store.CountMessagesFunc = func(context.Context, string, storage.MessageQueryOptions) (int, error) {
		return 0, errors.New("count failed")
	}
	svc := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, store)

	_, err := svc.CompactSession(context.Background(), storage.Session{ID: storage.DefaultSessionID}, traceSession)
	require.EqualError(t, err, "count failed")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_CompactSession_ReturnsHistoryTooShort(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	svc := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, summaryTestStore(summaryTestHistory(RecentSessionTail)))

	_, err := svc.CompactSession(context.Background(), storage.Session{ID: storage.DefaultSessionID}, traceSession)
	require.EqualError(t, err, "session history is too short to compact")
	requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
}

func TestService_CompactSession_ReturnsRefreshError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	svc := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{Err: errors.New("chat failed")}, store)

	_, err := svc.CompactSession(context.Background(), storage.Session{ID: storage.DefaultSessionID}, traceSession)
	require.EqualError(t, err, "chat failed")
}

func TestService_CompactSession_ReturnsSummary(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	store := summaryTestStore(summaryTestHistory(10))
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{
			OutputText: `{"session_summary":"Compacted","current_task":"t","discoveries":["d"],"open_questions":["q"],"next_actions":["n"]}`,
		}},
	}
	svc := summaryTestService(summaryTestConfig(true), client, store)

	out, err := svc.CompactSession(context.Background(), storage.Session{
		ID:               storage.DefaultSessionID,
		LastPromptTokens: 50,
	}, traceSession)

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, storage.DefaultSessionID, out.SessionID)
	require.Equal(t, "Compacted", out.SessionSummary)
	require.Equal(t, 2, out.SourceEndOffset)
	require.Equal(t, 10, out.SourceMessageCount)
	require.Equal(t, 1, client.CallCount)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummaryRequested)
	requireSummaryEvent(t, traceSession.Events, trace.EvtSummarySaved)
}

func TestService_TransitionCompactionPending(t *testing.T) {
	plan := refreshPlan{
		RequestedAt:        time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		TargetMessageCount: 10,
		TargetOffset:       2,
	}

	t.Run("nil session", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		err := service.transitionCompactionPending(context.Background(), nil, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "session is required")
	})

	t.Run("save error", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		})
		err := service.transitionCompactionPending(context.Background(), &storage.Session{ID: storage.DefaultSessionID}, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "save failed")
	})
}

func TestService_TransitionCompactionRunning(t *testing.T) {
	plan := refreshPlan{
		RequestedAt:        time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		TargetMessageCount: 10,
		TargetOffset:       2,
	}

	t.Run("nil session", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		err := service.transitionCompactionRunning(context.Background(), nil, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "session is required")
	})
}

func TestService_TransitionCompactionSucceeded(t *testing.T) {
	plan := refreshPlan{
		RequestedAt:        time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		TargetMessageCount: 10,
		TargetOffset:       2,
	}

	t.Run("nil session", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		err := service.transitionCompactionSucceeded(context.Background(), nil, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "session is required")
	})

	t.Run("save error", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		})
		session := &storage.Session{
			ID: storage.DefaultSessionID,
			Compaction: storage.SessionCompaction{
				LastError: "old",
				FailedAt:  time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
			},
		}
		err := service.transitionCompactionSucceeded(context.Background(), session, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "save failed")
	})
}

func TestService_ReconcileCompactionSucceeded(t *testing.T) {
	plan := refreshPlan{
		RequestedAt:        time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		TargetMessageCount: 10,
		TargetOffset:       2,
	}

	t.Run("nil session", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		err := service.reconcileCompactionSucceeded(context.Background(), nil, plan, &mocks.TraceSessionStub{})
		require.EqualError(t, err, "session is required")
	})

	t.Run("already reconciled", func(t *testing.T) {
		traceSession := &mocks.TraceSessionStub{}
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		session := &storage.Session{
			ID: storage.DefaultSessionID,
			Compaction: storage.SessionCompaction{
				Status:             storage.CompactionStatusSucceeded,
				TargetMessageCount: 10,
				TargetOffset:       2,
			},
		}
		require.NoError(t, service.reconcileCompactionSucceeded(context.Background(), session, plan, traceSession))
		require.Empty(t, traceSession.Events)
	})

	t.Run("save error records failure", func(t *testing.T) {
		traceSession := &mocks.TraceSessionStub{}
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		})
		session := &storage.Session{
			ID: storage.DefaultSessionID,
			Compaction: storage.SessionCompaction{
				RequestedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
				StartedAt:   time.Date(2026, 4, 3, 9, 1, 0, 0, time.UTC),
			},
		}
		err := service.reconcileCompactionSucceeded(context.Background(), session, plan, traceSession)
		require.EqualError(t, err, "save failed")
		requireSummaryEvent(t, traceSession.Events, trace.EvtContextCompactionFailed)
	})
}

func TestService_TransitionCompactionFailed(t *testing.T) {
	plan := refreshPlan{
		RequestedAt:        time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		TargetMessageCount: 10,
		TargetOffset:       2,
	}

	t.Run("nil session", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{})
		err := service.transitionCompactionFailed(context.Background(), nil, plan, errors.New("summary failed"), &mocks.TraceSessionStub{})
		require.EqualError(t, err, "session is required")
	})

	t.Run("save error", func(t *testing.T) {
		service := summaryTestService(summaryTestConfig(true), &mocks.ModelClientStub{}, &storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		})
		session := &storage.Session{ID: storage.DefaultSessionID}
		err := service.transitionCompactionFailed(context.Background(), session, plan, errors.New(" summary failed "), &mocks.TraceSessionStub{})
		require.EqualError(t, err, "save failed")
	})
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
	require.Equal(t, trace.EvtSummaryApplied, traceSession.Events[0].Type)
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

func TestFallbackSummary_UsesRawTextAsSessionSummary(t *testing.T) {
	summary, err := fallbackSummary(storage.DefaultSessionID, 1, 2, "## Summary\nKeep moving", time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, "## Summary\nKeep moving", summary.SessionSummary)
	require.Empty(t, summary.CurrentTask)
	require.Nil(t, summary.Discoveries)
	require.Nil(t, summary.OpenQuestions)
	require.Nil(t, summary.NextActions)
}

func TestFallbackSummary_StripsMarkdownFenceBeforeUsingRawText(t *testing.T) {
	summary, err := fallbackSummary(storage.DefaultSessionID, 1, 2, "```json\n## Summary\nKeep moving\n```", time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, "## Summary\nKeep moving", summary.SessionSummary)
}

func TestFallbackSummary_RejectsEmptyRaw(t *testing.T) {
	summary, err := fallbackSummary(storage.DefaultSessionID, 1, 2, "```json\n \n```", time.Now().UTC())
	require.Nil(t, summary)
	require.EqualError(t, err, "summary response is empty")
}

func TestFallbackSummary_RejectsMissingSessionID(t *testing.T) {
	summary, err := fallbackSummary("", 1, 2, "plain text", time.Now().UTC())
	require.Nil(t, summary)
	require.EqualError(t, err, "session summary is required")
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
		ContextLength:            100,
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	}))
}

func summaryTestConfig(enabled bool) *config.Config {
	return &config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
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
	for range count {
		history = append(history, handmsg.Message{
			Role:      handmsg.RoleUser,
			Content:   "history",
			CreatedAt: time.Now().UTC(),
		})
	}

	return history
}

func summaryTestService(cfg *config.Config, client models.Client, store SummaryStore) *Service {
	return NewService(cfg, client, store)
}

func summaryTestStore(history []handmsg.Message) *storagemock.SessionStore {
	session := storage.Session{ID: storage.DefaultSessionID}
	return &storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return session, true, nil
		},
		SaveFunc: func(_ context.Context, saved storage.Session) error {
			session = saved
			return nil
		},
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return len(history), nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			offset := max(opts.Offset, 0)
			if offset >= len(history) {
				return nil, nil
			}
			end := len(history)
			if opts.Limit > 0 && offset+opts.Limit < end {
				end = offset + opts.Limit
			}
			return append([]handmsg.Message(nil), history[offset:end]...), nil
		},
	}
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

func requireSummaryEventAbsent(t *testing.T, events []trace.Event, eventType string) {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			require.Fail(t, "unexpected trace event", eventType)
		}
	}
}

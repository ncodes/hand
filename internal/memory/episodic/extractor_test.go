package episodic

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	statemock "github.com/wandxy/hand/internal/state/mock"
	storememory "github.com/wandxy/hand/internal/state/storememory"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

type recordingTrace struct {
	events []recordedEvent
}

type recordedEvent struct {
	name    string
	payload any
}

func (r *recordingTrace) Record(name string, payload any) {
	r.events = append(r.events, recordedEvent{name: name, payload: payload})
}

func init() {
	logutils.SetOutput(io.Discard)
}

func TestService_ExtractWritesSourceLinkedEpisode(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)
	recorder := &recordingTrace{}

	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))
	require.NoError(t, manager.AppendMessages(ctx, storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "Remember the deployment checklist."},
		{Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{{Name: "read_file", Input: `{"path":"deploy.md"}`}}},
		{Role: handmsg.RoleTool, Name: "read_file", ToolCallID: "call_1", Content: "Run migration before deploy."},
	}))

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:      storage.DefaultSessionID,
		WindowSize:     2,
		MaxWindowChars: 1000,
		Trace:          recorder,
	})

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, result.SessionID)
	require.Equal(t, 2, len(result.Windows))
	require.Equal(t, 2, result.WriteCount)
	require.Equal(t, 3, result.MessageCount)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionStarted)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionWindowLoaded)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionExtractorRequested)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionCandidates)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionMemoryWritten)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionCompleted)

	memories, err := provider.Search(ctx, storage.MemorySearchQuery{
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusActive},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, memories.Hits, 2)
	require.Equal(t, storage.MemoryKindEpisodic, memories.Hits[0].Item.Kind)
	require.Equal(t, storage.DefaultSessionID, memories.Hits[0].Item.SourceLinks[0].SessionID)
	require.NotEmpty(t, memories.Hits[0].Item.SourceLinks[0].MessageIDs)
	require.NotEmpty(t, memories.Hits[0].Item.SourceLinks[0].Offsets)
	require.Contains(t, memories.Hits[0].Item.Text+memories.Hits[1].Item.Text, "tool_call read_file")
}

func TestService_ExtractSkipsDuplicateSourceRange(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)

	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))
	require.NoError(t, manager.AppendMessages(ctx, storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "Use pnpm for this workspace."},
		{Role: handmsg.RoleAssistant, Content: "I will remember that."},
	}))

	req := Request{
		SessionID:      storage.DefaultSessionID,
		OffsetStart:    new(0),
		OffsetEnd:      new(2),
		WindowSize:     2,
		MaxWindowChars: 1000,
	}
	first, err := newTestService(t, manager, provider).Extract(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 1, first.WriteCount)
	require.Equal(t, 0, first.SkipCount)

	second, err := newTestService(t, manager, provider).Extract(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 0, second.WriteCount)
	require.Equal(t, 1, second.SkipCount)

	memories, err := provider.Search(ctx, storage.MemorySearchQuery{
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusActive},
		Tags:     []string{sourceRangeTag(storage.DefaultSessionID, 0, 2)},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, memories.Hits, 1)
}

func TestService_ExtractLoadsBoundedWindows(t *testing.T) {
	ctx := context.Background()
	messages := []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "one"},
		{ID: 2, Role: handmsg.RoleAssistant, Content: "two"},
		{ID: 3, Role: handmsg.RoleUser, Content: "three"},
		{ID: 4, Role: handmsg.RoleAssistant, Content: "four"},
		{ID: 5, Role: handmsg.RoleUser, Content: "five"},
	}
	var loads []storage.MessageQueryOptions
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return len(messages), nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			loads = append(loads, opts)
			end := opts.Offset + opts.Limit
			if end > len(messages) {
				end = len(messages)
			}
			return append([]handmsg.Message(nil), messages[opts.Offset:end]...), nil
		},
	}
	manager := testManager(t, store)
	memoryStore := storememory.NewStore()
	provider := testProvider(t, memoryStore)

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:      storage.DefaultSessionID,
		WindowSize:     2,
		MaxWindows:     2,
		MaxWindowChars: 1000,
	})

	require.NoError(t, err)
	require.Equal(t, 2, len(result.Windows))
	require.Len(t, loads, 2)
	require.Equal(t, storage.MessageQueryOptions{Offset: 0, Limit: 2}, loads[0])
	require.Equal(t, storage.MessageQueryOptions{Offset: 2, Limit: 2}, loads[1])
}

func TestService_ExtractDoesNotPrecomputeLargeSessionWindows(t *testing.T) {
	ctx := context.Background()
	var loads []storage.MessageQueryOptions
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1_000_000_000, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			loads = append(loads, opts)
			return []handmsg.Message{{ID: uint(len(loads)), Role: handmsg.RoleUser, Content: "remember this"}}, nil
		},
	}
	manager := testManager(t, store)
	provider := testProvider(t, storememory.NewStore())

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:      storage.DefaultSessionID,
		WindowSize:     20,
		MaxWindows:     2,
		MaxWindowChars: 1000,
	})

	require.NoError(t, err)
	require.Len(t, result.Windows, 2)
	require.Len(t, loads, 2)
	require.Equal(t, storage.MessageQueryOptions{Offset: 0, Limit: 20}, loads[0])
	require.Equal(t, storage.MessageQueryOptions{Offset: 20, Limit: 20}, loads[1])
}

func TestService_ExtractHandlesEmptyAndShortSessions(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)

	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.NoError(t, err)
	require.Empty(t, result.Windows)
	require.Zero(t, result.WriteCount)

	require.NoError(t, manager.AppendMessages(ctx, storage.DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser}}))

	result, err = newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID: storage.DefaultSessionID,
	})
	require.NoError(t, err)
	require.Zero(t, result.WriteCount)
}

func TestService_ExtractBoundsEpisodeTextByTokenEstimate(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)

	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))
	require.NoError(t, manager.AppendMessages(ctx, storage.DefaultSessionID, []handmsg.Message{{
		Role:    handmsg.RoleUser,
		Content: "abcdefghijklmnopqrstuvwxyz",
	}}))

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:       storage.DefaultSessionID,
		WindowSize:      1,
		MaxWindowChars:  100,
		MaxWindowTokens: 2,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.WriteCount)

	memories, err := provider.Search(ctx, storage.MemorySearchQuery{
		Kinds:    []storage.MemoryKind{storage.MemoryKindEpisodic},
		Statuses: []storage.MemoryStatus{storage.MemoryStatusActive},
		Limit:    1,
	})
	require.NoError(t, err)
	require.Len(t, memories.Hits, 1)
	require.LessOrEqual(t, len([]rune(memories.Hits[0].Item.Text)), 8)
	logutils.PrettyPrint(memories.Hits[0].Item.Text)
}

func TestService_ExtractCapsDirectBudgetInputs(t *testing.T) {
	ctx := context.Background()
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return []handmsg.Message{{ID: 1, Role: handmsg.RoleUser, Content: "remember"}}, nil
		},
	}
	manager := testManager(t, store)
	provider := testProvider(t, storememory.NewStore())
	recorder := &recordingTrace{}

	_, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:       storage.DefaultSessionID,
		MaxWindowChars:  MaxWindowChars + 1,
		MaxWindowTokens: MaxWindowTokens + 1,
		Trace:           recorder,
	})

	require.NoError(t, err)
	started := tracePayloadFor(t, recorder, trace.EvtMemoryExtractionStarted)
	require.Equal(t, MaxWindowChars, started["max_window_chars"])
	require.Equal(t, MaxWindowTokens, started["max_window_tokens"])
}

func TestService_ExtractReturnsValidationAndProviderErrors(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)
	recorder := &recordingTrace{}

	_, err := NewService(nil, provider)
	require.EqualError(t, err, "state manager is required")

	_, err = NewService(manager, nil)
	require.EqualError(t, err, "memory repository is required")

	_, err = newTestService(t, manager, provider).Extract(ctx, Request{
		OffsetStart: intPtr(-1),
		Trace:       recorder,
	})
	require.EqualError(t, err, "offset_start must be greater than or equal to zero")
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionFailed)
}

func TestService_ExtractReturnsMissingDependencyErrors(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)

	var service *Service
	_, err := service.Extract(ctx, Request{})
	require.EqualError(t, err, "state manager is required")

	_, err = (&Service{memory: provider}).Extract(ctx, Request{})
	require.EqualError(t, err, "state manager is required")

	_, err = (&Service{manager: manager}).Extract(ctx, Request{})
	require.EqualError(t, err, "memory repository is required")
}

func TestService_ExtractUsesInjectedClock(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	provider := testProvider(t, store)
	recorder := &recordingTrace{}
	ticks := []time.Time{
		time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 10, 0, 2, 0, time.UTC),
	}
	service := newTestService(t, manager, provider)
	service.nowFunc = func() time.Time {
		next := ticks[0]
		ticks = ticks[1:]
		return next
	}

	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))

	result, err := service.Extract(ctx, Request{SessionID: storage.DefaultSessionID, Trace: recorder})

	require.NoError(t, err)
	require.Empty(t, result.Windows)
	completed := tracePayloadFor(t, recorder, trace.EvtMemoryExtractionCompleted)
	require.Equal(t, int64(2000), completed["duration_ms"])
}

func TestService_NormalizeRequestBoundsAndErrors(t *testing.T) {
	ctx := context.Background()
	countErr := errors.New("count failed")
	currentErr := errors.New("current failed")
	store := &statemock.Store{
		CurrentFunc: func(context.Context) (string, bool, error) {
			return "", false, currentErr
		},
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 3, nil
		},
	}
	manager := testManager(t, store)
	service := &Service{manager: manager, memory: testProvider(t, storememory.NewStore())}

	_, err := service.normalizeRequest(ctx, Request{})
	require.ErrorIs(t, err, currentErr)

	store.CurrentFunc = func(context.Context) (string, bool, error) {
		return storage.DefaultSessionID, true, nil
	}
	store.CountMessagesFunc = func(context.Context, string, storage.MessageQueryOptions) (int, error) {
		return 0, countErr
	}
	_, err = service.normalizeRequest(ctx, Request{})
	require.ErrorIs(t, err, countErr)

	store.CountMessagesFunc = func(context.Context, string, storage.MessageQueryOptions) (int, error) {
		return 3, nil
	}
	_, err = service.normalizeRequest(ctx, Request{OffsetStart: intPtr(3), OffsetEnd: intPtr(2)})
	require.EqualError(t, err, "offset_end must be greater than or equal to offset_start")

	_, err = service.normalizeRequest(ctx, Request{MaxWindows: -1})
	require.EqualError(t, err, "max_windows must be greater than or equal to zero")

	_, err = service.normalizeRequest(ctx, Request{MaxWindowTokens: -1})
	require.EqualError(t, err, "max_window_tokens must be greater than or equal to zero")

	normalized, err := service.normalizeRequest(ctx, Request{
		OffsetEnd:       intPtr(99),
		WindowSize:      MaxWindowSize + 1,
		MaxWindowChars:  MaxWindowChars + 1,
		MaxWindowTokens: MaxWindowTokens + 1,
		Trigger:         " manual ",
	})
	require.NoError(t, err)
	require.Equal(t, 3, normalized.OffsetEnd)
	require.Equal(t, MaxWindowSize, normalized.WindowSize)
	require.Equal(t, MaxWindowChars, normalized.MaxWindowChars)
	require.Equal(t, MaxWindowTokens, normalized.MaxWindowTokens)
	require.Equal(t, "manual", normalized.Trigger)
}

func TestService_ExtractReturnsWindowErrorsWithTrace(t *testing.T) {
	ctx := context.Background()
	recorder := &recordingTrace{}
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, errors.New("load failed")
		},
	}
	manager := testManager(t, store)
	provider := testProvider(t, storememory.NewStore())

	_, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID: storage.DefaultSessionID,
		Trace:     recorder,
	})

	require.EqualError(t, err, "load failed")
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionFailed)
}

func TestService_ExtractReturnsSearchAndWriteErrors(t *testing.T) {
	ctx := context.Background()
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return []handmsg.Message{{ID: 1, Role: handmsg.RoleUser, Content: "remember this"}}, nil
		},
	}
	manager := testManager(t, store)

	_, err := newTestService(t, manager, memoryProviderStub{
		searchErr: errors.New("search failed"),
	}).Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.EqualError(t, err, "search failed")

	_, err = newTestService(t, manager, memoryProviderStub{
		upsertErr: errors.New("write failed"),
	}).Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.EqualError(t, err, "write failed")
}

func TestMessageLineHandlesInvalidUTF8AndFallbackRole(t *testing.T) {
	line := messageLine(handmsg.Message{Content: string([]byte{0xff, 'o', 'k'})})

	require.Equal(t, "message: ok", line)
	require.Empty(t, messageLine(handmsg.Message{ToolCalls: []handmsg.ToolCall{{}}}))
}

func TestHelpersCoverEdgeBranches(t *testing.T) {
	require.Empty(t, truncateRunes("hello", 0))
	require.Equal(t, "hello", truncateRunes("hello", 10))
	logger := zerolog.Nop()
	logField(logger.Debug(), "enabled", true)
}

func testManager(t *testing.T, store storage.Store) *statemanager.Manager {
	t.Helper()

	manager, err := statemanager.NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	return manager
}

func testProvider(t *testing.T, store storage.Store) *testRepository {
	t.Helper()

	memoryStore, ok := store.(storage.MemoryStore)
	require.True(t, ok)
	return &testRepository{store: memoryStore}
}

func newTestService(t *testing.T, manager *statemanager.Manager, provider MemoryRepository) *Service {
	t.Helper()

	service, err := NewService(manager, provider)
	require.NoError(t, err)
	return service
}

func traceEventNames(recorder *recordingTrace) []string {
	names := make([]string, 0, len(recorder.events))
	for _, event := range recorder.events {
		names = append(names, event.name)
	}
	return names
}

func tracePayloadFor(t *testing.T, recorder *recordingTrace, name string) map[string]any {
	t.Helper()

	for _, event := range recorder.events {
		if event.name != name {
			continue
		}
		payload, ok := event.payload.(map[string]any)
		require.True(t, ok)
		return payload
	}
	require.FailNow(t, "trace event not found", name)
	return nil
}

func intPtr(value int) *int {
	return &value
}

type memoryProviderStub struct {
	searchErr error
	upsertErr error
}

func (p memoryProviderStub) Search(context.Context, storage.MemorySearchQuery) (storage.MemorySearchResult, error) {
	return storage.MemorySearchResult{}, p.searchErr
}

func (p memoryProviderStub) RecordEpisode(context.Context, EpisodeRecord) (storage.MemoryItem, error) {
	return storage.MemoryItem{}, p.upsertErr
}

type testRepository struct {
	store storage.MemoryStore
}

func (r *testRepository) Search(ctx context.Context, query storage.MemorySearchQuery) (storage.MemorySearchResult, error) {
	return r.store.SearchMemory(ctx, query)
}

func (r *testRepository) RecordEpisode(ctx context.Context, record EpisodeRecord) (storage.MemoryItem, error) {
	item := record.Item.Clone()
	item.Kind = storage.MemoryKindEpisodic
	if item.Status == "" {
		item.Status = storage.MemoryStatusActive
	}
	return r.store.UpsertMemory(ctx, item)
}

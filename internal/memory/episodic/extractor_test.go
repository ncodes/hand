package episodic

import (
	"context"
	"errors"
	"io"
	"strings"
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

type fakeCandidateExtractor struct {
	result CandidateResult
	err    error
	req    CandidateRequest
}

func (e fakeCandidateExtractor) ExtractCandidates(_ context.Context, req CandidateRequest) (CandidateResult, error) {
	if e.err != nil {
		return CandidateResult{}, e.err
	}
	return e.result, nil
}

type capturingCandidateExtractor struct {
	result CandidateResult
	req    CandidateRequest
}

func (e *capturingCandidateExtractor) ExtractCandidates(_ context.Context, req CandidateRequest) (CandidateResult, error) {
	e.req = req
	return e.result, nil
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

	service := newTestServiceWithCandidates(t, manager, provider, []episodeCandidate{{
		Kind:       episodeKindToolEvent,
		Title:      "Tool event: read_file",
		Text:       "Tool event: read_file captured deployment checklist context.",
		Confidence: 0.78,
		Metadata:   map[string]string{"tool_name": "read_file", "status": "success"},
	}})

	result, err := service.Extract(ctx, Request{
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
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate},
		Limit:    10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, memories.Hits)
	require.Len(t, memories.Hits, 2)
	require.Equal(t, storage.MemoryKindEpisodic, memories.Hits[0].Item.Kind)
	require.Equal(t, storage.MemoryStatusCandidate, memories.Hits[0].Item.Status)
	require.Equal(t, storage.DefaultSessionID, memories.Hits[0].Item.SourceLinks[0].SessionID)
	require.NotEmpty(t, memories.Hits[0].Item.SourceLinks[0].MessageIDs)
	require.NotEmpty(t, memories.Hits[0].Item.SourceLinks[0].Offsets)
	require.NotContains(t, memories.Hits[0].Item.Text, "user:")
	require.Contains(t, memoryHitText(memories.Hits), "Tool event:")
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
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate},
		Tags:     []string{sourceRangeTag(storage.DefaultSessionID, 0, 2)},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, memories.Hits, 1)
}

func TestService_ExtractChecksDuplicateByDeterministicMemoryID(t *testing.T) {
	ctx := context.Background()
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, errors.New("messages should not be loaded for completed checkpoint")
		},
	}
	manager := testManager(t, store)
	expectedID := candidateMemoryID(storage.DefaultSessionID, 0, 1, episodeKindDecision)
	provider := &memoryProviderStub{
		searchResult: storage.MemorySearchResult{
			Hits: []storage.MemorySearchHit{{Item: storage.MemoryItem{ID: expectedID}}},
		},
	}

	result, err := newTestService(t, manager, provider).Extract(ctx, Request{
		SessionID:  storage.DefaultSessionID,
		WindowSize: 1,
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.WriteCount)
	require.Equal(t, 1, result.SkipCount)
	require.Equal(t, candidateMemoryIDs(storage.DefaultSessionID, 0, 1), provider.searchQuery.IDs)
	require.Empty(t, provider.searchQuery.Tags)
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

func TestService_ExtractReturnsBackgroundCheckpointUpdateError(t *testing.T) {
	ctx := context.Background()
	checkpointErr := errors.New("checkpoint failed")
	store := &statemock.Store{
		CountMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) (int, error) {
			return 1, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return []handmsg.Message{{ID: 1, Role: handmsg.RoleUser, Content: "remember"}}, nil
		},
		UpdateEpisodicCheckpointFunc: func(context.Context, string, int) error {
			return checkpointErr
		},
	}
	manager := testManager(t, store)
	recorder := &recordingTrace{}

	_, err := newTestService(t, manager, &memoryProviderStub{}).Extract(ctx, Request{
		SessionID:  storage.DefaultSessionID,
		WindowSize: 1,
		Trigger:    backgroundTrigger,
		Trace:      recorder,
	})

	require.ErrorIs(t, err, checkpointErr)
	require.Contains(t, traceEventNames(recorder), trace.EvtMemoryExtractionFailed)
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
		Content: "Remember abcdefghijklmnopqrstuvwxyz",
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
		Statuses: []storage.MemoryStatus{storage.MemoryStatusCandidate},
		Limit:    1,
	})
	require.NoError(t, err)
	require.Len(t, memories.Hits, 1)
	require.LessOrEqual(t, len([]rune(memories.Hits[0].Item.Text)), 8)
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

	_, err := NewService(nil, provider, nil)
	require.EqualError(t, err, "state manager is required")

	_, err = NewService(manager, nil, nil)
	require.EqualError(t, err, "memory repository is required")

	_, err = NewService(manager, provider, nil)
	require.EqualError(t, err, "memory episode extractor is required")

	extractor, err := NewLLMExtractor(LLMExtractorOptions{
		Client: &llmExtractorClientStub{},
		Model:  "test-model",
	})
	require.NoError(t, err)
	service, err := NewService(manager, provider, extractor)
	require.NoError(t, err)
	require.NotNil(t, service)

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

	_, err = (&Service{manager: manager, memory: provider}).Extract(ctx, Request{})
	require.EqualError(t, err, "memory episode extractor is required")
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

	_, err := newTestService(t, manager, &memoryProviderStub{
		searchErr: errors.New("search failed"),
	}).Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.EqualError(t, err, "search failed")

	_, err = newTestService(t, manager, &memoryProviderStub{
		upsertErr: errors.New("write failed"),
	}).Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.EqualError(t, err, "write failed")

	service := newTestService(t, manager, &memoryProviderStub{})
	service.extractor = fakeCandidateExtractor{err: errors.New("extractor failed")}
	_, err = service.Extract(ctx, Request{SessionID: storage.DefaultSessionID})
	require.EqualError(t, err, "extractor failed")
}

func TestService_CandidatesFromMessages_UsesLLMExtractorCandidates(t *testing.T) {
	req := normalizedRequest{
		SessionID:       storage.DefaultSessionID,
		MaxWindowChars:  1000,
		MaxWindowTokens: 250,
		Trigger:         "command",
	}
	window := sourceWindow{Start: 0, End: 5}
	messages := []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "We should use generic StartBackground instead of episodic-specific public APIs."},
		{ID: 2, Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{{Name: "run_command", Input: `{"cmd":"go test ./..."}`}}},
		{ID: 3, Role: handmsg.RoleTool, Name: "run_command", Content: "error: tests failed"},
		{ID: 4, Role: handmsg.RoleAssistant, Content: "Implemented StartBackground and verified tests passed."},
		{ID: 5, Role: handmsg.RoleUser, Content: "Prefer generic provider background APIs going forward."},
	}

	service := Service{extractor: fakeCandidateExtractor{result: CandidateResult{Candidates: representativeEpisodeCandidates()}}}
	items, rejections, err := service.candidatesFromMessages(context.Background(), req, window, messages)

	require.NoError(t, err)
	require.Empty(t, rejections)
	require.Len(t, items, 9)
	byKind := memoryItemsByCandidateKind(items)
	require.Contains(t, byKind, episodeKindDecision)
	require.Contains(t, byKind, episodeKindOutcome)
	require.Contains(t, byKind, episodeKindToolEvent)
	require.Contains(t, byKind, episodeKindBlocker)
	require.Contains(t, byKind, episodeKindUserCorrection)
	require.Contains(t, byKind, episodeKindTaskTrace)
	require.Contains(t, byKind, episodeKindResolvedIssue)
	require.Contains(t, byKind, episodeKindMilestone)
	require.Contains(t, byKind, episodeKindDiscarded)

	decision := byKind[episodeKindDecision]
	require.Equal(t, "generic StartBackground", decision.Metadata["chosen_option"])
	require.Equal(t, "episodic-specific public background APIs", decision.Metadata["rejected_alternatives"])
	require.Equal(t, "provider background APIs can host multiple memory background processes", decision.Metadata["reason"])
	require.Equal(t, "0-5", decision.Metadata["source_range"])
	require.Equal(t, "0", decision.Metadata["source_start"])
	require.Equal(t, "5", decision.Metadata["source_end"])

	tool := byKind[episodeKindToolEvent]
	require.Equal(t, "run_command", tool.Metadata["tool_name"])
	require.Equal(t, "failed", tool.Metadata["status"])
	require.Equal(t, "failed", tool.Metadata["attempt_status"])
	require.Equal(t, "verify the background API implementation with the Go test suite", tool.Metadata["purpose"])
	require.Equal(t, "go test ./...", tool.Metadata["artifact_or_command_ref"])
	require.Contains(t, tool.Metadata["reference"], "go test")

	outcome := byKind[episodeKindOutcome]
	require.Equal(t, storage.MemoryStatusCandidate, outcome.Status)
	require.Equal(t, "success", outcome.Metadata["outcome_status"])
	require.Equal(t, "replace episodic-specific background APIs", outcome.Metadata["requested_goal"])
	require.Equal(t, "StartBackground now handles provider background processes", outcome.Metadata["resulting_change"])
	require.Equal(t, "go test ./... passed", outcome.Metadata["verification_status"])
	require.Equal(t, "none_identified", outcome.Metadata["remaining_risk"])
	require.Equal(t, "tests_passed_after_start_background_changes", outcome.Metadata["causal_reason"])
	require.Contains(t, outcome.Text, "because")
	require.NotContains(t, outcome.Text, "assistant:")
	require.NotEmpty(t, outcome.SourceLinks[0].MessageIDs)
	require.NotEmpty(t, outcome.SourceLinks[0].Offsets)
	require.LessOrEqual(t, outcome.Confidence, 1.0)
	require.GreaterOrEqual(t, outcome.Confidence, 0.0)
	require.Equal(t, "high", outcome.Metadata["source_quality"])
	require.Equal(t, "high", outcome.Metadata["usefulness"])
	require.Equal(t, "source_window", outcome.Metadata["recency"])

	require.Equal(t, "trace:2,trace:3", byKind[episodeKindTaskTrace].Metadata["trace_event_refs"])
	require.Equal(t, "fixed", byKind[episodeKindResolvedIssue].Metadata["resolution_status"])
	require.Equal(t, "unresolved", byKind[episodeKindBlocker].Metadata["blocker_status"])
	require.Equal(t, "open", byKind[episodeKindBlocker].Metadata["follow_up_status"])
	require.Equal(t, "medium", byKind[episodeKindBlocker].Metadata["uncertainty"])
	require.Equal(t, "partial", byKind[episodeKindMilestone].Metadata["progress_status"])
	require.Equal(t, "phase_8c", byKind[episodeKindMilestone].Metadata["milestone"])
	require.Equal(t, "manual_rule_parser", byKind[episodeKindDiscarded].Metadata["rejected_alternative"])
	require.Equal(t, "medium", byKind[episodeKindDiscarded].Metadata["uncertainty"])
}

func TestService_CandidatesFromMessages_IncludesTaskTraceEvidence(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	manager := testManager(t, store)
	require.NoError(t, manager.Save(ctx, storage.Session{ID: storage.DefaultSessionID}))

	_, err := manager.AppendTraceEvent(ctx, storage.TraceEvent{
		SessionID: storage.DefaultSessionID,
		Type:      trace.EvtChatStarted,
		Timestamp: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"ignored": true},
	})
	require.NoError(t, err)
	_, err = manager.AppendTraceEvent(ctx, storage.TraceEvent{
		SessionID: storage.DefaultSessionID,
		Type:      trace.EvtToolInvocationStarted,
		Timestamp: time.Date(2026, 5, 3, 12, 0, 1, 0, time.UTC),
		Payload:   map[string]any{"name": "run_command", "command": "go test ./..."},
	})
	require.NoError(t, err)
	_, err = manager.AppendTraceEvent(ctx, storage.TraceEvent{
		SessionID: storage.DefaultSessionID,
		Type:      trace.EvtToolInvocationCompleted,
		Timestamp: time.Date(2026, 5, 3, 12, 0, 2, 0, time.UTC),
		Payload:   map[string]any{"name": "run_command", "exit_code": 0},
	})
	require.NoError(t, err)
	_, err = manager.AppendTraceEvent(ctx, storage.TraceEvent{
		SessionID: storage.DefaultSessionID,
		Type:      trace.EvtMemoryExtractionStarted,
		Timestamp: time.Date(2026, 5, 3, 12, 0, 3, 0, time.UTC),
		Payload:   map[string]any{"ignored": true},
	})
	require.NoError(t, err)

	extractor := &capturingCandidateExtractor{result: CandidateResult{Candidates: []episodeCandidate{{
		Kind:       episodeKindToolEvent,
		Title:      "Tool event",
		Text:       "Tool event: run_command completed successfully.",
		Confidence: 0.88,
		Metadata: map[string]string{
			"tool_name":        "run_command",
			"status":           "success",
			"trace_event_refs": "trace:2,trace:3",
		},
	}}}}
	service := Service{manager: manager, extractor: extractor}

	items, rejections, err := service.candidatesFromMessages(ctx, normalizedRequest{
		SessionID:       storage.DefaultSessionID,
		MaxWindowChars:  1000,
		MaxWindowTokens: 250,
		Trigger:         "command",
	}, sourceWindow{Start: 0, End: 1}, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleAssistant, Content: "I ran tests and they passed."},
	})

	require.NoError(t, err)
	require.Empty(t, rejections)
	require.Len(t, items, 1)
	require.Contains(t, extractor.req.TraceEvents, taskTraceEvidence{Ref: "trace:2", Type: trace.EvtToolInvocationStarted, Timestamp: "2026-05-03T12:00:01Z", Payload: `{"command":"go test ./...","name":"run_command"}`})
	require.Len(t, extractor.req.TraceEvents, 2)
	require.Equal(t, []string{trace.EvtToolInvocationStarted, trace.EvtToolInvocationCompleted}, traceEvidenceTypes(extractor.req.TraceEvents))
	require.Equal(t, "trace:2", extractor.req.TraceEvents[0].Ref)
	require.Contains(t, extractor.req.TraceEvents[0].Payload, "go test ./...")
	require.Equal(t, "trace:2,trace:3", items[0].Metadata["trace_event_refs"])
	require.Equal(t, "trace:2,trace:3", items[0].Metadata["available_trace_event_refs"])
	require.Equal(t, "2", items[0].Metadata["available_trace_event_count"])
}

func TestService_CandidatesFromMessages_ReturnsTraceLoadError(t *testing.T) {
	traceErr := errors.New("trace load failed")
	service := Service{manager: traceErrorManager{err: traceErr}, extractor: fakeCandidateExtractor{}}

	_, _, err := service.candidatesFromMessages(context.Background(), normalizedRequest{
		SessionID:       storage.DefaultSessionID,
		MaxWindowChars:  1000,
		MaxWindowTokens: 250,
	}, sourceWindow{Start: 0, End: 1}, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "remember"},
	})

	require.ErrorIs(t, err, traceErr)
}

func TestService_TraceEvidenceHelpersCoverEdgeBranches(t *testing.T) {
	events, err := (Service{}).traceEvidence(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, events)

	service := Service{manager: traceErrorManager{err: storage.ErrTraceStoreUnsupported}}
	events, err = service.traceEvidence(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, events)

	service = Service{manager: traceErrorManager{result: storage.TraceResult{Events: []storage.TraceEvent{{
		SessionID: storage.DefaultSessionID,
		Type:      trace.EvtChatStarted,
	}}}}}
	events, err = service.traceEvidence(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, events)

	require.Equal(t, "trace_id:7", traceEventRef(storage.TraceEvent{ID: 7}))
	require.Equal(t, "trace:unknown", traceEventRef(storage.TraceEvent{}))
	require.Empty(t, traceEventTimestamp(storage.TraceEvent{}))
	require.Empty(t, tracePayloadText(nil))
	require.Empty(t, tracePayloadText(map[string]any{"invalid": func() {}}))
	require.LessOrEqual(
		t,
		len([]rune(tracePayloadText(map[string]any{"value": strings.Repeat("x", maxTracePayloadChars+50)}))),
		maxTracePayloadChars,
	)
}

func TestService_CandidatesFromMessages_UsesLLMExtractorRejections(t *testing.T) {
	req := normalizedRequest{SessionID: storage.DefaultSessionID, MaxWindowChars: 1000, MaxWindowTokens: 250}
	service := Service{extractor: fakeCandidateExtractor{result: CandidateResult{
		Rejections: []candidateRejection{{Kind: "window", Reason: "low_signal"}},
	}}}

	items, rejections, err := service.candidatesFromMessages(context.Background(), req, sourceWindow{Start: 0, End: 1}, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "hi"},
	})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, []candidateRejection{{Kind: "window", Reason: "low_signal"}}, rejections)

	service.extractor = fakeCandidateExtractor{err: errors.New("extract failed")}
	items, rejections, err = service.candidatesFromMessages(context.Background(), req, sourceWindow{Start: 0, End: 1}, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "Maybe someday there could be another interface."},
	})
	require.EqualError(t, err, "extract failed")
	require.Empty(t, items)
	require.Empty(t, rejections)
}

func TestService_CandidatesFromMessages_CoversEmptyAndInvalidCandidatePaths(t *testing.T) {
	req := normalizedRequest{SessionID: storage.DefaultSessionID, MaxWindowChars: 1000, MaxWindowTokens: 250}
	window := sourceWindow{Start: 0, End: 1}

	items, rejections, err := (Service{}).candidatesFromMessages(context.Background(), req, window, []handmsg.Message{{}})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, []candidateRejection{{Kind: "window", Reason: "empty_window"}}, rejections)

	_, _, err = (Service{}).candidatesFromMessages(context.Background(), req, window, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "This needs extraction."},
	})
	require.EqualError(t, err, "memory episode extractor is required")

	service := Service{extractor: fakeCandidateExtractor{result: CandidateResult{Candidates: []episodeCandidate{
		{Kind: "unknown", Title: "Unknown", Text: "Unknown candidate", Confidence: 0.5},
		{Kind: episodeKindDecision, Title: "   ", Text: "   ", Confidence: 0.5},
	}}}}
	items, rejections, err = service.candidatesFromMessages(context.Background(), req, window, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "This candidate should be rejected after validation."},
	})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, []candidateRejection{
		{Kind: "unknown", Reason: "empty_candidate"},
		{Kind: episodeKindDecision, Reason: "empty_candidate"},
	}, rejections)

	service.extractor = fakeCandidateExtractor{result: CandidateResult{}}
	items, rejections, err = service.candidatesFromMessages(context.Background(), req, window, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "The model returned no decision."},
	})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, []candidateRejection{{Kind: "window", Reason: "no_curated_candidate"}}, rejections)
}

func TestCuratedCandidateHelpersCoverEdgeBranches(t *testing.T) {
	req := normalizedRequest{SessionID: storage.DefaultSessionID, MaxWindowChars: 100, MaxWindowTokens: 25}
	window := sourceWindow{Start: 0, End: 1}
	emptyEvidence := messageEvidence{}

	item, ok := memoryItemFromCandidate(req, window, emptyEvidence, episodeCandidate{})
	require.False(t, ok)
	require.Empty(t, item)

	item, ok = memoryItemFromCandidate(req, window, emptyEvidence, episodeCandidate{
		Kind:       episodeKindOutcome,
		Title:      "Title only",
		Confidence: 0.4,
		SourceLinks: []storage.MemorySourceLink{{
			SessionID:     "custom-session",
			CreatedReason: "custom",
		}},
	})
	require.True(t, ok)
	require.Equal(t, "Title only", item.Title)
	require.Empty(t, item.Text)
	require.Equal(t, "custom-session", item.SourceLinks[0].SessionID)

	require.Equal(t, "medium", sourceQuality(emptyEvidence))
	require.Equal(t, "medium", usefulness(episodeKindBlocker))
	require.Equal(t, "medium", usefulness(episodeKindTaskTrace))
	require.Equal(t, "high", usefulness(episodeKindResolvedIssue))
	require.Equal(t, "high", usefulness(episodeKindMilestone))
	require.Equal(t, "high", usefulness(episodeKindDiscarded))
	require.Equal(t, "low", usefulness("unknown"))
	require.Equal(t, "low", uncertainty(0.9))
	require.Equal(t, "medium", uncertainty(0.7))
	require.Equal(t, "high", uncertainty(0.2))
	require.Equal(t, 0.0, clampConfidence(-1))
	require.Equal(t, 1.0, clampConfidence(2))
}

func TestMemoryItemFromCandidate_PreservesOutcomeStatusVariants(t *testing.T) {
	req := normalizedRequest{SessionID: storage.DefaultSessionID, MaxWindowChars: 200, MaxWindowTokens: 50}
	window := sourceWindow{Start: 0, End: 1}
	evidence := messageEvidence{MessageIDs: []uint{1}, Offsets: []int{0}, Lines: []string{"assistant: done"}}

	for _, status := range []string{"success", "failed", "partial", "follow_up_required"} {
		item, ok := memoryItemFromCandidate(req, window, evidence, episodeCandidate{
			Kind:       episodeKindOutcome,
			Title:      "Outcome",
			Text:       "Outcome status: " + status,
			Confidence: 0.72,
			Metadata: map[string]string{
				"outcome_status": status,
			},
		})

		require.True(t, ok)
		require.Equal(t, status, item.Metadata["outcome_status"])
	}
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

	return newTestServiceWithCandidates(t, manager, provider, []episodeCandidate{{
		Kind:       episodeKindUserCorrection,
		Title:      "User correction or preference",
		Text:       "User correction or preference: remember this",
		Confidence: 0.82,
		Metadata:   map[string]string{"durability": "explicit"},
	}})
}

func newTestServiceWithCandidates(
	t *testing.T,
	manager *statemanager.Manager,
	provider MemoryRepository,
	candidates []episodeCandidate,
) *Service {
	t.Helper()

	service := &Service{
		manager: manager,
		memory:  provider,
		extractor: fakeCandidateExtractor{result: CandidateResult{
			Candidates: candidates,
		}},
	}
	return service
}

func traceEventNames(recorder *recordingTrace) []string {
	names := make([]string, 0, len(recorder.events))
	for _, event := range recorder.events {
		names = append(names, event.name)
	}
	return names
}

func traceEvidenceTypes(events []taskTraceEvidence) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
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

func memoryHitText(hits []storage.MemorySearchHit) string {
	var text strings.Builder
	for _, hit := range hits {
		text.WriteString(hit.Item.Text)
		text.WriteString("\n")
	}
	return text.String()
}

func memoryItemsByCandidateKind(items []storage.MemoryItem) map[string]storage.MemoryItem {
	byKind := make(map[string]storage.MemoryItem, len(items))
	for _, item := range items {
		byKind[item.Metadata["candidate_kind"]] = item
	}
	return byKind
}

func representativeEpisodeCandidates() []episodeCandidate {
	return []episodeCandidate{
		{
			Kind:       episodeKindDecision,
			Title:      "Decision from session",
			Text:       "Decision: use generic StartBackground instead of episodic-specific public APIs.",
			Confidence: 0.72,
			Metadata: map[string]string{
				"chosen_option":         "generic StartBackground",
				"rejected_alternatives": "episodic-specific public background APIs",
				"reason":                "provider background APIs can host multiple memory background processes",
				"source_range":          "0-5",
			},
		},
		{
			Kind:       episodeKindOutcome,
			Title:      "Task outcome from session",
			Text:       "Task outcome: Implemented StartBackground and verified tests passed because the background API shape was corrected.",
			Confidence: 0.76,
			Metadata: map[string]string{
				"outcome_status":      "success",
				"requested_goal":      "replace episodic-specific background APIs",
				"resulting_change":    "StartBackground now handles provider background processes",
				"verification_status": "go test ./... passed",
				"remaining_risk":      "none_identified",
				"causal_reason":       "tests_passed_after_start_background_changes",
			},
		},
		{
			Kind:       episodeKindToolEvent,
			Title:      "Tool event: run_command",
			Text:       "Tool event: run_command completed with status failed. Relevant reference: go test ./....",
			Confidence: 0.78,
			Metadata: map[string]string{
				"tool_name":               "run_command",
				"status":                  "failed",
				"attempt_status":          "failed",
				"purpose":                 "verify the background API implementation with the Go test suite",
				"artifact_or_command_ref": "go test ./...",
				"reference":               "go test ./...",
			},
		},
		{
			Kind:       episodeKindBlocker,
			Title:      "Blocker or risk from session",
			Text:       "Blocker or risk: tests failed before being fixed.",
			Confidence: 0.68,
			Metadata: map[string]string{
				"resolution_status": "unresolved_or_uncertain",
				"blocker_status":    "unresolved",
				"follow_up_status":  "open",
			},
		},
		{
			Kind:       episodeKindUserCorrection,
			Title:      "User correction or preference",
			Text:       "User correction or preference: Prefer generic provider background APIs going forward.",
			Confidence: 0.82,
			Metadata:   map[string]string{"durability": "explicit"},
		},
		{
			Kind:       episodeKindTaskTrace,
			Title:      "Task trace: test failure and recovery",
			Text:       "Task trace: go test failed, then later passed after StartBackground changes.",
			Confidence: 0.74,
			Metadata:   map[string]string{"trace_event_refs": "trace:2,trace:3"},
		},
		{
			Kind:       episodeKindResolvedIssue,
			Title:      "Resolved issue: background API shape",
			Text:       "Resolved issue: episodic-specific public background APIs were replaced with generic provider background APIs.",
			Confidence: 0.79,
			Metadata:   map[string]string{"resolution_status": "fixed"},
		},
		{
			Kind:       episodeKindMilestone,
			Title:      "Project milestone: Phase 8c trace evidence",
			Text:       "Project milestone: curated extraction now uses task trace evidence alongside session messages.",
			Confidence: 0.81,
			Metadata: map[string]string{
				"milestone":       "phase_8c",
				"progress_status": "partial",
			},
		},
		{
			Kind:       episodeKindDiscarded,
			Title:      "Discarded approach: manual rule parser",
			Text:       "Discarded approach: manual rule parsing was rejected in favor of LLM-only curated extraction.",
			Confidence: 0.7,
			Metadata:   map[string]string{"rejected_alternative": "manual_rule_parser"},
		},
	}
}

func intPtr(value int) *int {
	return &value
}

type memoryProviderStub struct {
	searchQuery  storage.MemorySearchQuery
	searchResult storage.MemorySearchResult
	searchErr    error
	upsertErr    error
}

type traceErrorManager struct {
	result storage.TraceResult
	err    error
}

func (m traceErrorManager) CurrentSession(context.Context) (string, error) {
	return storage.DefaultSessionID, nil
}

func (m traceErrorManager) CountMessages(context.Context, string, storage.MessageQueryOptions) (int, error) {
	return 0, nil
}

func (m traceErrorManager) GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
	return nil, nil
}

func (m traceErrorManager) ListTraceEvents(context.Context, storage.TraceQuery) (storage.TraceResult, error) {
	return m.result, m.err
}

func (m traceErrorManager) UpdateEpisodicCheckpoint(context.Context, string, int) error {
	return nil
}

func (p *memoryProviderStub) Search(_ context.Context, query storage.MemorySearchQuery) (storage.MemorySearchResult, error) {
	p.searchQuery = query
	if p.searchErr != nil {
		return storage.MemorySearchResult{}, p.searchErr
	}
	return p.searchResult, nil
}

func (p *memoryProviderStub) RecordEpisode(context.Context, EpisodeRecord) (storage.MemoryItem, error) {
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

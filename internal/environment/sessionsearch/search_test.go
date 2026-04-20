package sessionsearch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	handmsg "github.com/wandxy/hand/internal/messages"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	memorystore "github.com/wandxy/hand/internal/storage/memory"
	storagemock "github.com/wandxy/hand/internal/storage/mock"
	"github.com/wandxy/hand/pkg/nanoid"
)

var sessionSearchTestSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "session-search", "EnvironmentSearchTestSeed")

func TestSearch_FindsAssistantToolCallsAndPlainText(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))

	now := time.Now().UTC()
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello plain text", CreatedAt: now},
		{Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "search_files", Input: `{"pattern":"needle"}`}}, CreatedAt: now.Add(time.Second)},
	}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "assistant", results[0].Role)
	require.Equal(t, "search_files", results[0].ToolName)

	results, err = Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "plain",
		Role:      "user",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "user", results[0].Role)
}

func TestSearch_AssistantToolNameFilterDoesNotMatchOtherToolCallPayloads(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))

	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{
		{
			Role: handmsg.RoleAssistant,
			ToolCalls: []handmsg.ToolCall{
				{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
				{ID: "call-2", Name: "search_files", Input: `{"pattern":"needle"}`},
			},
			CreatedAt: time.Now().UTC(),
		},
	}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
		ToolName:  "process",
	})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestSearch_FiltersAndClampsResults(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))

	now := time.Now().UTC()
	messages := make([]handmsg.Message, 0, 25)
	for i := 0; i < 25; i++ {
		messages = append(messages, handmsg.Message{
			Role:       handmsg.RoleTool,
			Name:       "process",
			Content:    `{"status":"running"}`,
			ToolCallID: "call-1",
			CreatedAt:  now.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, messages))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID:  sessionSearchTestSessionID,
		Query:      "running",
		ToolName:   "process",
		MaxResults: 100,
	})
	require.NoError(t, err)
	require.Len(t, results, maxSessionSearchResults)
	require.True(t, results[0].CreatedAt > results[len(results)-1].CreatedAt)
}

func TestSearch_BuildsRuneSafeSnippet(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{{
		Role:      handmsg.RoleUser,
		Content:   "AéB needle C",
		CreatedAt: time.Now().UTC(),
	}}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "AéB needle C", results[0].Snippet)
}

func TestSearch_ReportsMatchIndexFromOriginalText(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))

	text := "İx needle"
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{{
		Role:      handmsg.RoleUser,
		Content:   text,
		CreatedAt: time.Now().UTC(),
	}}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, strings.Index(text, "needle"), results[0].MatchIndex)
	require.Equal(t, text, results[0].Snippet)
}

func TestSearch_ValidatesManagerAndQuery(t *testing.T) {
	_, err := Search(context.Background(), nil, envtypes.SessionSearchRequest{Query: "x"})
	require.EqualError(t, err, "session manager is required")

	store := memorystore.NewSessionStore()
	manager, createErr := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, createErr)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))

	_, err = Search(context.Background(), manager, envtypes.SessionSearchRequest{SessionID: sessionSearchTestSessionID})
	require.EqualError(t, err, "query is required")
}

func TestSearch_OmitsOriginSessionWhenSessionIDIsBlank(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: storage.DefaultSessionID}))
	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{{
		Role:      handmsg.RoleUser,
		Content:   "other needle",
		CreatedAt: time.Now().UTC(),
	}}))
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{{
		Role:      handmsg.RoleUser,
		Content:   "origin needle",
		CreatedAt: time.Now().UTC(),
	}}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		IgnoreSessionID: sessionSearchTestSessionID,
		Query:           "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "other needle", results[0].Snippet)
}

func TestSearch_ReturnsStoreErrorsAndSkipsEmptyDerivedSearchText(t *testing.T) {
	store := memorystore.NewSessionStore()
	manager, err := sessionstore.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)

	_, err = Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: "ses_invalid",
		Query:     "needle",
	})
	require.Error(t, err)

	require.NoError(t, manager.Save(context.Background(), memorystore.Session{ID: sessionSearchTestSessionID}))
	require.NoError(t, manager.AppendMessages(context.Background(), sessionSearchTestSessionID, []handmsg.Message{{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "process", Input: "{bad json"}},
		CreatedAt: time.Now().UTC(),
	}}))

	results, err := Search(context.Background(), manager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
	})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestSearch_ForwardsCanonicalSearchOptions(t *testing.T) {
	now := time.Now().UTC()
	mockManager, err := sessionstore.NewManager(&storagemock.SessionStore{
		SearchMessagesFunc: func(_ context.Context, id string, opts storage.SearchMessageOptions) ([]handmsg.Message, error) {
			require.Empty(t, id)
			require.Equal(t, storage.DefaultSessionID, opts.IgnoreSessionID)
			require.Equal(t, "needle", opts.Query)
			require.Equal(t, handmsg.RoleAssistant, opts.Role)
			require.Equal(t, "process", opts.ToolName)
			require.Equal(t, 2, opts.Limit)
			require.Zero(t, opts.Offset)

			return []handmsg.Message{{
				ID:   200,
				Role: handmsg.RoleAssistant,
				ToolCalls: []handmsg.ToolCall{{
					ID:    "call-1",
					Name:  "process",
					Input: `{"pattern":"needle"}`,
				}},
				CreatedAt: now,
			}}, nil
		},
	}, time.Minute, time.Hour)
	require.NoError(t, err)

	results, err := Search(context.Background(), mockManager, envtypes.SessionSearchRequest{
		IgnoreSessionID: storage.DefaultSessionID,
		Query:           "needle",
		Role:            "assistant",
		ToolName:        "process",
		MaxResults:      2,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "assistant", results[0].Role)
}

func TestSearch_ShapesStoreCandidatesWithoutFallbackMatching(t *testing.T) {
	mockManager, err := sessionstore.NewManager(&storagemock.SessionStore{
		SearchMessagesFunc: func(_ context.Context, _ string, _ storage.SearchMessageOptions) ([]handmsg.Message, error) {
			return []handmsg.Message{{
				ID:        1,
				Role:      handmsg.RoleUser,
				Content:   "stale candidate",
				CreatedAt: time.Now().UTC(),
			}}, nil
		},
	}, time.Minute, time.Hour)
	require.NoError(t, err)

	results, err := Search(context.Background(), mockManager, envtypes.SessionSearchRequest{
		SessionID: sessionSearchTestSessionID,
		Query:     "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "stale candidate", results[0].Snippet)
	require.Equal(t, -1, results[0].MatchIndex)
}

func TestCaseInsensitiveMatchIndex_AndSnippetAround_EdgeCases(t *testing.T) {
	index, length := caseInsensitiveMatchIndex("hello", "")
	require.Equal(t, -1, index)
	require.Zero(t, length)

	index, length = caseInsensitiveMatchIndex("hello", "zzz")
	require.Equal(t, -1, index)
	require.Zero(t, length)

	index, length = caseInsensitiveMatchIndex("Hello World", "world")
	require.Equal(t, strings.Index("Hello World", "World"), index)
	require.Equal(t, len("World"), length)

	require.Empty(t, snippetAround("", 0, 0, 10))
	require.Empty(t, snippetAround("hello", 0, 0, 0))

	zeroLenText := strings.Repeat("a", 30)
	zeroLenSnippet := snippetAround(zeroLenText, 10, 0, 10)
	require.NotEmpty(t, zeroLenSnippet)

	invalid := string([]byte{'a', 0xff, 'b'})
	require.Equal(t, "ab", snippetAround(invalid, 0, 1, 10))

	longText := strings.Repeat("a", 120) + "needle" + strings.Repeat("b", 120)
	snippet := snippetAround(longText, 120, len("needle"), 20)
	require.Contains(t, snippet, "needle")
	require.True(t, strings.HasPrefix(snippet, "..."))
	require.True(t, strings.HasSuffix(snippet, "..."))

	endText := strings.Repeat("a", 40) + "needle"
	endSnippet := snippetAround(endText, strings.Index(endText, "needle"), len("needle"), 20)
	require.Contains(t, endSnippet, "needle")
	require.True(t, strings.HasPrefix(endSnippet, "..."))
	require.False(t, strings.HasSuffix(endSnippet, "..."))
}

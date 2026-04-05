package inspect

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	handtrace "github.com/wandxy/hand/internal/trace"
)

func Test_Store_ListSessions_BuildsSummariesAndDetail(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "20260329T002738.170520000Z-4fca4857", []any{
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 38, 171258000, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
				Model:     "qwen/qwen3.5-27b",
				APIMode:   "chat-completions",
				Source:    "agent",
				TraceDir:  ".hand/traces",
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtUserMessageAccepted,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 38, 171671000, time.UTC),
			Payload:   map[string]any{"message": "List files in the root"},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtModelRequest,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 38, 171759000, time.UTC),
			Payload: models.Request{
				Model:        "qwen/qwen3.5-27b",
				APIMode:      "chat-completions",
				Instructions: "Daemon is the user's personal agent.",
				Messages: []handmsg.Message{
					{
						Role:      handmsg.RoleUser,
						Content:   "List files in the root",
						CreatedAt: time.Date(2026, 3, 29, 0, 27, 38, 171668000, time.UTC),
					},
				},
				Tools: []models.ToolDefinition{
					{Name: "list_files", Description: "List files and directories under an allowed workspace root."},
				},
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtModelResponse,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 41, 430260000, time.UTC),
			Payload: models.Response{
				ID:                "gen-1",
				Model:             "qwen/qwen3.5-27b-20260224",
				RequiresToolCalls: true,
				ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "list_files", Input: "{}"}},
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtModelRequest,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 45, 171759000, time.UTC),
			Payload: models.Request{
				Model:       "qwen/qwen3.5-27b",
				APIMode:     "chat-completions",
				Messages:    []handmsg.Message{{Role: handmsg.RoleTool, Content: `{"entries":[]}`}},
				Temperature: 0,
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtModelResponse,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 46, 430260000, time.UTC),
			Payload: models.Response{
				ID:         "gen-2",
				Model:      "qwen/qwen3.5-27b-20260224",
				OutputText: "Here are the files and directories in the root.",
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtToolInvocationStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 41, 430685000, time.UTC),
			Payload:   models.ToolCall{ID: "call-1", Name: "list_files", Input: "{}"},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtToolInvocationCompleted,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 41, 447625000, time.UTC),
			Payload: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "list_files",
				ToolCallID: "call-1",
				Content:    `{"name":"list_files","output":"{\"entries\":[]}"}`,
				CreatedAt:  time.Date(2026, 3, 29, 0, 27, 41, 447625000, time.UTC),
			},
		},
		handtrace.Event{
			SessionID: "4fca4857",
			Type:      handtrace.EvtFinalAssistantResponse,
			Timestamp: time.Date(2026, 3, 29, 0, 27, 47, 273707000, time.UTC),
			Payload:   map[string]any{"message": "Here are the files and directories in the root."},
		},
	})

	writeTraceFile(t, dir, "20260330T002738.170520000Z-failed", []any{
		handtrace.Event{
			SessionID: "failed",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 30, 0, 27, 38, 171258000, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
				Model:     "qwen/qwen3.5-27b",
				APIMode:   "chat-completions",
				Source:    "agent",
			},
		},
		handtrace.Event{
			SessionID: "failed",
			Type:      handtrace.EvtSessionFailed,
			Timestamp: time.Date(2026, 3, 30, 0, 27, 39, 171258000, time.UTC),
			Payload:   map[string]any{"error": "tool failed"},
		},
	})

	store := NewStore(dir)
	summaries, err := store.ListSessions()

	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, "failed", summaries[0].ID)
	require.Equal(t, "failed", summaries[0].FinalStatus)
	require.Equal(t, "4fca4857", summaries[1].ID)
	require.Equal(t, "completed", summaries[1].FinalStatus)
	require.Equal(t, "Daemon", summaries[1].AgentName)

	detail, err := store.GetSession("4fca4857")
	require.NoError(t, err)
	require.Empty(t, detail.LoadError)
	require.Equal(t, "completed", detail.Summary.FinalStatus)
	require.Len(t, detail.Timeline, 9)
	require.NotNil(t, detail.Timeline[2].ModelRequest)
	require.Equal(t, 1, detail.Timeline[2].ModelRequest.Sequence)
	require.Equal(t, 36, detail.Timeline[2].ModelRequest.Context.InstructionChars)
	require.Equal(t, 1, detail.Timeline[2].ModelRequest.Context.MessageCount)
	require.Equal(t, len("List files in the root"), detail.Timeline[2].ModelRequest.Context.MessageChars)
	require.Equal(t, 1, detail.Timeline[2].ModelRequest.Context.ToolCount)
	require.Equal(t, 0, detail.Timeline[2].ModelRequest.Context.ToolCallCount)
	require.NotNil(t, detail.Timeline[3].ModelResponse)
	require.Equal(t, 1, detail.Timeline[3].ModelResponse.Sequence)
	require.NotNil(t, detail.Timeline[4].ModelRequest)
	require.Equal(t, 2, detail.Timeline[4].ModelRequest.Sequence)
	require.NotNil(t, detail.Timeline[5].ModelResponse)
	require.Equal(t, 2, detail.Timeline[5].ModelResponse.Sequence)
	require.NotNil(t, detail.Timeline[6].ToolInvocation)
	require.NotNil(t, detail.Timeline[7].ToolInvocation)
	require.Equal(t, 7, *detail.Timeline[6].ToolInvocation.PairIndex)
	require.Equal(t, 6, *detail.Timeline[7].ToolInvocation.PairIndex)
}

func Test_Store_ListSessions_SortsTiesByIDDescending(t *testing.T) {
	dir := t.TempDir()
	timestamp := time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)
	writeTraceFile(t, dir, "aaa", []any{
		handtrace.Event{SessionID: "aaa", Type: handtrace.EvtChatStarted, Timestamp: timestamp, Payload: handtrace.Metadata{AgentName: "A"}},
	})
	writeTraceFile(t, dir, "zzz", []any{
		handtrace.Event{SessionID: "zzz", Type: handtrace.EvtChatStarted, Timestamp: timestamp, Payload: handtrace.Metadata{AgentName: "Z"}},
	})

	summaries, err := NewStore(dir).ListSessions()
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, "zzz", summaries[0].ID)
	require.Equal(t, "aaa", summaries[1].ID)
}

func Test_Store_ListSessions_SortsOlderSessionsAfterNewerOnComparatorReversePath(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "older", []any{
		handtrace.Event{SessionID: "older", Type: handtrace.EvtChatStarted, Timestamp: time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), Payload: handtrace.Metadata{AgentName: "Older"}},
	})
	writeTraceFile(t, dir, "newer", []any{
		handtrace.Event{SessionID: "newer", Type: handtrace.EvtChatStarted, Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), Payload: handtrace.Metadata{AgentName: "Newer"}},
	})
	writeTraceFile(t, dir, "newest", []any{
		handtrace.Event{SessionID: "newest", Type: handtrace.EvtChatStarted, Timestamp: time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC), Payload: handtrace.Metadata{AgentName: "Newest"}},
	})

	permutations := [][]string{
		{"older", "newer", "newest"},
		{"older", "newest", "newer"},
		{"newer", "older", "newest"},
		{"newer", "newest", "older"},
		{"newest", "older", "newer"},
		{"newest", "newer", "older"},
	}

	restoreReadDirectory(t)
	for _, ids := range permutations {
		readDirectory = func(string) ([]os.DirEntry, error) {
			entries := make([]os.DirEntry, 0, len(ids))
			for _, id := range ids {
				entries = append(entries, mustDirEntry(t, filepath.Join(dir, id+".jsonl")))
			}

			return entries, nil
		}

		summaries, err := NewStore(dir).ListSessions()
		require.NoError(t, err)
		require.Len(t, summaries, 3)
		require.Equal(t, "newest", summaries[0].ID)
		require.Equal(t, "newer", summaries[1].ID)
		require.Equal(t, "older", summaries[2].ID)
	}
}

func Test_LoadSessionFile_SurfacesMalformedJSONAsLoadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\n"), 0o600))

	detail, err := LoadSessionFile(path)

	require.NoError(t, err)
	require.Equal(t, "load_error", detail.Summary.FinalStatus)
	require.Contains(t, detail.LoadError, "failed to parse line 1")
}

func Test_LoadSessionFile_RecordsSessionIDMismatchAndSummaryFallback(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "mismatch", []any{
		handtrace.Event{
			SessionID: "different",
			Type:      handtrace.EvtSummaryFallbackStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload:   map[string]any{"remaining": 0},
		},
	})

	detail, err := LoadSessionFile(filepath.Join(dir, "mismatch.jsonl"))

	require.NoError(t, err)
	require.Len(t, detail.Warnings, 1)
	require.Contains(t, detail.Warnings[0], "does not match")
	require.NotNil(t, detail.Timeline[0].SummaryFallback)
}

func Test_LoadSessionFile_HandlesGenericPayloadsAndInvalidStructuredPayloads(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "generic", []any{
		handtrace.Event{
			SessionID: "generic",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload:   map[string]any{"agent_name": 99},
		},
		handtrace.Event{
			SessionID: "generic",
			Type:      "mystery.event",
			Timestamp: time.Date(2026, 3, 29, 0, 0, 1, 0, time.UTC),
			Payload:   map[string]any{"raw": true},
		},
	})

	detail, err := LoadSessionFile(filepath.Join(dir, "generic.jsonl"))

	require.NoError(t, err)
	require.Empty(t, detail.Timeline[0].StartedMetadata)
	require.Contains(t, detail.Timeline[0].GenericPayloadRaw, `"agent_name":99`)
	require.Contains(t, detail.Timeline[1].GenericPayloadRaw, `"raw":true`)
}

func Test_LoadSessionFile_ParsesContextSummaryAndCompactionEvents(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	writeTraceFile(t, dir, "memory-events", []any{
		handtrace.Event{
			SessionID: "memory-events",
			Type:      handtrace.EvtContextPreflight,
			Timestamp: now,
			Payload: map[string]any{
				"source":            "estimate",
				"prompt_tokens":     144,
				"context_limit":     1000,
				"trigger_threshold": 800,
				"warn_threshold":    650,
			},
		},
		handtrace.Event{
			SessionID: "memory-events",
			Type:      handtrace.EvtSummaryParseFailed,
			Timestamp: now.Add(time.Second),
			Payload: map[string]any{
				"session_id":           "memory-events",
				"source_end_offset":    14,
				"source_message_count": 22,
				"updated_at":           now.Add(time.Second),
				"error":                "summary requested tool calls",
			},
		},
		handtrace.Event{
			SessionID: "memory-events",
			Type:      handtrace.EvtContextCompactionRunning,
			Timestamp: now.Add(2 * time.Second),
			Payload: map[string]any{
				"session_id":           "memory-events",
				"status":               "running",
				"target_message_count": 22,
				"target_offset":        14,
				"requested_at":         now,
				"started_at":           now.Add(2 * time.Second),
			},
		},
	})

	detail, err := LoadSessionFile(filepath.Join(dir, "memory-events.jsonl"))

	require.NoError(t, err)
	require.Len(t, detail.Timeline, 3)
	require.NotNil(t, detail.Timeline[0].ContextEvent)
	require.Equal(t, "estimate", detail.Timeline[0].ContextEvent.Source)
	require.Equal(t, 144, detail.Timeline[0].ContextEvent.PromptTokens)
	require.Equal(t, 1000, detail.Timeline[0].ContextEvent.ContextLimit)
	require.Equal(t, 800, detail.Timeline[0].ContextEvent.TriggerThreshold)
	require.Equal(t, 650, detail.Timeline[0].ContextEvent.WarnThreshold)
	require.Empty(t, detail.Timeline[0].GenericPayloadRaw)

	require.NotNil(t, detail.Timeline[1].SummaryEvent)
	require.Equal(t, "memory-events", detail.Timeline[1].SummaryEvent.SessionID)
	require.Equal(t, 14, detail.Timeline[1].SummaryEvent.SourceEndOffset)
	require.Equal(t, 22, detail.Timeline[1].SummaryEvent.SourceMessageCount)
	require.Equal(t, "summary requested tool calls", detail.Timeline[1].SummaryEvent.Error)
	require.Empty(t, detail.Timeline[1].GenericPayloadRaw)

	require.NotNil(t, detail.Timeline[2].CompactionEvent)
	require.Equal(t, "memory-events", detail.Timeline[2].CompactionEvent.SessionID)
	require.Equal(t, "running", detail.Timeline[2].CompactionEvent.Status)
	require.Equal(t, 22, detail.Timeline[2].CompactionEvent.TargetMessageCount)
	require.Equal(t, 14, detail.Timeline[2].CompactionEvent.TargetOffset)
	require.True(t, detail.Timeline[2].CompactionEvent.RequestedAt.Equal(now))
	require.True(t, detail.Timeline[2].CompactionEvent.StartedAt.Equal(now.Add(2*time.Second)))
	require.Empty(t, detail.Timeline[2].GenericPayloadRaw)
}

func Test_App_Handler_ServesIndexAndSessionEndpoints(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "session", []any{
		handtrace.Event{
			SessionID: "session",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
				Model:     "model",
				APIMode:   "chat-completions",
			},
		},
	})

	app := NewApp(dir)
	handler := app.Handler()

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRec := httptest.NewRecorder()
	handler.ServeHTTP(indexRec, indexReq)
	require.Equal(t, http.StatusOK, indexRec.Code)
	require.Contains(t, indexRec.Body.String(), "Trace Viewer")

	listReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)
	require.Contains(t, listRec.Body.String(), "\"sessions\"")

	detailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/session", nil)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	require.Equal(t, http.StatusOK, detailRec.Code)
	require.Contains(t, detailRec.Body.String(), "\"summary\"")

	missingReq := httptest.NewRequest(http.MethodGet, "/api/sessions/missing", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	require.Equal(t, http.StatusNotFound, missingRec.Code)

	_, err := NewStore(dir).GetSession("../../etc/passwd")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func Test_App_Handler_RequiresBasicAuthWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "session", []any{
		handtrace.Event{
			SessionID: "session",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
				Model:     "model",
				APIMode:   "chat-completions",
			},
		},
	})

	app := NewApp(dir)
	app.SetBasicAuth("viewer", "secret")
	handler := app.Handler()

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	unauthorizedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRec, unauthorizedReq)
	require.Equal(t, http.StatusUnauthorized, unauthorizedRec.Code)
	require.Contains(t, unauthorizedRec.Header().Get("WWW-Authenticate"), "Basic")

	authorizedReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	authorizedReq.SetBasicAuth("viewer", "secret")
	authorizedRec := httptest.NewRecorder()
	handler.ServeHTTP(authorizedRec, authorizedReq)
	require.Equal(t, http.StatusOK, authorizedRec.Code)
	require.Contains(t, authorizedRec.Body.String(), "\"sessions\"")
}

func Test_Store_ValidateAndResolvePath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "trace.jsonl")
	require.NoError(t, os.WriteFile(filePath, []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o600))

	require.EqualError(t, (*Store)(nil).Validate(), "trace directory is required")
	require.EqualError(t, NewStore(" ").Validate(), "trace directory is required")
	require.EqualError(t, NewStore(filepath.Join(dir, "missing")).Validate(), `trace directory "`+filepath.Join(dir, "missing")+`" does not exist`)
	require.EqualError(t, NewStore(filePath).Validate(), `trace directory "`+filePath+`" is not a directory`)
	require.NoError(t, NewStore(dir).Validate())

	restoreStatPath(t)
	statPath = func(path string) (os.FileInfo, error) {
		if path == dir {
			return nil, fs.ErrPermission
		}

		return os.Stat(path)
	}
	require.ErrorContains(t, NewStore(dir).Validate(), "failed to access trace directory")

	_, err := resolveSessionPath("", "session")
	require.ErrorIs(t, err, os.ErrNotExist)

	validPath, err := resolveSessionPath(dir, "session")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "session.jsonl"), validPath)

	for _, id := range []string{"", " ", ".", "..", "../etc/passwd", `..\etc\passwd`, "nested/file", `nested\file`} {
		_, err = resolveSessionPath(dir, id)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func Test_Store_ListSessions_IgnoresNonJSONLAndGetSessionErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("ignore"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	writeTraceFile(t, dir, "session", []any{
		handtrace.Event{
			SessionID: "session",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
			},
		},
	})

	store := NewStore(dir)
	summaries, err := store.ListSessions()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, filepath.Join(dir, "session.jsonl"), summaries[0].Path)

	_, err = store.GetSession("missing")
	require.ErrorIs(t, err, os.ErrNotExist)

	restoreReadDirectory(t)
	readDirectory = func(string) ([]os.DirEntry, error) {
		return nil, fs.ErrPermission
	}
	_, err = store.ListSessions()
	require.ErrorContains(t, err, "failed to read trace directory")

	restoreStatPath(t)
	sessionPath := filepath.Join(dir, "session.jsonl")
	statPath = func(path string) (os.FileInfo, error) {
		if path == sessionPath {
			return nil, fs.ErrPermission
		}

		return os.Stat(path)
	}
	_, err = store.GetSession("session")
	require.ErrorIs(t, err, fs.ErrPermission)

	restoreStatPath(t)
	readDirectory = os.ReadDir
	statPath = func(path string) (os.FileInfo, error) {
		if path == sessionPath {
			return nil, fs.ErrInvalid
		}

		return os.Stat(path)
	}
	_, err = store.ListSessions()
	require.ErrorIs(t, err, fs.ErrInvalid)

	restoreOpenPath(t)
	openPath = func(path string) (io.ReadCloser, error) {
		if path == sessionPath {
			return nil, fs.ErrInvalid
		}

		return os.Open(path)
	}
	_, err = store.ListSessions()
	require.ErrorIs(t, err, fs.ErrInvalid)
}

func Test_Utility_Helpers(t *testing.T) {
	require.Nil(t, buildToolCallViewsFromContext(nil))
	require.Equal(t, []ToolCallView{{
		ID:    "call-1",
		Name:  "list_files",
		Input: "{}",
	}}, buildToolCallViewsFromContext([]handmsg.ToolCall{{
		ID:    "call-1",
		Name:  "list_files",
		Input: "{}",
	}}))

	require.Equal(t, "", compactJSON(nil))
	require.Equal(t, "not-json", compactJSON([]byte(" not-json ")))
	require.Equal(t, `{"a":1}`, compactJSON([]byte("{\n  \"a\": 1\n}")))
}

func Test_PairToolInvocations_IgnoresUnmatchedAndBlankIDs(t *testing.T) {
	timeline := []TimelineEvent{
		{ToolInvocation: &ToolInvocationView{Phase: "completed", ID: "missing"}},
		{ToolInvocation: &ToolInvocationView{Phase: "started", ID: "call-1"}},
		{ToolInvocation: &ToolInvocationView{Phase: "started", ID: " "}},
		{ToolInvocation: &ToolInvocationView{Phase: "completed", ID: "call-1"}},
	}

	pairToolInvocations(timeline)

	require.Nil(t, timeline[0].ToolInvocation.PairIndex)
	require.NotNil(t, timeline[1].ToolInvocation.PairIndex)
	require.Equal(t, 3, *timeline[1].ToolInvocation.PairIndex)
	require.Nil(t, timeline[2].ToolInvocation.PairIndex)
	require.NotNil(t, timeline[3].ToolInvocation.PairIndex)
	require.Equal(t, 1, *timeline[3].ToolInvocation.PairIndex)
}

func Test_App_AndAuth_Helpers(t *testing.T) {
	require.EqualError(t, (*App)(nil).Validate(), "trace app is required")

	app := &App{}
	require.EqualError(t, app.Validate(), "trace app is required")

	var nilApp *App
	nilApp.SetBasicAuth("user", "secret")
	require.False(t, nilApp.requiresAuth())
	require.True(t, nilApp.authorized(httptest.NewRequest(http.MethodGet, "/", nil)))

	app.SetBasicAuth(" viewer ", "secret")
	require.True(t, app.requiresAuth())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	require.False(t, app.authorized(req))

	req.SetBasicAuth("wrong", "secret")
	require.False(t, app.authorized(req))

	req.SetBasicAuth("viewer", "wrong")
	require.False(t, app.authorized(req))

	req.SetBasicAuth("viewer", "secret")
	require.True(t, app.authorized(req))
	require.NoError(t, NewApp(t.TempDir()).Validate())
}

func Test_App_Handler_ErrorPaths(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing")
	app := NewApp(missingDir)
	handler := app.Handler()

	listReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusInternalServerError, listRec.Code)
	require.Contains(t, listRec.Body.String(), "does not exist")

	emptyDetailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/", nil)
	emptyDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(emptyDetailRec, emptyDetailReq)
	require.Equal(t, http.StatusNotFound, emptyDetailRec.Code)

	detailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/session", nil)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	require.Equal(t, http.StatusInternalServerError, detailRec.Code)

	restoreReadAssetFile(t)
	readAssetFile = func(_ fs.FS, _ string) ([]byte, error) {
		return nil, fs.ErrNotExist
	}
	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRec := httptest.NewRecorder()
	NewApp(t.TempDir()).Handler().ServeHTTP(indexRec, indexReq)
	require.Equal(t, http.StatusInternalServerError, indexRec.Code)
}

func Test_LoadSessionFile_InputAndScannerErrors(t *testing.T) {
	_, err := LoadSessionFile(" ")
	require.EqualError(t, err, "trace session path is required")

	restoreStatPath(t)
	statPath = func(string) (os.FileInfo, error) {
		return nil, fs.ErrPermission
	}
	_, err = LoadSessionFile("blocked.jsonl")
	require.ErrorIs(t, err, fs.ErrPermission)

	dir := t.TempDir()
	path := filepath.Join(dir, "huge.jsonl")
	var line bytes.Buffer
	line.WriteString(`{"session_id":"huge","type":"chat.started","timestamp":"2026-03-29T00:00:00Z","payload":"`)
	line.WriteString(strings.Repeat("x", 8*1024*1024+1))
	line.WriteString("\"}\n")
	require.NoError(t, os.WriteFile(path, line.Bytes(), 0o600))

	_, err = LoadSessionFile(path)
	require.Error(t, err)

	restoreStatPath(t)
	restoreOpenPath(t)
	statPath = os.Stat
	openPath = func(string) (io.ReadCloser, error) {
		return nil, fs.ErrPermission
	}
	_, err = LoadSessionFile(path)
	require.ErrorIs(t, err, fs.ErrPermission)

	restoreStatPath(t)
	restoreOpenPath(t)
	statPath = os.Stat
	openPath = func(string) (io.ReadCloser, error) {
		return io.NopCloser(&failingReader{}), nil
	}
	_, err = LoadSessionFile(path)
	require.ErrorIs(t, err, fs.ErrInvalid)
}

func Test_LoadSessionFile_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blank.jsonl")
	content := "\n\n" + `{"session_id":"blank","type":"summary.fallback.started","timestamp":"2026-03-29T00:00:00Z","payload":{"remaining":0}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	detail, err := LoadSessionFile(path)
	require.NoError(t, err)
	require.Equal(t, 1, detail.Summary.EventCount)
	require.Len(t, detail.Timeline, 1)
}

func Test_ApplyEvent_PreservesSummaryAndFallsBackToGenericPayload(t *testing.T) {
	detail := SessionDetail{
		Summary: SessionSummary{
			Model:       "existing-model",
			APIMode:     "responses",
			FinalStatus: "incomplete",
		},
	}
	timelineEvent := TimelineEvent{}

	requestPayload, err := json.Marshal(models.Request{
		Model:   "new-model",
		APIMode: "chat-completions",
	})
	require.NoError(t, err)

	applyEvent(&detail, &timelineEvent, rawEvent{
		Type:    handtrace.EvtModelRequest,
		Payload: requestPayload,
	})
	require.Equal(t, "existing-model", detail.Summary.Model)
	require.Equal(t, "responses", detail.Summary.APIMode)
	require.NotNil(t, timelineEvent.ModelRequest)

	timelineEvent = TimelineEvent{}
	applyEvent(&detail, &timelineEvent, rawEvent{
		Type:    handtrace.EvtFinalAssistantResponse,
		Payload: []byte(`{"message":1}`),
	})
	require.Nil(t, timelineEvent.FinalResponse)
	require.Contains(t, timelineEvent.GenericPayloadRaw, `"message":1`)

	detail = SessionDetail{Summary: SessionSummary{FinalStatus: "incomplete"}}
	timelineEvent = TimelineEvent{}
	applyEvent(&detail, &timelineEvent, rawEvent{
		Type:    handtrace.EvtModelRequest,
		Payload: requestPayload,
	})
	require.Equal(t, "new-model", detail.Summary.Model)
	require.Equal(t, "chat-completions", detail.Summary.APIMode)
}

func Test_App_Handler_HandleSessionPermissionAndInternalErrors(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "session", []any{
		handtrace.Event{
			SessionID: "session",
			Type:      handtrace.EvtChatStarted,
			Timestamp: time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC),
			Payload: handtrace.Metadata{
				AgentName: "Daemon",
			},
		},
	})

	sessionPath := filepath.Join(dir, "session.jsonl")
	restoreStatPath(t)
	statPath = func(path string) (os.FileInfo, error) {
		if path == sessionPath {
			return nil, fs.ErrPermission
		}

		return os.Stat(path)
	}

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/sessions/session", nil)
	forbiddenRec := httptest.NewRecorder()
	NewApp(dir).Handler().ServeHTTP(forbiddenRec, forbiddenReq)
	require.Equal(t, http.StatusForbidden, forbiddenRec.Code)

	restoreStatPath(t)
	statPath = func(path string) (os.FileInfo, error) {
		if path == sessionPath {
			return nil, fs.ErrInvalid
		}

		return os.Stat(path)
	}

	internalReq := httptest.NewRequest(http.MethodGet, "/api/sessions/session", nil)
	internalRec := httptest.NewRecorder()
	NewApp(dir).Handler().ServeHTTP(internalRec, internalReq)
	require.Equal(t, http.StatusInternalServerError, internalRec.Code)
}

func restoreStatPath(t *testing.T) {
	t.Helper()
	original := statPath
	t.Cleanup(func() {
		statPath = original
	})
}

func restoreReadDirectory(t *testing.T) {
	t.Helper()
	original := readDirectory
	t.Cleanup(func() {
		readDirectory = original
	})
}

func restoreOpenPath(t *testing.T) {
	t.Helper()
	original := openPath
	t.Cleanup(func() {
		openPath = original
	})
}

func restoreReadAssetFile(t *testing.T) {
	t.Helper()
	original := readAssetFile
	t.Cleanup(func() {
		readAssetFile = original
	})
}

func mustDirEntry(t *testing.T, path string) os.DirEntry {
	t.Helper()
	entry, err := os.Stat(path)
	require.NoError(t, err)
	return statDirEntry{info: entry}
}

type statDirEntry struct {
	info os.FileInfo
}

func (e statDirEntry) Name() string               { return e.info.Name() }
func (e statDirEntry) IsDir() bool                { return e.info.IsDir() }
func (e statDirEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e statDirEntry) Info() (os.FileInfo, error) { return e.info, nil }

type failingReader struct {
	called bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if !r.called {
		r.called = true
		p[0] = '\n'
		return 1, nil
	}

	return 0, fs.ErrInvalid
}

func writeTraceFile(t *testing.T, dir, id string, events []any) {
	t.Helper()

	path := filepath.Join(dir, id+".jsonl")
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, event := range events {
		require.NoError(t, encoder.Encode(event))
	}
}

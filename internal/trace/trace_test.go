package trace

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

const testTraceSessionID = "ses_testtraceid"

func TestJSONLFactory_OpenSessionCreatesSessionAndWritesEvents(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())

	session := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{AgentName: "hand", Model: "gpt-5.1", APIMode: "responses", Source: "agent"})
	require.Equal(t, testTraceSessionID, session.ID())
	session.Record(EvtModelRequest, map[string]any{"authorization": "Bearer secret", "message": "hello"})
	session.Close()

	matches, err := filepath.Glob(filepath.Join(dir, "*"+testTraceSessionID+".jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	file, err := os.Open(matches[0])
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var events []Event
	for scanner.Scan() {
		var event Event
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &event))
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	require.Len(t, events, 2)
	require.Equal(t, EvtChatStarted, events[0].Type)
	require.Equal(t, EvtModelRequest, events[1].Type)
	payload := events[1].Payload.(map[string]any)
	require.Equal(t, "[REDACTED]", payload["authorization"])
	require.Equal(t, "hello", payload["message"])
}

func TestJSONLFactory_OpenSessionSecondOpenAppendsWithoutDuplicateChatStarted(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())
	meta := Metadata{AgentName: "hand", Model: "m", APIMode: "responses", Source: "agent"}

	s1 := factory.OpenSession(context.Background(), testTraceSessionID, meta)
	s1.Record(EvtModelRequest, map[string]any{"n": 1})
	s1.Close()

	s2 := factory.OpenSession(context.Background(), testTraceSessionID, meta)
	s2.Record(EvtModelResponse, map[string]any{"ok": true})
	s2.Close()

	matches, err := filepath.Glob(filepath.Join(dir, "*"+testTraceSessionID+".jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 3)

	var types []string
	for _, line := range lines {
		var event Event
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		types = append(types, event.Type)
	}
	require.Equal(t, []string{EvtChatStarted, EvtModelRequest, EvtModelResponse}, types)
}

func TestJSONLFactory_OpenSessionReturnsNoopWhenDirectoryIsEmpty(t *testing.T) {
	factory := NewFactory("", guardrails.NewRedactor())
	session := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
	session.Record("ignored", nil)
	session.Close()
}

func TestJSONLFactory_OpenSessionReturnsNoopWhenSessionIDInvalid(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		session := factory.OpenSession(context.Background(), id, Metadata{})
		require.Equal(t, "", session.ID(), id)
		session.Close()
	}
}

func TestNoopFactory_OpenSessionReturnsSession(t *testing.T) {
	session := NoopFactory().OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
	session.Record("ignored", map[string]any{"x": 1})
	session.Close()
}

func TestJSONLSession_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	session := NewFactory(dir, guardrails.NewRedactor()).OpenSession(context.Background(), testTraceSessionID, Metadata{})
	session.Close()
	session.Close()
}

func TestJSONLFactory_OpenSessionReturnsNoopWhenDirectoryInitializationFails(t *testing.T) {
	dir := t.TempDir()
	blockedPath := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))

	session := NewFactory(filepath.Join(blockedPath, "child"), guardrails.NewRedactor()).
		OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLSession_RecordAfterCloseIsIgnored(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())
	session := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{})
	session.Close()
	session.Record("ignored", map[string]any{"x": 1})

	matches, err := filepath.Glob(filepath.Join(dir, "*"+testTraceSessionID+".jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	content, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Equal(t, 1, len(strings.Split(strings.TrimSpace(string(content)), "\n")))
}

func TestJSONLSession_IDHandlesNilReceiver(t *testing.T) {
	var session *jsonlSession
	require.Equal(t, "", session.ID())
}

func TestNewFactory_UsesDefaultRedactor(t *testing.T) {
	factory := NewFactory(t.TempDir(), nil)
	require.NotNil(t, factory.redactor)
}

func TestJSONLFactory_OpenSessionHandlesNilReceiver(t *testing.T) {
	var factory *JSONLFactory
	session := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLFactory_OpenSessionReturnsNoopWhenOpenFails(t *testing.T) {
	originalOpen := openTraceFile
	openTraceFile = func(string, int, os.FileMode) (*os.File, error) {
		return nil, os.ErrPermission
	}
	defer func() {
		openTraceFile = originalOpen
	}()

	session := NewFactory(t.TempDir(), guardrails.NewRedactor()).
		OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLFactory_OpenSessionReturnsNoopWhenMkdirFails(t *testing.T) {
	originalMkdirAll := mkdirAll
	mkdirAll = func(string, os.FileMode) error {
		return os.ErrPermission
	}
	defer func() {
		mkdirAll = originalMkdirAll
	}()

	session := NewFactory(t.TempDir(), guardrails.NewRedactor()).
		OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", session.ID())
}

func TestNoopSession_NoOps(t *testing.T) {
	session := noopSession{}
	require.Equal(t, "", session.ID())
	session.Record("ignored", map[string]any{"x": 1})
	session.Close()
}

func TestJSONLSession_RecordHandlesNilReceiverAndNoop(t *testing.T) {
	var session *jsonlSession
	session.Record("ignored", nil)

	noop := &jsonlSession{noop: true}
	noop.Record("ignored", nil)
}

func TestJSONLSession_RecordHandlesEncoderError(t *testing.T) {
	dir := t.TempDir()
	session := NewFactory(dir, guardrails.NewRedactor()).
		OpenSession(context.Background(), testTraceSessionID, Metadata{}).(*jsonlSession)
	require.NoError(t, session.file.Close())
	session.Record("broken", map[string]any{"x": 1})
}

func TestJSONLSession_CloseHandlesNilReceiverAndNoop(t *testing.T) {
	var session *jsonlSession
	session.Close()

	noop := &jsonlSession{noop: true}
	noop.Close()
}

func TestJSONLSession_CloseHandlesFileCloseError(t *testing.T) {
	dir := t.TempDir()
	session := NewFactory(dir, guardrails.NewRedactor()).
		OpenSession(context.Background(), testTraceSessionID, Metadata{}).(*jsonlSession)
	require.NoError(t, session.file.Close())
	session.Close()
}

func TestSessionIDFromTraceFilename(t *testing.T) {
	require.Equal(t, "ses_abc", SessionIDFromTraceFilename("20060102T150405.000000000Z-ses_abc"))
	require.Equal(t, "plainstem", SessionIDFromTraceFilename("plainstem"))
}

func TestResolveTraceFilePath_TimePrefixedFile(t *testing.T) {
	dir := t.TempDir()
	name := "20260102T000000.000000000Z-ses_x.jsonl"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	p, err := ResolveTraceFilePath(dir, "ses_x")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, name), p)
}

func TestResolveTraceFilePath_NotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveTraceFilePath(dir, "ses_missing")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestResolveTraceFilePath_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a-ses_x.jsonl"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b-ses_x.jsonl"), []byte("y"), 0o644))
	_, err := ResolveTraceFilePath(dir, "ses_x")
	require.ErrorIs(t, err, ErrAmbiguousTraceFiles)
}

func TestResolveTraceFilePath_GlobError(t *testing.T) {
	orig := globTraceFiles
	globTraceFiles = func(string) ([]string, error) {
		return nil, os.ErrPermission
	}
	defer func() { globTraceFiles = orig }()

	_, err := ResolveTraceFilePath(t.TempDir(), testTraceSessionID)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestResolveTraceFilePath_EmptyDirectory(t *testing.T) {
	_, err := ResolveTraceFilePath("", testTraceSessionID)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestResolveTraceFilePath_WhitespaceDirectory(t *testing.T) {
	_, err := ResolveTraceFilePath("   ", testTraceSessionID)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestJSONLFactory_OpenSessionGlobErrorReturnsNoop(t *testing.T) {
	dir := t.TempDir()
	orig := globTraceFiles
	globTraceFiles = func(string) ([]string, error) {
		return nil, os.ErrPermission
	}
	defer func() { globTraceFiles = orig }()

	factory := NewFactory(dir, guardrails.NewRedactor())
	s := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", s.ID())
	s.Close()
}

func TestJSONLFactory_OpenSessionStatErrorReturnsNoop(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())

	origStat := statOpenedFile
	statOpenedFile = func(*os.File) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	defer func() { statOpenedFile = origStat }()

	s := factory.OpenSession(context.Background(), testTraceSessionID, Metadata{})
	require.Equal(t, "", s.ID())
	s.Close()
}

func TestJSONLSession_CloseTraceFileError(t *testing.T) {
	dir := t.TempDir()
	session := NewFactory(dir, guardrails.NewRedactor()).OpenSession(context.Background(), testTraceSessionID, Metadata{}).(*jsonlSession)

	origClose := closeTraceFile
	closeTraceFile = func(*os.File) error {
		return os.ErrPermission
	}
	defer func() { closeTraceFile = origClose }()

	session.Close()
}

func TestJSONLFactory_OpenSessionAmbiguousReturnsNoop(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a-ses_amb.jsonl"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b-ses_amb.jsonl"), []byte("y"), 0o644))
	factory := NewFactory(dir, guardrails.NewRedactor())
	s := factory.OpenSession(context.Background(), "ses_amb", Metadata{})
	require.Equal(t, "", s.ID())
	s.Close()
}

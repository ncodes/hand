package trace

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

func TestJSONLFactory_NewSessionCreatesSessionAndWritesEvents(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())
	factory.now = func() time.Time { return time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC) }

	session := factory.NewSession(context.Background(), Metadata{AgentName: "hand", Model: "gpt-5.1", APIMode: "responses", Source: "agent"})
	require.NotEmpty(t, session.ID())
	session.Record(EvtModelRequest, map[string]any{"authorization": "Bearer secret", "message": "hello"})
	session.Close()

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	file, err := os.Open(files[0])
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

func TestJSONLFactory_NewSessionReturnsNoopWhenDirectoryIsEmpty(t *testing.T) {
	factory := NewFactory("", guardrails.NewRedactor())
	session := factory.NewSession(context.Background(), Metadata{})
	require.Equal(t, "", session.ID())
	session.Record("ignored", nil)
	session.Close()
}

func TestNoopFactory_NewSessionReturnsSession(t *testing.T) {
	session := NoopFactory().NewSession(context.Background(), Metadata{})
	require.Equal(t, "", session.ID())
	session.Record("ignored", map[string]any{"x": 1})
	session.Close()
}

func TestJSONLSession_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	session := NewFactory(dir, guardrails.NewRedactor()).NewSession(context.Background(), Metadata{})
	session.Close()
	session.Close()
}

func TestJSONLFactory_NewSessionReturnsNoopWhenDirectoryInitializationFails(t *testing.T) {
	dir := t.TempDir()
	blockedPath := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blockedPath, []byte("x"), 0o600))

	session := NewFactory(filepath.Join(blockedPath, "child"), guardrails.NewRedactor()).NewSession(context.Background(), Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLSession_RecordAfterCloseIsIgnored(t *testing.T) {
	dir := t.TempDir()
	factory := NewFactory(dir, guardrails.NewRedactor())
	session := factory.NewSession(context.Background(), Metadata{})
	session.Close()
	session.Record("ignored", map[string]any{"x": 1})

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
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

func TestJSONLFactory_NewSessionHandlesNilReceiver(t *testing.T) {
	var factory *JSONLFactory
	session := factory.NewSession(context.Background(), Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLFactory_NewSessionReturnsNoopWhenCreateFails(t *testing.T) {
	originalCreateFile := createFile
	createFile = func(string) (*os.File, error) {
		return nil, os.ErrPermission
	}
	defer func() {
		createFile = originalCreateFile
	}()

	session := NewFactory(t.TempDir(), guardrails.NewRedactor()).NewSession(context.Background(), Metadata{})
	require.Equal(t, "", session.ID())
}

func TestJSONLFactory_NewSessionReturnsNoopWhenMkdirFails(t *testing.T) {
	originalMkdirAll := mkdirAll
	mkdirAll = func(string, os.FileMode) error {
		return os.ErrPermission
	}
	defer func() {
		mkdirAll = originalMkdirAll
	}()

	session := NewFactory(t.TempDir(), guardrails.NewRedactor()).NewSession(context.Background(), Metadata{})
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
	session := NewFactory(dir, guardrails.NewRedactor()).NewSession(context.Background(), Metadata{}).(*jsonlSession)
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
	session := NewFactory(dir, guardrails.NewRedactor()).NewSession(context.Background(), Metadata{}).(*jsonlSession)
	require.NoError(t, session.file.Close())
	session.Close()
}

func TestRandomSuffix_FallsBack(t *testing.T) {
	originalReadRandom := readRandom
	readRandom = func([]byte) (int, error) {
		return 0, os.ErrPermission
	}
	defer func() {
		readRandom = originalReadRandom
	}()

	require.Equal(t, "trace", randomSuffix())
}

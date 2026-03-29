package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handtrace "github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

func TestNewCommand_ShowsHelpWithoutSubcommand(t *testing.T) {
	cmd := NewCommand()
	cmd.Writer = &bytes.Buffer{}
	cmd.ErrWriter = &bytes.Buffer{}
	err := cmd.Run(context.Background(), []string{"trace"})
	require.NoError(t, err)
}

func TestViewCommand_ServesTraceViewerAndPrintsURL(t *testing.T) {
	dir := t.TempDir()
	writeTraceSession(t, dir, "session")

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--trace-dir", dir, "--listen", "127.0.0.1:0"})
	}()

	url := waitForURL(t, &logs)
	resp, err := http.Get(url + "/api/sessions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	require.NoError(t, <-errCh)
}

func TestViewCommand_UsesExplicitTraceDir(t *testing.T) {
	dir := t.TempDir()
	writeTraceSession(t, dir, "session")

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{
			"trace", "view",
			"--trace-dir", dir,
			"--listen", "127.0.0.1:0",
		})
	}()

	url := waitForURL(t, &logs)
	resp, err := http.Get(url + "/api/sessions/session")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	require.NoError(t, <-errCh)
}

func TestViewCommand_UsesDefaultTraceDirFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	traceDir := filepath.Join(home, "traces")
	require.NoError(t, os.MkdirAll(traceDir, 0o755))
	writeTraceSession(t, traceDir, "session")

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--listen", "127.0.0.1:0"})
	}()

	url := waitForURL(t, &logs)
	resp, err := http.Get(url + "/api/sessions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Sessions []map[string]any `json:"sessions"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Sessions, 1)

	cancel()
	require.NoError(t, <-errCh)
}

func TestViewCommand_ReturnsMissingDirectoryError(t *testing.T) {
	restoreLogs(t, io.Discard)
	missingDir := filepath.Join(t.TempDir(), "missing")
	err := NewCommand().Run(context.Background(), []string{"trace", "view", "--trace-dir", missingDir})
	require.EqualError(t, err, `trace directory "`+missingDir+`" does not exist`)
}

func TestViewCommand_LogsResolvedArguments(t *testing.T) {
	dir := t.TempDir()
	writeTraceSession(t, dir, "session")

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--trace-dir", dir, "--listen", "127.0.0.1:0"})
	}()

	url := waitForURL(t, &logs)
	require.Contains(t, logs.String(), "Starting trace viewer")
	require.Contains(t, logs.String(), dir)
	require.Contains(t, logs.String(), "listen=127.0.0.1:0")

	resp, err := http.Get(url + "/api/sessions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	require.NoError(t, <-errCh)
	require.Contains(t, logs.String(), "Trace viewer listening")
	require.Contains(t, logs.String(), url)
}

func TestViewCommand_RequiresBasicAuthWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	writeTraceSession(t, dir, "session")

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{
			"trace", "view",
			"--trace-dir", dir,
			"--listen", "127.0.0.1:0",
			"--username", "viewer",
			"--password", "secret",
		})
	}()

	url := waitForURL(t, &logs)

	req, err := http.NewRequest(http.MethodGet, url+"/api/sessions", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, err = http.NewRequest(http.MethodGet, url+"/api/sessions", nil)
	require.NoError(t, err)
	req.SetBasicAuth("viewer", "secret")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	require.NoError(t, <-errCh)
}

func TestViewCommand_RejectsPartialBasicAuthConfiguration(t *testing.T) {
	restoreLogs(t, io.Discard)
	dir := t.TempDir()
	writeTraceSession(t, dir, "session")

	err := NewCommand().Run(context.Background(), []string{
		"trace", "view",
		"--trace-dir", dir,
		"--username", "viewer",
	})
	require.EqualError(t, err, "trace viewer basic auth requires both username and password")
}

func waitForURL(t *testing.T, logs *bytes.Buffer) string {
	t.Helper()
	urlPattern := regexp.MustCompile(`url=(http://\S+)`)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		value := strings.TrimSpace(logs.String())
		if matches := urlPattern.FindStringSubmatch(value); len(matches) == 2 {
			return matches[1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for server URL")
	return ""
}

func writeTraceSession(t *testing.T, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	encoder := json.NewEncoder(file)
	require.NoError(t, encoder.Encode(handtrace.Event{
		SessionID: id,
		Type:      "chat.started",
		Timestamp: time.Now().UTC(),
		Payload: handtrace.Metadata{
			AgentName: "Daemon",
			Model:     "model",
			APIMode:   "chat-completions",
		},
	}))
}

func restoreLogs(t *testing.T, out io.Writer) {
	t.Helper()
	logutils.SetOutput(out)
	t.Cleanup(func() {
		logutils.SetOutput(os.Stderr)
	})
}

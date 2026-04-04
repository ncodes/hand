package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	listenAddr := reserveListenAddress(t)

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--trace-dir", dir, "--listen", listenAddr})
	}()

	url := "http://" + listenAddr
	waitForServer(t, url+"/api/sessions")
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
	listenAddr := reserveListenAddress(t)

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{
			"trace", "view",
			"--trace-dir", dir,
			"--listen", listenAddr,
		})
	}()

	url := "http://" + listenAddr
	waitForServer(t, url+"/api/sessions/session")
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
	listenAddr := reserveListenAddress(t)

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--listen", listenAddr})
	}()

	url := "http://" + listenAddr
	waitForServer(t, url+"/api/sessions")
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
	listenAddr := reserveListenAddress(t)

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{"trace", "view", "--trace-dir", dir, "--listen", listenAddr})
	}()

	url := "http://" + listenAddr
	waitForServer(t, url+"/api/sessions")
	require.Contains(t, logs.String(), "Starting trace viewer")
	require.Contains(t, logs.String(), dir)
	require.Contains(t, logs.String(), "listen="+listenAddr)

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
	listenAddr := reserveListenAddress(t)

	var logs bytes.Buffer
	restoreLogs(t, &logs)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewCommand().Run(ctx, []string{
			"trace", "view",
			"--trace-dir", dir,
			"--listen", listenAddr,
			"--username", "viewer",
			"--password", "secret",
		})
	}()

	url := "http://" + listenAddr
	waitForServer(t, url+"/api/sessions")

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

func reserveListenAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}

func waitForServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for server %s", url)
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
		Type:      handtrace.EvtChatStarted,
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
	t.Setenv("LOG_NO_COLOR", "true")
	logutils.SetOutput(out)
	t.Cleanup(func() {
		logutils.SetOutput(os.Stderr)
	})
}

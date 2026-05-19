package session

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/runtime"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
)

func TestNewCommandSessionNewCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{CreatedSession: storage.Session{ID: "project-a"}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "new", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a\n", output.String())
}

func TestNewCommandSessionListCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{Sessions: []storage.Session{
		{ID: "default", Title: "Daily Planning", TitleSource: storage.SessionTitleSourceGenerated},
		{ID: "project-a"},
	}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "list"})

	require.NoError(t, err)
	require.Equal(t, "Daily Planning (default)\nproject-a\n", output.String())
}

func TestNewCommandSessionListUsesProfileRuntimeEndpoint(t *testing.T) {
	originalNewClient := newClient
	originalOutput := sessionOutput
	originalProfile := profile.Active()
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
		profile.SetActive(originalProfile)
	})

	home := t.TempDir()
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte("models:\n  verify: false\n"), 0o600))
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: profileHome}))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
	})
	port := listener.Addr().(*net.TCPAddr).Port
	_, err = runtime.WriteActive("127.0.0.1", port)
	require.NoError(t, err)

	sessionOutput = io.Discard
	stub := &agentstub.AgentServiceStub{Sessions: []storage.Session{{ID: "default"}}}
	var got *config.Config
	newClient = func(_ context.Context, cfg *config.Config) (rpcclient.SessionClient, error) {
		got = cfg
		return stub, nil
	}

	err = NewCommand().Run(context.Background(), []string{"session", "list"})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "127.0.0.1", got.RPC.Address)
	require.Equal(t, port, got.RPC.Port)
}

func TestNewCommandSessionCurrentCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{CurrentSessionID: storage.DefaultSessionID}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "current"})

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID+"\n", output.String())
}

func TestNewCommandSessionUseCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "use", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.UsedSessionID)
	require.Equal(t, "project-a\n", output.String())
}

func TestNewCommandSessionCompactCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{CompactResult: rpcclient.CompactSessionResult{
		SessionID:            "project-a",
		SourceEndOffset:      12,
		SourceMessageCount:   20,
		UpdatedAt:            time.Unix(123, 0).UTC(),
		CurrentContextLength: 4000,
		TotalContextLength:   128000,
	}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "compact", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "id=project-a source_end_offset=12 source_message_count=20 updated_at=1970-01-01T00:02:03Z current_context_length=4000 total_context_length=128000\n", output.String())
}

func TestNewCommandSessionRepairCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{RepairResult: search.VectorRepairResult{
		SessionsScanned: 2,
		MessagesScanned: 3,
		RowsScanned:     4,
		MissingRows:     5,
		StaleRows:       6,
		UnchangedRows:   7,
		RebuiltRows:     8,
		DeletedSources:  9,
		Batches:         10,
	}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "repair", "--full", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.RepairOptions.SessionID)
	require.True(t, stub.RepairOptions.Full)
	require.Equal(t, "sessions_scanned=2 messages_scanned=3 rows_scanned=4 missing_rows=5 stale_rows=6 unchanged_rows=7 rebuilt_rows=8 deleted_sources=9 batches=10\n", output.String())
}

func TestNewCommandSessionStatusCallsRPC(t *testing.T) {
	setSessionTestProfile(t)
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	created := time.Date(2024, 5, 1, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 5, 2, 9, 0, 0, 0, time.UTC)
	stub := &agentstub.AgentServiceStub{StatusResult: rpcclient.ContextStatus{
		SessionID:        "project-a",
		Offset:           12,
		Size:             20,
		Length:           128000,
		Used:             64000,
		Remaining:        64000,
		UsedPct:          0.5,
		RemainingPct:     0.5,
		CreatedAt:        created,
		UpdatedAt:        updated,
		CompactionStatus: "succeeded",
	}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "status", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "id=project-a created_at=2024-05-01T08:00:00Z updated_at=2024-05-02T09:00:00Z compaction_status=succeeded offset=12 size=20 length=128000 used=64000 remaining=64000 pct_used=0.5000 pct_remaining=0.5000\n", output.String())
}

func TestGetSessionListLabel(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		title string
		want  string
	}{
		{
			name:  "title and id",
			id:    " default ",
			title: " Daily Planning ",
			want:  "Daily Planning (default)",
		},
		{
			name: "id only",
			id:   " project-a ",
			want: "project-a",
		},
		{
			name:  "title only",
			title: " Daily Planning ",
			want:  "Daily Planning",
		},
		{
			name: "empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, getSessionListLabel(tt.id, tt.title))
		})
	}
}

func TestFormatSessionTime(t *testing.T) {
	require.Empty(t, formatSessionTime(time.Time{}))
	require.Equal(
		t,
		"2024-05-01T07:00:00Z",
		formatSessionTime(time.Date(2024, 5, 1, 8, 0, 0, 0, time.FixedZone("test", 3600))),
	)
}

func setSessionTestProfile(t *testing.T) {
	t.Helper()
	t.Setenv("HAND_RPC_ADDRESS", "")
	t.Setenv("HAND_RPC_PORT", "")

	original := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(original)
	})
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: t.TempDir()}))
}

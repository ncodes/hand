package session

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/storage"
)

func TestNewCommandSessionNewCallsRPC(t *testing.T) {
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
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &agentstub.AgentServiceStub{Sessions: []storage.Session{{ID: "default"}, {ID: "project-a"}}}
	newClient = func(context.Context, *config.Config) (rpcclient.SessionClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "list"})

	require.NoError(t, err)
	require.Equal(t, "default\nproject-a\n", output.String())
}

func TestNewCommandSessionCurrentCallsRPC(t *testing.T) {
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

func TestNewCommandSessionStatusCallsRPC(t *testing.T) {
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
	stub := &agentstub.AgentServiceStub{StatusResult: rpcclient.SessionContextStatus{
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

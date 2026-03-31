package session

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	sessionstore "github.com/wandxy/hand/internal/storage/session"
)

type runnerStub struct {
	usedSessionID string
}

func (s *runnerStub) CreateSession(context.Context, string) (sessionstore.Session, error) {
	return sessionstore.Session{ID: "project-a"}, nil
}

func (s *runnerStub) ListSessions(context.Context) ([]sessionstore.Session, error) {
	return []sessionstore.Session{{ID: "default"}, {ID: "project-a"}}, nil
}

func (s *runnerStub) UseSession(_ context.Context, id string) error {
	s.usedSessionID = id
	return nil
}

func (s *runnerStub) CurrentSession(context.Context) (string, error) {
	return sessionstore.DefaultSessionID, nil
}

func (s *runnerStub) Close() error {
	return nil
}

func TestNewCommandSessionNewCallsRPC(t *testing.T) {
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	var output bytes.Buffer
	sessionOutput = &output

	stub := &runnerStub{}
	newClient = func(context.Context, *config.Config) (runner, error) {
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

	stub := &runnerStub{}
	newClient = func(context.Context, *config.Config) (runner, error) {
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

	stub := &runnerStub{}
	newClient = func(context.Context, *config.Config) (runner, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "current"})

	require.NoError(t, err)
	require.Equal(t, sessionstore.DefaultSessionID+"\n", output.String())
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

	stub := &runnerStub{}
	newClient = func(context.Context, *config.Config) (runner, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"session", "use", "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", stub.usedSessionID)
	require.Equal(t, "project-a\n", output.String())
}

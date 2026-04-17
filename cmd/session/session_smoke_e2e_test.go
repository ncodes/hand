package session

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/e2e"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func Test_E2E_SessionCommand_CreateSessionViaRPCSmoke(t *testing.T) {
	originalNewClient := newClient
	originalOutput := sessionOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		sessionOutput = originalOutput
	})

	h, err := e2e.NewRPCHarness(context.Background(), e2e.HarnessOptions{
		Spec:        e2eTestHarnessSpec(t),
		Config:      e2eTestHarnessConfig(),
		ModelClient: e2e.NewTextClient("ok"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	newClient = func(ctx context.Context, _ *config.Config) (rpcclient.SessionClient, error) {
		return h.Client(ctx)
	}

	var output bytes.Buffer
	sessionOutput = &output

	err = NewCommand().Run(context.Background(), []string{"session", "new", "ses_123456789012345678901"})
	require.NoError(t, err)
	require.Equal(t, "ses_123456789012345678901\n", output.String())
}

func e2eTestHarnessSpec(t *testing.T) e2e.HarnessSpec {
	t.Helper()
	home := filepath.Join(t.TempDir(), "hand-home")
	dataDir := filepath.Join(home, "data")
	return e2e.HarnessSpec{
		PrimaryEntrypoint:   e2e.EntrypointDirectAgent,
		SecondaryEntrypoint: e2e.EntrypointCommandRPC,
		Config:              e2e.ConfigInput{AllowInMemory: true},
		Isolation: e2e.Isolation{
			WorkspaceDir: filepath.Join(home, "workspace"),
			DataDir:      dataDir,
			StoragePath:  filepath.Join(dataDir, "state.db"),
			TraceDir:     filepath.Join(home, "traces"),
		},
	}
}

func e2eTestHarnessConfig() *config.Config {
	stream := false
	return &config.Config{
		Name:                     "Test Hand",
		Model:                    "test-model",
		Stream:                   &stream,
		StorageBackend:           "memory",
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
	}
}

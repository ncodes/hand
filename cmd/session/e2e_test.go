package session

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

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
	return e2e.DefaultSpec(filepath.Join(t.TempDir(), "hand-home"))
}

func e2eTestHarnessConfig() *config.Config {
	return e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "memory"})
}

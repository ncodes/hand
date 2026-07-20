package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserNetworkGuardScript_DisablesWebSocketsAndBackgroundWorkers(t *testing.T) {
	require.Contains(t, browserNetworkGuardScript, `"WebSocket"`)
	require.Contains(t, browserNetworkGuardScript, `"Worker"`)
	require.Contains(t, browserNetworkGuardScript, `"SharedWorker"`)
	require.Contains(t, browserNetworkGuardScript, `serviceWorker`)
}

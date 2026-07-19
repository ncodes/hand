package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
	"github.com/wandxy/morph/internal/config"
)

func TestChromiumBackend_RejectsInvalidLaunchConfiguration(t *testing.T) {
	backend := ChromiumBackend{}
	_, err := backend.Start(context.Background(), LaunchOptions{})
	require.EqualError(t, err, "browser startup timeout must be greater than zero")
	_, err = backend.Start(context.Background(), LaunchOptions{Timeout: time.Second, Mode: "bad"})
	require.EqualError(t, err, "browser profile mode is invalid")
	_, err = backend.Start(context.Background(), LaunchOptions{
		Timeout: time.Second, Mode: config.BrowserProfileManagedEphemeral,
	})
	require.EqualError(t, err, "browser executable is required")
	_, err = backend.Start(context.Background(), LaunchOptions{
		Timeout: time.Second, Mode: config.BrowserProfileRemoteCDP,
	})
	require.EqualError(t, err, "browser CDP endpoint is required")
	_, err = backend.Start(context.Background(), LaunchOptions{
		Timeout: time.Second, ProxyURL: "http://127.0.0.1:1234",
	})
	require.EqualError(t, err, "browser proxy credentials are required")
	_, err = backend.Start(context.Background(), LaunchOptions{
		Timeout: time.Second, ProxyUser: "morph", ProxySecret: "secret",
	})
	require.EqualError(t, err, "browser proxy URL is required for proxy credentials")
}

func TestChromiumBackend_RedactsRemoteRelaySecretFromConnectionError(t *testing.T) {
	_, err := (ChromiumBackend{}).Start(context.Background(), LaunchOptions{
		Mode:        config.BrowserProfileRemoteCDP,
		CDPEndpoint: "ws://127.0.0.1:1/devtools/browser/missing?_morph_browser_token=secret",
		Timeout:     time.Second,
	})
	require.EqualError(t, err, "browser CDP connection failed")
	require.NotContains(t, err.Error(), "secret")
}

func TestChromiumBackend_UsesAuthenticatedProxyAndCannotBypassStrictPolicy(t *testing.T) {
	executable, err := discoverChromiumExecutable("")
	if err != nil {
		t.Skip("Chromium is not installed")
	}
	var permissiveRequests atomic.Int64
	var originReceivedAuthorization atomic.Bool
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		permissiveRequests.Add(1)
		if request.URL.Path == "/auth" {
			if request.Header.Get("Authorization") != "" {
				originReceivedAuthorization.Store(true)
			}
			writer.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(writer, "<html><body>fixture</body></html>")
	}))
	defer fixture.Close()

	permissive, err := startEgressProxy(NetworkPolicy{Strict: false})
	require.NoError(t, err)
	session := startChromiumSession(t, executable, permissive)
	chromium := session.(*chromiumSession)
	require.NoError(t, chromedp.Run(chromium.ctx, chromedp.Navigate(fixture.URL)))
	require.Positive(t, permissiveRequests.Load())
	_ = chromedp.Run(chromium.ctx, chromedp.Navigate(fixture.URL+"/auth"))
	require.False(t, originReceivedAuthorization.Load())
	require.NoError(t, session.Close(context.Background()))
	require.NoError(t, permissive.Close(context.Background()))

	var strictRequests atomic.Int64
	strictFixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		strictRequests.Add(1)
		_, _ = io.WriteString(writer, "<html><body>blocked</body></html>")
	}))
	defer strictFixture.Close()
	strict, err := startEgressProxy(NetworkPolicy{Strict: true})
	require.NoError(t, err)
	session = startChromiumSession(t, executable, strict)
	chromium = session.(*chromiumSession)
	_ = chromedp.Run(chromium.ctx, chromedp.Navigate(strictFixture.URL))
	require.Zero(t, strictRequests.Load())
	require.NoError(t, session.Close(context.Background()))
	require.NoError(t, strict.Close(context.Background()))
}

func startChromiumSession(t *testing.T, executable string, proxy *egressProxy) BackendSession {
	t.Helper()
	username, password := proxy.authorization.credentials()
	session, err := (ChromiumBackend{}).Start(context.Background(), LaunchOptions{
		Executable:  executable,
		Mode:        config.BrowserProfileManagedEphemeral,
		DataDir:     t.TempDir(),
		ProxyURL:    proxy.chromiumURL(),
		ProxyUser:   username,
		ProxySecret: password,
		Timeout:     15 * time.Second,
	})
	require.NoError(t, err)

	return session
}

func TestChromiumBackend_StartsAvailableChromium(t *testing.T) {
	executable, err := discoverChromiumExecutable("")
	if err != nil {
		t.Skip("Chromium is not installed")
	}
	backend := ChromiumBackend{}
	session, err := backend.Start(context.Background(), LaunchOptions{
		Executable: executable,
		Mode:       config.BrowserProfileManagedEphemeral,
		DataDir:    t.TempDir(),
		Timeout:    15 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, session.Health(context.Background()))
	require.NoError(t, session.Close(context.Background()))
	require.NoError(t, session.Close(context.Background()))
}

package browser

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoteCDPRelay_PinsValidatedAddressAndRewritesDiscoveryURL(t *testing.T) {
	var endpoint string
	observed := make(chan *http.Request, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- request.Clone(context.Background())
		if request.URL.Path == "/json/version" {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"Browser":              "fixture",
				"webSocketDebuggerUrl": endpoint + "/devtools/browser/fixture",
			})
			return
		}
		_, _ = io.WriteString(writer, request.URL.Path)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	port, err := strconv.ParseUint(parsed.Port(), 10, 16)
	require.NoError(t, err)
	resolverCalls := 0
	policy := NetworkPolicy{
		Strict: false,
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			resolverCalls++
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
	}
	endpoint = "ws://browser.invalid:" + strconv.FormatUint(port, 10)
	relay, err := startRemoteCDPRelay(context.Background(), "http"+endpoint[2:], policy)
	require.NoError(t, err)

	response, err := http.Get(getRelayRequestURL(t, relay.URL(), "/json/version"))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	var discovery map[string]string
	require.NoError(t, json.NewDecoder(response.Body).Decode(&discovery))
	rewritten, err := url.Parse(discovery["webSocketDebuggerUrl"])
	require.NoError(t, err)
	relayURL, err := url.Parse(relay.URL())
	require.NoError(t, err)
	require.Equal(t, "ws", rewritten.Scheme)
	require.Equal(t, relayURL.Host, rewritten.Host)
	require.NotEmpty(t, relayURL.Query().Get(localRelayTokenParameter))
	require.Equal(
		t, relayURL.Query().Get(localRelayTokenParameter), rewritten.Query().Get(localRelayTokenParameter),
	)
	require.Equal(t, "/devtools/browser/fixture", rewritten.Path)
	require.NotContains(t, discovery["webSocketDebuggerUrl"], "browser.invalid")
	forwarded := <-observed
	require.Empty(t, forwarded.URL.Query().Get(localRelayTokenParameter))
	require.Empty(t, forwarded.Header.Get("Authorization"))
	require.GreaterOrEqual(t, resolverCalls, 2)
	relay.setPolicy(NetworkPolicy{Strict: true})
	response, err = http.Get(getRelayRequestURL(t, relay.URL(), "/json/version"))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	unauthorized := *relayURL
	unauthorized.RawQuery = ""
	response, err = http.Get(getRelayRequestURL(t, unauthorized.String(), "/json/version"))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, relay.Close(context.Background()))
	require.NoError(t, relay.Close(context.Background()))
}

func TestRemoteCDPRelay_RewritesTargetArraysAndDropsFrontendURL(t *testing.T) {
	var endpoint string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode([]map[string]string{{
			"id":                   "target",
			"devtoolsFrontendUrl":  "https://remote.invalid/devtools/inspector.html",
			"webSocketDebuggerUrl": endpoint + "/devtools/page/target",
		}})
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	endpoint = "ws://" + parsed.Host

	relay, err := startRemoteCDPRelay(context.Background(), upstream.URL, NetworkPolicy{Strict: false})
	require.NoError(t, err)
	response, err := http.Get(getRelayRequestURL(t, relay.URL(), "/json/list"))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	var targets []map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&targets))
	require.Len(t, targets, 1)
	require.NotContains(t, targets[0], "devtoolsFrontendUrl")
	rewritten, err := url.Parse(targets[0]["webSocketDebuggerUrl"].(string))
	require.NoError(t, err)
	require.Equal(t, mustParseURL(t, relay.URL()).Host, rewritten.Host)
	require.Equal(
		t,
		mustParseURL(t, relay.URL()).Query().Get(localRelayTokenParameter),
		rewritten.Query().Get(localRelayTokenParameter),
	)
	require.NoError(t, relay.Close(context.Background()))
}

func TestRemoteCDPRelay_RejectsDiscoveryOriginChange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"webSocketDebuggerUrl": "ws://different.invalid:9222/devtools/browser/fixture",
		})
	}))
	defer upstream.Close()
	relay, err := startRemoteCDPRelay(context.Background(), upstream.URL, NetworkPolicy{Strict: false})
	require.NoError(t, err)

	response, err := http.Get(getRelayRequestURL(t, relay.URL(), "/json/version"))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, relay.Close(context.Background()))
}

func TestRemoteCDPRelay_RejectsRedirectBeforeClientCanFollowIt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", "http://127.0.0.1/private")
		writer.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	relay, err := startRemoteCDPRelay(context.Background(), upstream.URL, NetworkPolicy{Strict: false})
	require.NoError(t, err)

	response, err := http.Get(getRelayRequestURL(t, relay.URL(), "/json/version"))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, relay.Close(context.Background()))
}

func TestRemoteCDPRelay_RejectsInvalidUnsafeAndMalformedDiscoveryEndpoints(t *testing.T) {
	_, err := startRemoteCDPRelay(context.Background(), "not-an-endpoint", NetworkPolicy{Strict: false})
	require.EqualError(t, err, "browser CDP endpoint is invalid")
	_, err = startRemoteCDPRelay(
		context.Background(), "http://127.0.0.1:9222", NetworkPolicy{Strict: true},
	)
	require.EqualError(t, err, "browser target resolves to a blocked address")

	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `not-json`},
		{name: "unsupported scalar", body: `"value"`},
		{name: "unsupported array item", body: `["value"]`},
		{name: "invalid websocket", body: `{"webSocketDebuggerUrl":"not-a-websocket"}`},
		{name: "wrong scheme", body: `{"webSocketDebuggerUrl":"wss://127.0.0.1:1/devtools/browser/id"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, test.body)
			}))
			defer upstream.Close()
			relay, relayErr := startRemoteCDPRelay(
				context.Background(), upstream.URL, NetworkPolicy{Strict: false},
			)
			require.NoError(t, relayErr)
			response, requestErr := http.Get(getRelayRequestURL(t, relay.URL(), "/json/version"))
			require.NoError(t, requestErr)
			require.Equal(t, http.StatusBadGateway, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.NoError(t, relay.Close(context.Background()))
		})
	}
}

func TestRemoteCDPRelay_PreservesDirectWebSocketPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, request.URL.Path)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	relay, err := startRemoteCDPRelay(
		context.Background(), "ws://"+parsed.Host+"/devtools/browser/direct", NetworkPolicy{Strict: false},
	)
	require.NoError(t, err)
	require.Equal(t, "/devtools/browser/direct", mustParseURL(t, relay.URL()).Path)
	require.Equal(t, "ws", mustParseURL(t, relay.URL()).Scheme)
	require.NoError(t, relay.Close(context.Background()))
	require.Empty(t, (*remoteCDPRelay)(nil).URL())
	require.NoError(t, (*remoteCDPRelay)(nil).Close(context.Background()))
}

func TestDialResolvedIPs_ReturnsJoinedFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	_, err = dialResolvedIPs(context.Background(), []net.IP{net.ParseIP("127.0.0.1")}, uint16(port))
	require.Error(t, err)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

func getRelayRequestURL(t *testing.T, raw, path string) string {
	t.Helper()
	parsed := mustParseURL(t, raw)
	parsed.Path = path
	return parsed.String()
}

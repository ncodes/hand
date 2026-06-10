package slack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	pkgslack "github.com/wandxy/hand/pkg/gateway/slack"
)

func TestStartSocketWithClient_DispatchesSocketEvents(t *testing.T) {
	origNewSlackAPI := newSlackAPI
	t.Cleanup(func() { newSlackAPI = origNewSlackAPI })
	api := &fakeSlackAPI{}
	newSlackAPI = func(config.GatewaySlackConfig) API { return api }
	service := newSlackServiceStub()
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	cfg.AllowedUsers = []string{"U1"}
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeSocketClient{
		run: func(ctx context.Context, handler func(context.Context, pkgslack.SocketEnvelope) error) error {
			err := handler(ctx, slackSocketEnvelope(t))
			cancel()
			return err
		},
	}

	err := StartSocketWithClient(ctx, cfg, service, client)

	require.NoError(t, err)
	require.Equal(t, 1, service.callCount())
	require.Len(t, api.allCalls(), 3)
}

func TestStartSocket_WaitsWhenDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- StartSocket(ctx, config.GatewayConfig{}, newSlackServiceStub())
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("disabled socket did not stop after context cancellation")
	}
}

func TestStartSocketWithClient_WaitsWhenDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- StartSocketWithClient(ctx, config.GatewayConfig{}, newSlackServiceStub(), &fakeSocketClient{})
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("disabled socket did not stop after context cancellation")
	}
}

func TestNewSocketClientSetsDefaults(t *testing.T) {
	client := newSocketClient(" xapp-token ")

	require.Equal(t, "xapp-token", client.appToken)
	require.Equal(t, defaultSlackAPIBase, client.baseURL)
	require.NotNil(t, client.http)
	require.NotNil(t, client.dial)
	_, err := client.dial("://bad")
	require.Error(t, err)
}

func TestStartSocketWithClient_RequiresDependencies(t *testing.T) {
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket

	require.EqualError(t, StartSocketWithClient(context.Background(), cfg, nil, &fakeSocketClient{}),
		"slack gateway service is required")
	require.EqualError(t, StartSocketWithClient(context.Background(), cfg, newSlackServiceStub(), nil),
		"slack socket client is required")
}

func TestStartSocketWithClient_StopsWhenReconnectSleepIsCanceled(t *testing.T) {
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeSocketClient{
		run: func(ctx context.Context, handler func(context.Context, pkgslack.SocketEnvelope) error) error {
			cancel()
			return errSlackTest
		},
	}

	err := StartSocketWithClient(ctx, cfg, newSlackServiceStub(), client)

	require.NoError(t, err)
}

func TestStartSocketWithClient_ReconnectsAfterClientError(t *testing.T) {
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	client := &fakeSocketClient{
		run: func(ctx context.Context, handler func(context.Context, pkgslack.SocketEnvelope) error) error {
			runs++
			if runs == 1 {
				return errSlackTest
			}
			cancel()
			return nil
		},
	}

	err := StartSocketWithClient(ctx, cfg, newSlackServiceStub(), client)

	require.NoError(t, err)
	require.Equal(t, 2, runs)
}

func TestStartSocketWithClient_TreatsNormalizeErrorAsReconnectable(t *testing.T) {
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &fakeSocketClient{
		run: func(ctx context.Context, handler func(context.Context, pkgslack.SocketEnvelope) error) error {
			err := handler(ctx, pkgslack.SocketEnvelope{Payload: []byte(`not json`)})
			cancel()
			return err
		},
	}

	err := StartSocketWithClient(ctx, cfg, newSlackServiceStub(), client)

	require.NoError(t, err)
}

func TestStartSocketWithClient_SleepStopsWhenContextCancelsAfterRunError(t *testing.T) {
	cfg := slackGatewayConfig()
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeSocketClient{
		run: func(ctx context.Context, handler func(context.Context, pkgslack.SocketEnvelope) error) error {
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
			return errSlackTest
		},
	}

	err := StartSocketWithClient(ctx, cfg, newSlackServiceStub(), client)

	require.NoError(t, err)
}

func TestSocketClient_RunReturnsInvalidEnvelopeAndHandlerErrors(t *testing.T) {
	tests := []struct {
		name    string
		message []byte
		handler func(context.Context, pkgslack.SocketEnvelope) error
	}{
		{name: "invalid json", message: []byte(`not json`)},
		{name: "handler error", message: slackSocketEnvelopeBytes(t), handler: func(context.Context, pkgslack.SocketEnvelope) error {
			return errSlackTest
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := socketClientWithConn(t, newFakeSocketConn(tt.message))

			err := client.Run(context.Background(), tt.handler)

			require.Error(t, err)
		})
	}
}

func TestSocketClient_RunReturnsReadError(t *testing.T) {
	conn := newFakeSocketConn(nil)
	conn.readErr = errSlackTest
	client := socketClientWithConn(t, conn)

	err := client.Run(context.Background(), nil)

	require.ErrorIs(t, err, errSlackTest)
}

func TestSocketClient_RunSkipsEmptyReads(t *testing.T) {
	conn := newFakeSocketConn(slackSocketEnvelopeBytes(t))
	conn.emptyReads = 1
	client := socketClientWithConn(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Run(ctx, func(ctx context.Context, envelope pkgslack.SocketEnvelope) error {
		require.Equal(t, "env1", envelope.EnvelopeID)
		cancel()
		return nil
	})

	require.NoError(t, err)
}

func TestSocketClient_RunSkipsWhitespaceFrames(t *testing.T) {
	conn := newFakeSocketConnFrames([]byte(" \n\t"), slackSocketEnvelopeBytes(t))
	client := socketClientWithConn(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Run(ctx, func(ctx context.Context, envelope pkgslack.SocketEnvelope) error {
		require.Equal(t, "env1", envelope.EnvelopeID)
		cancel()
		return nil
	})

	require.NoError(t, err)
}

func TestSocketClient_RunReturnsDialAndOpenErrors(t *testing.T) {
	t.Run("open error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
		}))
		defer server.Close()
		client := &socketClient{appToken: "xapp-token", http: server.Client(), baseURL: server.URL}

		err := client.Run(context.Background(), nil)

		require.EqualError(t, err, "invalid_auth")
	})

	t.Run("dial error", func(t *testing.T) {
		client := socketClientWithConn(t, nil)
		client.dial = func(string) (socketConn, error) {
			return nil, errSlackTest
		}

		err := client.Run(context.Background(), nil)

		require.ErrorIs(t, err, errSlackTest)
	})
}

func TestSocketClient_RunAcksBeforeHandlingEnvelope(t *testing.T) {
	conn := newFakeSocketConn(slackSocketEnvelopeBytes(t))
	client := socketClientWithConn(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Run(ctx, func(ctx context.Context, envelope pkgslack.SocketEnvelope) error {
		require.JSONEq(t, `{"envelope_id":"env1"}`, conn.writeString())
		cancel()
		return nil
	})

	require.NoError(t, err)
}

func TestSocketClient_RunReturnsAckWriteError(t *testing.T) {
	conn := newFakeSocketConn(slackSocketEnvelopeBytes(t))
	conn.writeErr = errSlackTest
	client := socketClientWithConn(t, conn)

	err := client.Run(context.Background(), nil)

	require.ErrorIs(t, err, errSlackTest)
}

func socketClientWithConn(t *testing.T, conn socketConn) *socketClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/apps.connections.open", r.URL.Path)
		require.Equal(t, "Bearer xapp-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"ok":true,"url":"wss://socket.test"}`))
	}))
	t.Cleanup(server.Close)

	return &socketClient{
		appToken: "xapp-token",
		http:     server.Client(),
		baseURL:  server.URL,
		dial: func(url string) (socketConn, error) {
			require.Equal(t, "wss://socket.test", url)
			return conn, nil
		},
	}
}

func TestSocketClient_OpenConnectionErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "http error", status: http.StatusBadGateway, body: `{"ok":true}`, want: "slack socket open failed"},
		{name: "invalid json", status: http.StatusOK, body: `not json`, want: "invalid character 'o' in literal null (expecting 'u')"},
		{name: "ok false", status: http.StatusOK, body: `{"ok":false,"error":"invalid_auth"}`, want: "invalid_auth"},
		{name: "ok false without message", status: http.StatusOK, body: `{"ok":false}`, want: "slack socket open returned ok=false"},
		{name: "missing url", status: http.StatusOK, body: `{"ok":true}`, want: "slack socket URL is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := &socketClient{appToken: "xapp-token", http: server.Client(), baseURL: server.URL}

			_, err := client.openConnection(context.Background())

			require.EqualError(t, err, tt.want)
		})
	}
}

func TestSocketClient_OpenConnectionReturnsRequestAndTransportErrors(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		client := &socketClient{baseURL: "http://[::1", http: http.DefaultClient}

		_, err := client.openConnection(context.Background())

		require.Error(t, err)
	})

	t.Run("transport", func(t *testing.T) {
		client := &socketClient{
			baseURL: "https://slack.test",
			http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errSlackTest
			})},
		}

		_, err := client.openConnection(context.Background())

		require.ErrorIs(t, err, errSlackTest)
	})
}

func TestSocketClient_OpenConnectionDefaultsHTTPClientAndBaseURL(t *testing.T) {
	origTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = origTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, defaultSlackAPIBase+"/apps.connections.open", r.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"url":"wss://socket.test"}`)),
			Header:     make(http.Header),
		}, nil
	})
	client := &socketClient{appToken: "xapp-token"}

	url, err := client.openConnection(context.Background())

	require.NoError(t, err)
	require.Equal(t, "wss://socket.test", url)
}

func TestSleepSocketReconnectReturnsFalseWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.False(t, sleepSocketReconnect(ctx, time.Hour))
}

type fakeSocketClient struct {
	run func(context.Context, func(context.Context, pkgslack.SocketEnvelope) error) error
}

func (c *fakeSocketClient) Run(
	ctx context.Context,
	handler func(context.Context, pkgslack.SocketEnvelope) error,
) error {
	if c.run == nil {
		<-ctx.Done()
		return nil
	}

	return c.run(ctx, handler)
}

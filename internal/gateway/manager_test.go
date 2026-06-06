package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
)

func TestManager_StartDisabledKeepsDisabledState(t *testing.T) {
	manager := NewManager(Options{})

	require.NoError(t, manager.Start(context.Background(), config.GatewayConfig{}, nil))

	status := manager.Status()
	require.Equal(t, StateDisabled, status.State)
}

func TestManager_WaitBeforeStartIsAlreadyDone(t *testing.T) {
	manager := NewManager(Options{})

	select {
	case err := <-manager.Wait():
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("gateway wait blocked before start")
	}
	require.Equal(t, StateStopped, manager.Status().State)
}

func TestManager_StopBeforeStartDoesNothing(t *testing.T) {
	manager := NewManager(Options{})

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
	require.Equal(t, StateStopped, manager.Status().State)
}

func TestManager_StartsAndStopsHTTP(t *testing.T) {
	manager := NewManager(Options{ShutdownTimeout: time.Second})
	cfg := testGatewayConfig()

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	require.Equal(t, StateRunning, manager.Status().State)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
	require.Equal(t, StateStopped, manager.Status().State)
}

func TestManager_StartIsIdempotentWhileRunning(t *testing.T) {
	listenCount := 0
	manager := NewManager(Options{
		Listen: func(network string, address string) (net.Listener, error) {
			listenCount++
			return net.Listen(network, address)
		},
	})
	cfg := testGatewayConfig()

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	require.Equal(t, 1, listenCount)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
}

func TestManager_ReturnsListenError(t *testing.T) {
	manager := NewManager(Options{
		Listen: func(string, string) (net.Listener, error) {
			return nil, errors.New("listen failed")
		},
	})
	cfg := testGatewayConfig()

	err := manager.Start(context.Background(), cfg, nil)

	require.EqualError(t, err, "listen failed")
	status := manager.Status()
	require.Equal(t, StateFailed, status.State)
	require.Equal(t, "listen failed", status.LastError)
}

func TestManager_ReportsHTTPServeError(t *testing.T) {
	serveErr := errors.New("serve failed")
	server := &fakeHTTPServer{serveErr: serveErr}
	manager := NewManager(Options{
		NewHTTPServer: func(config.GatewayConfig, Responder) HTTPServer {
			return server
		},
	})
	cfg := testGatewayConfig()

	require.NoError(t, manager.Start(context.Background(), cfg, nil))

	select {
	case err := <-manager.Wait():
		require.EqualError(t, err, "gateway http: serve failed")
	case <-time.After(time.Second):
		t.Fatal("gateway did not report serve error")
	}
	require.True(t, server.closed)
	require.Equal(t, StateFailed, manager.Status().State)
}

func TestManager_ReportsUnexpectedHTTPServeStop(t *testing.T) {
	manager := NewManager(Options{
		NewHTTPServer: func(config.GatewayConfig, Responder) HTTPServer {
			return &fakeHTTPServer{serveErr: http.ErrServerClosed}
		},
	})
	cfg := testGatewayConfig()

	require.NoError(t, manager.Start(context.Background(), cfg, nil))

	select {
	case err := <-manager.Wait():
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("gateway did not report stopped HTTP server")
	}
	require.Equal(t, StateStopped, manager.Status().State)
}

func TestManager_StartsSlackSocketAndTelegramPolling(t *testing.T) {
	slackStarted := make(chan struct{})
	telegramStarted := make(chan struct{})
	manager := NewManager(Options{
		StartSlackSocket: func(ctx context.Context, cfg config.GatewaySlackConfig) error {
			close(slackStarted)
			<-ctx.Done()
			return nil
		},
		StartTelegramPolling: func(ctx context.Context, cfg config.GatewayTelegramConfig) error {
			close(telegramStarted)
			<-ctx.Done()
			return nil
		},
	})
	cfg := testGatewayConfig()
	cfg.Slack.Enabled = true
	cfg.Slack.Mode = config.GatewaySlackModeSocket
	cfg.Telegram.Enabled = true
	cfg.Telegram.Mode = config.GatewayTelegramModePolling

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	require.Eventually(t, func() bool {
		select {
		case <-slackStarted:
		default:
			return false
		}
		select {
		case <-telegramStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
}

func TestManager_StopTreatsComponentContextCancellationAsCleanShutdown(t *testing.T) {
	manager := NewManager(Options{
		StartSlackSocket: func(ctx context.Context, cfg config.GatewaySlackConfig) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	cfg := testGatewayConfig()
	cfg.Slack.Enabled = true
	cfg.Slack.Mode = config.GatewaySlackModeSocket

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
	require.Equal(t, StateStopped, manager.Status().State)

	select {
	case err := <-manager.Wait():
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("gateway wait blocked after stop")
	}
}

func TestManager_RestartStopsExistingRuntimeBeforeStartingReplacement(t *testing.T) {
	stopCount := 0
	manager := NewManager(Options{
		NewHTTPServer: func(config.GatewayConfig, Responder) HTTPServer {
			return &fakeHTTPServer{onShutdown: func() {
				stopCount++
			}}
		},
	})
	cfg := testGatewayConfig()

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	replacement := cfg
	replacement.Port = 0
	require.NoError(t, manager.Restart(context.Background(), replacement, nil))

	require.Equal(t, 1, stopCount)
	require.Equal(t, StateRunning, manager.Status().State)
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.Stop(stopCtx))
}

func TestManager_RestartReturnsStopError(t *testing.T) {
	release := make(chan struct{})
	manager := NewManager(Options{
		StartSlackSocket: func(context.Context, config.GatewaySlackConfig) error {
			<-release
			return nil
		},
	})
	cfg := testGatewayConfig()
	cfg.Slack.Enabled = true
	cfg.Slack.Mode = config.GatewaySlackModeSocket

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	restartCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, manager.Restart(restartCtx, cfg, nil), context.DeadlineExceeded)

	close(release)
	select {
	case <-manager.Wait():
	case <-time.After(time.Second):
		t.Fatal("gateway did not stop after stuck component was released")
	}
}

func TestManager_StopReturnsContextErrorWhenComponentDoesNotStop(t *testing.T) {
	release := make(chan struct{})
	manager := NewManager(Options{
		StartSlackSocket: func(context.Context, config.GatewaySlackConfig) error {
			<-release
			return nil
		},
	})
	cfg := testGatewayConfig()
	cfg.Slack.Enabled = true
	cfg.Slack.Mode = config.GatewaySlackModeSocket

	require.NoError(t, manager.Start(context.Background(), cfg, nil))
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	require.ErrorIs(t, manager.Stop(stopCtx), context.DeadlineExceeded)
	close(release)
	select {
	case <-manager.Wait():
	case <-time.After(time.Second):
		t.Fatal("gateway did not stop after stuck component was released")
	}
}

func TestComponentErrorUnwrapsCause(t *testing.T) {
	cause := errors.New("cause")

	require.ErrorIs(t, componentError{name: "component", err: cause}, cause)
}

func testGatewayConfig() config.GatewayConfig {
	return config.GatewayConfig{
		Enabled:   true,
		Address:   "127.0.0.1",
		Port:      0,
		AuthToken: "token",
		Telegram: config.GatewayTelegramConfig{
			Mode:     config.GatewayTelegramModePolling,
			BotToken: "telegram-token",
		},
		Slack: config.GatewaySlackConfig{
			Mode:     config.GatewaySlackModeSocket,
			BotToken: "slack-token",
			AppToken: "app-token",
		},
	}
}

type fakeHTTPServer struct {
	serveErr   error
	closed     bool
	onShutdown func()
	done       chan struct{}
}

func (s *fakeHTTPServer) Serve(net.Listener) error {
	if s.serveErr != nil {
		return s.serveErr
	}
	<-s.getDone()
	return http.ErrServerClosed
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	if s.onShutdown != nil {
		s.onShutdown()
	}
	return s.Close()
}

func (s *fakeHTTPServer) Close() error {
	if !s.closed {
		s.closed = true
		close(s.getDone())
	}
	return nil
}

func (s *fakeHTTPServer) getDone() chan struct{} {
	if s.done == nil {
		s.done = make(chan struct{})
	}

	return s.done
}

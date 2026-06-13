package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/pkg/logutils"
)

func TestEnsureDaemonRunning_ReturnsConfigError(t *testing.T) {
	_, err := EnsureDaemonRunning(context.Background(), nil)

	require.EqualError(t, err, "config is required")
}

func TestCheckDaemonRPCImpl_CallsHealthService(t *testing.T) {
	rpcAddress, stop := startHealthRPCServer(t, healthpb.HealthCheckResponse_SERVING)
	defer stop()

	err := checkDaemonRPCImpl(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: rpcAddress.address, Port: rpcAddress.port}},
	)

	require.NoError(t, err)
}

func TestCheckDaemonRPCImpl_ReturnsConfigError(t *testing.T) {
	err := checkDaemonRPCImpl(context.Background(), nil)

	require.EqualError(t, err, "config is required")
}

func TestCheckDaemonRPCImpl_ReturnsMissingAddressError(t *testing.T) {
	err := checkDaemonRPCImpl(context.Background(), &config.Config{
		RPC: config.RPCConfig{Port: 50051},
	})

	require.EqualError(t, err, "rpc address is required")
}

func TestCheckDaemonRPCImpl_ReturnsMissingPortError(t *testing.T) {
	err := checkDaemonRPCImpl(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "127.0.0.1"},
	})

	require.EqualError(t, err, "rpc port must be greater than zero")
}

func TestCheckDaemonRPCImpl_ReturnsClientConstructionError(t *testing.T) {
	err := checkDaemonRPCImpl(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "%", Port: 50051},
	})

	require.ErrorContains(t, err, "invalid URL escape")
}

func TestCheckDaemonRPCImpl_ReturnsNonServingHealthError(t *testing.T) {
	rpcAddress, stop := startHealthRPCServer(t, healthpb.HealthCheckResponse_NOT_SERVING)
	defer stop()

	err := checkDaemonRPCImpl(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: rpcAddress.address, Port: rpcAddress.port}},
	)

	require.EqualError(t, err, "daemon health status is NOT_SERVING")
}

func TestCheckDaemonRPCImpl_ReturnsHealthCheckTransportError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, listener.Close())

	checkCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = checkDaemonRPCImpl(
		checkCtx,
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: tcpAddr.Port}},
	)

	require.Error(t, err)
}

func TestEnsureDaemonRunning_UsesExistingRPC(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	checks := 0
	checkDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		return nil
	}
	startDaemonRuntime = func(context.Context, *config.Config) (func() error, error) {
		t.Fatal("daemon should not start when RPC is already reachable")
		return nil, nil
	}

	cleanup, err := EnsureDaemonRunning(context.Background(), &config.Config{})

	require.NoError(t, err)
	require.Nil(t, cleanup)
	require.Equal(t, 1, checks)
}

func TestEnsureDaemonRunning_StartsRuntimeAndWaitsForRPC(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	started := false
	cleaned := false
	startDaemonRuntime = func(context.Context, *config.Config) (func() error, error) {
		started = true
		return func() error {
			cleaned = true
			return nil
		}, nil
	}
	checks := 0
	checkDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		if checks < 3 {
			return errors.New("connection refused")
		}

		return nil
	}

	cleanup, err := EnsureDaemonRunning(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
	)

	require.NoError(t, err)
	require.True(t, started)
	require.Equal(t, 3, checks)
	require.NoError(t, cleanup())
	require.True(t, cleaned)
}

func TestEnsureDaemonRunning_ReturnsRuntimeStartError(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	expectedErr := errors.New("runtime failed")
	checkDaemonRPC = func(context.Context, *config.Config) error {
		return errors.New("connection refused")
	}
	startDaemonRuntime = func(context.Context, *config.Config) (func() error, error) {
		return nil, expectedErr
	}

	_, err := EnsureDaemonRunning(context.Background(), &config.Config{})

	require.ErrorIs(t, err, expectedErr)
}

func TestEnsureDaemonRunning_ReturnsReadinessError(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	cleaned := false
	checkDaemonRPC = func(context.Context, *config.Config) error {
		return errors.New("connection refused")
	}
	startDaemonRuntime = func(context.Context, *config.Config) (func() error, error) {
		return func() error {
			cleaned = true
			return nil
		}, nil
	}

	_, err := EnsureDaemonRunning(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "RPC did not become ready at 127.0.0.1:50051")
	require.Contains(t, err.Error(), "connection refused")
	require.True(t, cleaned)
}

func TestEnsureDaemonRunning_ReturnsCleanupErrorAfterReadinessFailure(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	expectedErr := errors.New("cleanup failed")
	checkDaemonRPC = func(context.Context, *config.Config) error {
		return errors.New("connection refused")
	}
	startDaemonRuntime = func(context.Context, *config.Config) (func() error, error) {
		return func() error {
			return expectedErr
		}, nil
	}

	_, err := EnsureDaemonRunning(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
	)

	require.ErrorIs(t, err, expectedErr)
	require.ErrorContains(t, err, "cleanup after readiness failure")
}

func TestWaitForDaemonRPC_UsesSingleCheckWhenTimeoutIsNotPositive(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	expectedErr := errors.New("connection refused")
	checks := 0
	checkDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		return expectedErr
	}

	err := waitForDaemonRPC(context.Background(), &config.Config{}, 0)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, checks)
}

func TestWaitForDaemonRPC_ReturnsContextCancellation(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	checkDaemonRPC = func(context.Context, *config.Config) error {
		cancel()
		return errors.New("connection refused")
	}

	err := waitForDaemonRPC(ctx, &config.Config{}, time.Second)

	require.ErrorIs(t, err, context.Canceled)
}

func TestStartDaemonRuntimeImpl_CancelsRunAndRestoresOutput(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	initialOutput := &bytes.Buffer{}
	originalOutput := SetDaemonOutput(initialOutput)
	t.Cleanup(func() {
		SetDaemonOutput(originalOutput)
	})

	started := make(chan struct{})
	done := make(chan struct{})
	gotAddress := make(chan string, 1)
	runDaemonRuntimeOnce = func(ctx context.Context, cfg *config.Config) error {
		gotAddress <- cfg.RPC.Address
		close(started)
		<-ctx.Done()
		close(done)
		return nil
	}

	cleanup, err := startDaemonRuntimeImpl(
		context.Background(),
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
	)
	require.NoError(t, err)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not start")
	}
	require.Equal(t, "127.0.0.1", <-gotAddress)

	require.NoError(t, cleanup())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not stop")
	}
	require.NoError(t, cleanup())

	previousOutput := SetDaemonOutput(io.Discard)
	require.Same(t, initialOutput, previousOutput)
	SetDaemonOutput(previousOutput)
}

func TestStartDaemonRuntimeImpl_DisablesConsoleLoggingAndKeepsFileLogging(t *testing.T) {
	restore := replaceDaemonBootstrapHooks(t)
	defer restore()

	consoleOutput := &bytes.Buffer{}
	fileOutput := &bytes.Buffer{}
	previousOutput := SetDaemonOutput(io.Discard)
	t.Cleanup(func() {
		SetDaemonOutput(previousOutput)
		logutils.SetOutput(nil)
		logutils.SetFileOutput(nil)
		logutils.SetConsoleEnabled(true)
	})
	logutils.SetOutput(consoleOutput)
	logutils.SetFileOutput(fileOutput)
	logutils.SetConsoleEnabled(true)
	_ = logutils.ConfigureLogger("hand", true)

	started := make(chan struct{})
	done := make(chan struct{})
	daemonLog := logutils.Module("daemon")
	runDaemonRuntimeOnce = func(ctx context.Context, _ *config.Config) error {
		daemonLog.Info().Msg("temporary daemon started")
		close(started)
		<-ctx.Done()
		close(done)
		return nil
	}

	cleanup, err := startDaemonRuntimeImpl(context.Background(), &config.Config{})
	require.NoError(t, err)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not start")
	}
	require.Empty(t, consoleOutput.String())
	require.Contains(t, fileOutput.String(), `"message":"temporary daemon started"`)
	require.Contains(t, fileOutput.String(), `"module":"daemon"`)

	require.NoError(t, cleanup())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not stop")
	}

	logutils.Module("hand").Info().Msg("console restored")
	require.Contains(t, consoleOutput.String(), "console restored")
}

func replaceDaemonBootstrapHooks(t *testing.T) func() {
	t.Helper()

	originalCheckDaemonRPC := checkDaemonRPC
	originalStartDaemonRuntime := startDaemonRuntime
	originalRunDaemonRuntimeOnce := runDaemonRuntimeOnce
	originalInitialTimeout := daemonBootstrapInitialTimeout
	originalReadyTimeout := daemonBootstrapReadyTimeout
	originalPollInterval := daemonBootstrapPollInterval
	daemonBootstrapInitialTimeout = time.Millisecond
	daemonBootstrapReadyTimeout = 5 * time.Millisecond
	daemonBootstrapPollInterval = time.Millisecond

	return func() {
		checkDaemonRPC = originalCheckDaemonRPC
		startDaemonRuntime = originalStartDaemonRuntime
		runDaemonRuntimeOnce = originalRunDaemonRuntimeOnce
		daemonBootstrapInitialTimeout = originalInitialTimeout
		daemonBootstrapReadyTimeout = originalReadyTimeout
		daemonBootstrapPollInterval = originalPollInterval
	}
}

type healthRPCAddress struct {
	address string
	port    int
}

func startHealthRPCServer(
	t *testing.T,
	status healthpb.HealthCheckResponse_ServingStatus,
) (healthRPCAddress, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", status)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- grpcServer.Serve(listener)
	}()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	stop := func() {
		grpcServer.Stop()
		serveErr := <-serverErr
		require.True(t, serveErr == nil || errors.Is(serveErr, grpc.ErrServerStopped))
	}

	return healthRPCAddress{address: "127.0.0.1", port: tcpAddr.Port}, stop
}

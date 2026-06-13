package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/pkg/logutils"
)

const (
	defaultDaemonBootstrapInitialTimeout = 250 * time.Millisecond
	defaultDaemonBootstrapReadyTimeout   = 10 * time.Second
	defaultDaemonBootstrapPollInterval   = 100 * time.Millisecond
)

var (
	checkDaemonRPC                = checkDaemonRPCImpl
	startDaemonRuntime            = startDaemonRuntimeImpl
	runDaemonRuntimeOnce          = RunDaemonOnce
	daemonBootstrapInitialTimeout = defaultDaemonBootstrapInitialTimeout
	daemonBootstrapReadyTimeout   = defaultDaemonBootstrapReadyTimeout
	daemonBootstrapPollInterval   = defaultDaemonBootstrapPollInterval
)

func EnsureDaemonRunning(ctx context.Context, cfg *config.Config) (func() error, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	checkCtx, cancel := context.WithTimeout(ctx, daemonBootstrapInitialTimeout)
	err := checkDaemonRPC(checkCtx, cfg)
	cancel()
	if err == nil {
		return nil, nil
	}

	cleanup, err := startDaemonRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := waitForDaemonRPC(ctx, cfg, daemonBootstrapReadyTimeout); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return nil, fmt.Errorf("start Hand daemon: cleanup after readiness failure: %w", cleanupErr)
		}
		return nil, fmt.Errorf("start Hand daemon: RPC did not become ready at %s:%d: %w",
			strings.TrimSpace(cfg.RPC.Address), cfg.RPC.Port, err)
	}

	return cleanup, nil
}

func waitForDaemonRPC(ctx context.Context, cfg *config.Config, timeout time.Duration) error {
	if timeout <= 0 {
		return checkDaemonRPC(ctx, cfg)
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		checkCtx, cancel := context.WithTimeout(ctx, daemonBootstrapInitialTimeout)
		err := checkDaemonRPC(checkCtx, cfg)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().Add(daemonBootstrapPollInterval).After(deadline) {
			return lastErr
		}

		timer := time.NewTimer(daemonBootstrapPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func checkDaemonRPCImpl(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	address := strings.TrimSpace(cfg.RPC.Address)
	if address == "" {
		return fmt.Errorf("rpc address is required")
	}
	if cfg.RPC.Port <= 0 {
		return fmt.Errorf("rpc port must be greater than zero")
	}

	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", address, cfg.RPC.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("daemon health status is %s", resp.GetStatus())
	}

	return nil
}

func startDaemonRuntimeImpl(ctx context.Context, cfg *config.Config) (func() error, error) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	previousOutput := SetDaemonOutput(io.Discard)
	previousConsoleEnabled := logutils.SetConsoleEnabled(false)
	var once sync.Once
	var cleanupErr error

	go func() {
		done <- runDaemonRuntimeOnce(runCtx, cfg)
	}()

	cleanup := func() error {
		once.Do(func() {
			cancel()
			cleanupErr = <-done
			SetDaemonOutput(previousOutput)
			logutils.SetConsoleEnabled(previousConsoleEnabled)
		})
		return cleanupErr
	}

	return cleanup, nil
}

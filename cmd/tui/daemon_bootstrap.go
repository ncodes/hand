package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

const (
	defaultDaemonBootstrapInitialTimeout = 250 * time.Millisecond
	defaultDaemonBootstrapReadyTimeout   = 10 * time.Second
	defaultDaemonBootstrapPollInterval   = 100 * time.Millisecond
)

var (
	ensureTUIDaemonRunning        = ensureTUIDaemonRunningImpl
	checkTUIDaemonRPC             = checkTUIDaemonRPCImpl
	startTUIDaemonRuntime         = startTUIDaemonRuntimeImpl
	runTUIDaemonOnce              = handcli.RunDaemonOnce
	daemonBootstrapInitialTimeout = defaultDaemonBootstrapInitialTimeout
	daemonBootstrapReadyTimeout   = defaultDaemonBootstrapReadyTimeout
	daemonBootstrapPollInterval   = defaultDaemonBootstrapPollInterval
)

func ensureTUIDaemonRunningImpl(ctx context.Context, cfg *config.Config) (func() error, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	cleanup, err := startTUIDaemonRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := waitForTUIDaemonRPC(ctx, cfg, daemonBootstrapReadyTimeout); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return nil, fmt.Errorf("start Hand daemon: cleanup after readiness failure: %w", cleanupErr)
		}
		return nil, fmt.Errorf("start Hand daemon: RPC did not become ready at %s:%d: %w",
			strings.TrimSpace(cfg.RPC.Address), cfg.RPC.Port, err)
	}

	return cleanup, nil
}

func waitForTUIDaemonRPC(ctx context.Context, cfg *config.Config, timeout time.Duration) error {
	if timeout <= 0 {
		return checkTUIDaemonRPC(ctx, cfg)
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		checkCtx, cancel := context.WithTimeout(ctx, daemonBootstrapInitialTimeout)
		err := checkTUIDaemonRPC(checkCtx, cfg)
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

func checkTUIDaemonRPCImpl(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	client, err := rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = client.Gateway.GatewayStatus(ctx)
	return err
}

func startTUIDaemonRuntimeImpl(ctx context.Context, cfg *config.Config) (func() error, error) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	previousOutput := handcli.SetDaemonOutput(io.Discard)
	var once sync.Once
	var cleanupErr error

	go func() {
		done <- runTUIDaemonOnce(runCtx, cfg)
	}()

	cleanup := func() error {
		once.Do(func() {
			cancel()
			cleanupErr = <-done
			handcli.SetDaemonOutput(previousOutput)
		})
		return cleanupErr
	}

	return cleanup, nil
}

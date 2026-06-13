package daemon

import (
	"context"
	"errors"
	"fmt"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/gateway"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/internal/rpc/server"
	handruntime "github.com/wandxy/hand/internal/runtime"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// listenFunc is swapped in tests to simulate listen failures.
var listenFunc = net.Listen

// grpcServerServe is swapped in tests to exercise serveRPC select branches.
var grpcServerServe = func(srv *grpc.Server, lis net.Listener) error {
	return srv.Serve(lis)
}

// grpcGracefulStop and serveRPCShutdownTimeout are swapped in tests to hit forced shutdown paths.
var grpcGracefulStop = func(srv *grpc.Server) {
	srv.GracefulStop()
}

var serveRPCShutdownTimeout = 5 * time.Second

// postShutdownServeErrHook is swapped in tests to cover the final serverErr branch.
var postShutdownServeErrHook = func(err error) error { return err }

var writeRuntimeMetadata = handruntime.WriteActive

var openRPCListener = openRPCListenerImpl

type gatewayManager interface {
	Start(context.Context, config.GatewayConfig, gateway.AgentService) error
	Stop(context.Context) error
	Status() gateway.Status
}

var newGatewayManager = func() gatewayManager {
	return gateway.NewManager(gateway.Options{})
}

var stopGatewayTimeout = 5 * time.Second

func serveDaemonServices(ctx context.Context, cfg *config.Config, agent agentRunner, lis net.Listener) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	manager := newGatewayManager()
	if cfg == nil {
		return serveRPC(runCtx, cfg, agent, lis, manager)
	}
	if err := manager.Start(runCtx, cfg.Gateway, agent); err != nil {
		_ = lis.Close()
		return err
	}
	if !cfg.Gateway.Enabled {
		return serveRPC(runCtx, cfg, agent, lis, manager)
	}

	logGatewayStarted(cfg.Gateway)

	rpcDone := make(chan error, 1)
	go func() {
		rpcDone <- serveRPC(runCtx, cfg, agent, lis, manager)
	}()

	var err error
	select {
	case err = <-rpcDone:
		cancel()
		stopGatewayWithTimeout(manager)
		return err
	case <-ctx.Done():
		cancel()
		stopGatewayWithTimeout(manager)
		return <-rpcDone
	}
}

func logGatewayStarted(cfg config.GatewayConfig) {
	event := daemonLog.Info().Str("gatewayAddress", cfg.Address).Int("gatewayPort", cfg.Port)
	if cfg.Telegram.Enabled {
		event = event.Str("telegramMode", cfg.Telegram.Mode)
	}
	if cfg.Slack.Enabled {
		event = event.Str("slackMode", cfg.Slack.Mode)
	}

	event.Msg("Gateway started")
}

func stopGatewayWithTimeout(manager gatewayManager) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), stopGatewayTimeout)
	defer cancel()
	if err := manager.Stop(shutdownCtx); err != nil {
		daemonLog.Warn().Err(err).Msg("Gateway shutdown failed")
	}
}

func isMissingCredentialLockError(err error) bool {
	if err == nil || !os.IsNotExist(err) {
		return false
	}

	return strings.Contains(err.Error(), "auth.json.lock")
}

func openRPCListenerImpl(cfg *config.Config) (net.Listener, error) {
	lis, err := listenFunc("tcp", fmt.Sprintf("%s:%d", cfg.RPC.Address, cfg.RPC.Port))
	if err != nil {
		return nil, err
	}

	if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
		cfg.RPC.Port = tcpAddr.Port
	}
	if active := profile.Active(); strings.TrimSpace(active.HomeDir) != "" || strings.TrimSpace(active.RuntimePath) != "" {
		if _, err := writeRuntimeMetadata(cfg.RPC.Address, cfg.RPC.Port); err != nil {
			_ = lis.Close()
			return nil, err
		}
	}

	return lis, nil
}

var serveRPC = func(
	ctx context.Context,
	cfg *config.Config,
	agent agentRunner,
	lis net.Listener,
	manager gatewayManager,
) error {
	defer lis.Close()

	var gatewayCfg config.GatewayConfig
	var pairingSecret string
	if cfg != nil {
		gatewayCfg = cfg.Gateway
		pairingSecret = strings.TrimSpace(cfg.Gateway.PairingSecret)
	}

	grpcSrv := server.New(agent, server.Options{
		Health:               true,
		GatewayPairingSecret: pairingSecret,
		GatewayConfig:        gatewayCfg,
		GatewayRuntime:       manager,
	})

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- grpcServerServe(grpcSrv, lis)
	}()

	daemonLog.Info().
		Str("rpcAddress", cfg.RPC.Address).
		Int("rpcPort", cfg.RPC.Port).
		Msg("RPC server listening for daemon requests")

	select {
	case err := <-serverErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}

		return err
	case <-sigCtx.Done():
		daemonLog.Info().
			Msg("received shutdown signal")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), serveRPCShutdownTimeout)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcGracefulStop(grpcSrv)
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		daemonLog.Warn().
			Msg("RPC graceful shutdown timed out, forcing stop")
		grpcSrv.Stop()
		<-stopped
	}

	if err := postShutdownServeErrHook(<-serverErr); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}

	daemonLog.Info().
		Msg("RPC server stopped")
	return nil
}

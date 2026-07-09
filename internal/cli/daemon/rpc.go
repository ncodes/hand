package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wandxy/morph/internal/automation"
	"github.com/wandxy/morph/internal/config"
	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/gateway"
	"github.com/wandxy/morph/internal/profile"
	morphrpc "github.com/wandxy/morph/internal/rpc"
	"github.com/wandxy/morph/internal/rpc/server"
	morphruntime "github.com/wandxy/morph/internal/runtime"
	"github.com/wandxy/morph/pkg/str"
	"google.golang.org/grpc"
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

var writeRuntimeMetadata = morphruntime.WriteActive

var openRPCListener = openRPCListenerImpl

type gatewayManager interface {
	Start(context.Context, config.GatewayConfig, gateway.AgentService) error
	Stop(context.Context) error
	Status() gateway.Status
}

type automationServiceBinder interface {
	SetAutomationService(envtypes.AutomationService)
}

var newGatewayManager = func() gatewayManager {
	return gateway.NewManager(gateway.Options{})
}

var stopGatewayTimeout = 5 * time.Second

var newAutomationService = func(
	store automation.Store,
	runner automation.Runner,
	opts automation.ServiceOptions,
) (*automation.Service, error) {
	opts.Store = store
	opts.Runner = runner
	return automation.NewService(opts)
}

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

func buildAutomationService(ctx context.Context, agent agentRunner) (*automation.Service, error) {
	if agent == nil {
		return nil, nil
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, nil
	}

	store, ok, err := agent.AutomationStore(ctx)
	if err != nil || !ok {
		return nil, err
	}

	return newAutomationService(
		store,
		automation.NewAgentRunner(automation.AgentRunnerOptions{}),
		automation.ServiceOptions{})
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
	active := profile.Active()
	activeHomeDir := str.String(active.HomeDir)
	activeRuntimePath := str.String(active.RuntimePath)
	if activeHomeDir.Trim() != "" || activeRuntimePath.Trim() != "" {
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

	automationService, err := buildAutomationService(ctx, agent)
	if err != nil {
		return err
	}
	if automationService != nil {
		if err := automationService.Start(ctx); err != nil {
			return err
		}
		defer automationService.Stop()
		if binder, ok := agent.(automationServiceBinder); ok {
			binder.SetAutomationService(automationService)
		}
	}

	var gatewayCfg config.GatewayConfig
	var pairingSecret string
	if cfg != nil {
		gatewayCfg = cfg.Gateway
		stringValue3 := str.String(cfg.Gateway.PairingSecret)
		pairingSecret = stringValue3.Trim()
	}

	grpcSrv := server.New(agent, server.Options{
		RuntimeModel:         morphrpc.ModelRuntimeFromConfig(cfg),
		Health:               true,
		GatewayPairingSecret: pairingSecret,
		GatewayConfig:        gatewayCfg,
		GatewayRuntime:       manager,
		Automation:           automationService,
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

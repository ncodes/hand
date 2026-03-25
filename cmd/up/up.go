package up

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/wandxy/hand/internal/agent"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/diagnostics"
	"github.com/wandxy/hand/internal/models"
	rpc "github.com/wandxy/hand/internal/rpc"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"github.com/wandxy/hand/pkg/logutils"
)

type agentRunner interface {
	Run(context.Context) error
	Chat(context.Context, string) (string, error)
}

const (
	handBadge  = "██   ██  █████  ███    ██ ██████\n██   ██ ██   ██ ████   ██ ██   ██\n███████ ███████ ██ ██  ██ ██   ██\n██   ██ ██   ██ ██  ██ ██ ██   ██\n██   ██ ██   ██ ██   ████ ██████\n"
	colorGray  = "\x1b[90m"
	colorReset = "\x1b[0m"
)

var startupOutput io.Writer = os.Stdout

var newAgentRunner = func(ctx context.Context, cfg *config.Config, modelClient models.Client) agentRunner {
	return agent.NewAgent(ctx, cfg, modelClient)
}

func renderStartupPanel(cfg *config.Config) string {
	if cfg == nil {
		return handBadge
	}

	logStyle := "color"
	debugRequests := "disabled"
	if cfg.LogNoColor {
		logStyle = "plain"
	}
	if cfg.DebugRequests {
		debugRequests = "enabled"
	}
	traceStatus := "disabled"
	if cfg.DebugTraces {
		traceStatus = fmt.Sprintf("enabled (%s)", cfg.DebugTraceDir)
	}

	lines := []string{
		styleStartup(handBadge, cfg.LogNoColor),
		styleStartup("Hand daemon", cfg.LogNoColor),
		styleStartup(handcli.AppDescription, cfg.LogNoColor),
		"",
		fmt.Sprintf("%s %s", styleLabel("Instance", cfg.LogNoColor), cfg.Name),
		fmt.Sprintf("%s %s", styleLabel("Model", cfg.LogNoColor), cfg.Model),
		fmt.Sprintf("%s %s", styleLabel("Router", cfg.LogNoColor), cfg.ModelRouter),
		fmt.Sprintf("%s %s", styleLabel("RPC", cfg.LogNoColor), fmt.Sprintf("%s:%d", cfg.RPCAddress, cfg.RPCPort)),
		fmt.Sprintf("%s %s", styleLabel("Logs", cfg.LogNoColor), fmt.Sprintf("%s (%s)", cfg.LogLevel, logStyle)),
		fmt.Sprintf("%s %s", styleLabel("Debug requests", cfg.LogNoColor), debugRequests),
		fmt.Sprintf("%s %s", styleLabel("Traces", cfg.LogNoColor), traceStatus),
		"",
		styleStartup("Ready to accept RPC connections.", cfg.LogNoColor),
	}

	return strings.Join(lines, "\n") + "\n"
}

func styleStartup(value string, noColor bool) string {
	if noColor {
		return value
	}
	return colorGray + value + colorReset
}

func styleLabel(value string, noColor bool) string {
	if noColor {
		return value + ":"
	}
	return colorGray + value + ":" + colorReset
}

var serveRPC = func(ctx context.Context, cfg *config.Config, agent agentRunner) error {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.RPCAddress, cfg.RPCPort))
	if err != nil {
		return err
	}
	defer lis.Close()

	grpcSrv := grpc.NewServer()
	healthcheck := health.NewServer()
	handpb.RegisterHandServiceServer(grpcSrv, rpc.NewService(agent))
	healthpb.RegisterHealthServer(grpcSrv, healthcheck)
	healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- grpcSrv.Serve(lis)
	}()

	log.Info().
		Str("rpcAddress", cfg.RPCAddress).
		Int("rpcPort", cfg.RPCPort).
		Msg("Starting RPC server")

	select {
	case err := <-serverErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	case <-sigCtx.Done():
		log.Info().Msg("Received shutdown signal")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		log.Warn().Msg("RPC graceful shutdown timed out, forcing stop")
		grpcSrv.Stop()
		<-stopped
	}

	if err := <-serverErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}

	log.Info().Msg("RPC server stopped")
	return nil
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "up",
		Usage: "start the agent runtime",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("env-file"), cmd.String("config"))
			if err != nil {
				return err
			}
			handcli.ApplyConfigOverrides(cmd, cfg)
			report := diagnostics.Build(cmd.String("env-file"), cmd.String("config"), cfg, nil)
			if report.HasFailures() {
				return errors.New(report.FirstFailure())
			}
			auth, _ := cfg.ResolveModelAuth()

			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
			logutils.SetLogLevel(cfg.LogLevel)

			if _, err := fmt.Fprint(startupOutput, renderStartupPanel(cfg)); err != nil {
				return err
			}

			log.Info().
				Str("name", cfg.Name).
				Str("model", cfg.Model).
				Str("router", cfg.ModelRouter).
				Bool("debugTraces", cfg.DebugTraces).
				Str("debugTraceDir", cfg.DebugTraceDir).
				Msg("Configuration loaded")

			log.Info().
				Str("service", "hand").
				Str("rpcAddress", cfg.RPCAddress).
				Int("rpcPort", cfg.RPCPort).
				Str("rpcEndpoint", fmt.Sprintf("%s:%d", cfg.RPCAddress, cfg.RPCPort)).
				Bool("debugTraces", cfg.DebugTraces).
				Str("debugTraceDir", cfg.DebugTraceDir).
				Msg("Starting Hand services")

			clientOptions := make([]option.RequestOption, 0, 1)
			if cfg.ModelBaseURL != "" {
				clientOptions = append(clientOptions, option.WithBaseURL(cfg.ModelBaseURL))
			}

			modelClient, err := models.NewOpenAIClient(auth.APIKey, clientOptions...)
			if err != nil {
				return err
			}

			agent := newAgentRunner(ctx, cfg, modelClient)
			if err := agent.Run(ctx); err != nil {
				return err
			}

			return serveRPC(ctx, cfg, agent)
		},
	}
}

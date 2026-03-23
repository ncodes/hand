package up

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/signal"
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

type runner interface {
	Run(context.Context) error
}

var newAgentRunner = func(ctx context.Context, cfg *config.Config, modelClient models.Client) runner {
	return agent.NewAgent(ctx, cfg, modelClient)
}

var serveRPC = func(ctx context.Context, cfg *config.Config) error {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.RPCAddress, cfg.RPCPort))
	if err != nil {
		return err
	}
	defer lis.Close()

	grpcSrv := grpc.NewServer()
	healthcheck := health.NewServer()
	handpb.RegisterHandServiceServer(grpcSrv, rpc.NewService())
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

			log.Info().
				Str("name", cfg.Name).
				Str("model", cfg.Model).
				Str("modelRouter", cfg.ModelRouter).
				Str("modelBaseURL", cfg.ModelBaseURL).
				Str("logLevel", cfg.LogLevel).
				Bool("logNoColor", cfg.LogNoColor).
				Msg("configuration loaded")

			clientOptions := make([]option.RequestOption, 0, 1)
			if cfg.ModelBaseURL != "" {
				clientOptions = append(clientOptions, option.WithBaseURL(cfg.ModelBaseURL))
			}

			modelClient, err := models.NewOpenAIClient(auth.APIKey, clientOptions...)
			if err != nil {
				return err
			}

			app := newAgentRunner(ctx, cfg, modelClient)
			if err := app.Run(ctx); err != nil {
				return err
			}

			return serveRPC(ctx, cfg)
		},
	}
}

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

	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"

	handagent "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/brand"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/diagnostics"
	models "github.com/wandxy/hand/internal/model"
	modelclient "github.com/wandxy/hand/internal/model/client"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/internal/rpc/server"
	handruntime "github.com/wandxy/hand/internal/runtime"
	"github.com/wandxy/hand/pkg/logutils"
)

type agentRunner interface {
	Start(context.Context) error
	handagent.ServiceAPI
}

const (
	colorGray  = "\x1b[90m"
	colorReset = "\x1b[0m"
)

var handBadge = joinStartupBanner(brand.Mark, brand.Wordmark)

var startupOutput io.Writer = os.Stdout

func SetOutput(w io.Writer) io.Writer {
	previous := startupOutput
	if w == nil {
		startupOutput = io.Discard
		return previous
	}
	startupOutput = w
	return previous
}

func newAgentRunnerImpl(
	ctx context.Context,
	cfg *config.Config,
	modelClient,
	summaryClient models.Client,
) agentRunner {
	return handagent.NewAgent(ctx, cfg, modelClient, summaryClient)
}

// newAgentRunner is swapped in tests to stub the agent.
var newAgentRunner = newAgentRunnerImpl

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

type modelClientFactoryAPI interface {
	NewClient(modelclient.ClientRequest) (models.Client, error)
}

var modelClientFactory modelClientFactoryAPI = modelclient.NewDefaultClientFactory()

// resolveSummaryAuth resolves summary model credentials (hooked in tests).
var resolveSummaryAuth = func(cfg *config.Config) (config.ModelAuth, error) {
	return cfg.ResolveSummaryModelAuth()
}

func modelClientRequest(
	role modelclient.ModelRole,
	model string,
	auth config.ModelAuth,
	maxRetries int,
) modelclient.ClientRequest {
	return modelclient.ClientRequest{
		Role:       role,
		Model:      model,
		Provider:   auth.Provider,
		API:        auth.API,
		APIKey:     auth.APIKey,
		BaseURL:    auth.BaseURL,
		Headers:    auth.Headers,
		MaxRetries: maxRetries,
	}
}

func renderStartupPanel(cfg *config.Config) string {
	if cfg == nil {
		return handBadge
	}

	logStyle := "color"
	debugRequests := "disabled"
	if cfg.Log.NoColor {
		logStyle = "plain"
	}

	if cfg.Debug.Requests {
		debugRequests = "enabled"
	}
	traceStatus := "disabled"
	if cfg.Trace.Enabled {
		traceDir := strings.TrimSpace(cfg.Trace.Disk.Dir)
		traceStatus = fmt.Sprintf("enabled (%s)", traceDir)
	}

	lines := []string{
		"",
		styleStartup(handBadge, cfg.Log.NoColor),
		"",
		fmt.Sprintf("%s %s", styleLabel("Instance", cfg.Log.NoColor), cfg.Name),
		fmt.Sprintf("%s %s", styleLabel("Model", cfg.Log.NoColor), cfg.Models.Main.Name),
		fmt.Sprintf("%s %s", styleLabel("Provider", cfg.Log.NoColor), cfg.Models.Main.Provider),
		fmt.Sprintf("%s %s", styleLabel("Summary model", cfg.Log.NoColor), cfg.SummaryModelEffective()),
		fmt.Sprintf("%s %s", styleLabel("Summary provider", cfg.Log.NoColor), cfg.SummaryProviderEffective()),
		fmt.Sprintf("%s %s", styleLabel("Storage", cfg.Log.NoColor), getEffectiveStorageBackend(cfg)),
	}
	if cfg.SummaryModelAPIEffective() != cfg.Models.Main.API {
		lines = append(lines, fmt.Sprintf("%s %s", styleLabel("Summary API", cfg.Log.NoColor), cfg.SummaryModelAPIEffective()))
	}
	lines = append(lines,
		fmt.Sprintf("%s %t", styleLabel("Streaming", cfg.Log.NoColor), cfg.StreamEnabled()),
		fmt.Sprintf("%s %s", styleLabel("RPC", cfg.Log.NoColor), fmt.Sprintf("%s:%d", cfg.RPC.Address, cfg.RPC.Port)),
		fmt.Sprintf("%s %s", styleLabel("Logs", cfg.Log.NoColor), fmt.Sprintf("%s (%s)", cfg.Log.Level, logStyle)),
		fmt.Sprintf("%s %s", styleLabel("Debug requests", cfg.Log.NoColor), debugRequests),
		fmt.Sprintf("%s %s", styleLabel("Traces", cfg.Log.NoColor), traceStatus),
		fmt.Sprintf("%s %s", styleLabel("Safety", cfg.Log.NoColor), handcli.SafetySummary(cfg)),
	)
	if cfg.Search.Vector.Enabled {
		lines = append(lines,
			fmt.Sprintf("%s %s", styleLabel("Embedding model", cfg.Log.NoColor), cfg.Models.Embedding.Name),
			fmt.Sprintf("%s %s", styleLabel("Embedding provider", cfg.Log.NoColor), cfg.ModelEmbeddingProviderEffective()),
			fmt.Sprintf("%s %s", styleLabel("Reranker", cfg.Log.NoColor), cfg.RerankerEffective()),
		)
	}

	return strings.Join(lines, "\n") + "\n"
}

func joinStartupBanner(mark string, wordmark string) string {
	markLines := strings.Split(mark, "\n")
	wordmarkLines := strings.Split(wordmark, "\n")
	lines := make([]string, 0, max(len(markLines), len(wordmarkLines)))

	for index := range max(len(markLines), len(wordmarkLines)) {
		lines = append(lines, getStartupBannerLine(markLines, index)+"  "+getStartupBannerLine(wordmarkLines, index))
	}

	return strings.Join(lines, "\n")
}

func getStartupBannerLine(lines []string, index int) string {
	if index < 0 || index >= len(lines) {
		return ""
	}

	return lines[index]
}

func getConfigBoolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}

func getEffectiveStorageBackend(cfg *config.Config) string {
	if cfg == nil {
		return "sqlite"
	}

	backend := strings.TrimSpace(strings.ToLower(cfg.Storage.Backend))
	if backend == "" {
		return "sqlite"
	}

	return backend
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
	lis, err := listenFunc("tcp", fmt.Sprintf("%s:%d", cfg.RPC.Address, cfg.RPC.Port))
	if err != nil {
		return err
	}
	defer lis.Close()

	if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
		cfg.RPC.Port = tcpAddr.Port
	}
	if active := profile.Active(); strings.TrimSpace(active.HomeDir) != "" || strings.TrimSpace(active.RuntimePath) != "" {
		if _, err := writeRuntimeMetadata(cfg.RPC.Address, cfg.RPC.Port); err != nil {
			return err
		}
	}

	grpcSrv := server.New(agent, server.Options{Health: true})

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- grpcServerServe(grpcSrv, lis)
	}()

	log.Info().
		Str("rpcAddress", cfg.RPC.Address).
		Int("rpcPort", cfg.RPC.Port).
		Msg("RPC server listening for agent requests")

	select {
	case err := <-serverErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}

		return err
	case <-sigCtx.Done():
		log.Info().
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
		log.Warn().
			Msg("RPC graceful shutdown timed out, forcing stop")
		grpcSrv.Stop()
		<-stopped
	}

	if err := postShutdownServeErrHook(<-serverErr); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}

	log.Info().
		Msg("RPC server stopped")
	return nil
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "up",
		Usage: "Start the agent runtime",
		Flags: []cli.Flag{handcli.PersistentInstructFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, inputs, err := handcli.LoadConfig(cmd)
			if err != nil {
				return err
			}

			handcli.ApplyConfigOverrides(cmd, cfg)
			handcli.AddStartupFilesystemRoots(cfg, inputs)
			report := diagnostics.Build(inputs.EnvPath, inputs.ConfigPath, cfg, nil)
			if report.HasFailures() {
				return errors.New(report.FirstFailure())
			}
			auth, _ := cfg.ResolveModelAuth()

			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.Log.NoColor)
			logutils.SetLogLevel(cfg.Log.Level)

			if _, err := fmt.Fprint(startupOutput, renderStartupPanel(cfg)); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(startupOutput); err != nil {
				return err
			}

			log.Info().
				Str("name", cfg.Name).
				Str("model", cfg.Models.Main.Name).
				Str("provider", cfg.Models.Main.Provider).
				Str("summaryModel", cfg.SummaryModelEffective()).
				Str("summaryProvider", cfg.SummaryProviderEffective()).
				Str("storage", getEffectiveStorageBackend(cfg)).
				Bool("inputSafety", cfg.InputSafetyEnabled()).
				Bool("outputSafety", cfg.OutputSafetyEnabled()).
				Bool("piiSafety", cfg.OutputPIIRedactionEnabled()).
				Msg("Configuration loaded")
			if cfg.Search.Vector.Enabled {
				vectorLog := log.Info().
					Str("target", "session_and_memory_search").
					Str("embeddingModel", cfg.Models.Embedding.Name).
					Str("embeddingProvider", cfg.ModelEmbeddingProviderEffective()).
					Bool("rerankerEnabled", getConfigBoolDefault(cfg.Reranker.Enabled, true)).
					Bool("searchRerankEnabled", getConfigBoolDefault(cfg.Search.EnableRerank, true)).
					Str("reranker", cfg.RerankerEffective())
				if cfg.RerankerEffective() == "llm" {
					vectorLog = vectorLog.
						Str("rerankModel", cfg.RerankerModelEffective()).
						Str("rerankAPI", cfg.SummaryModelAPIEffective())
				}
				vectorLog.Msg("Vector retrieval configured")
			}

			startupLog := log.Info().
				Str("plan", "create_model_clients_start_agent_start_rpc_server").
				Str("rpcEndpoint", fmt.Sprintf("%s:%d", cfg.RPC.Address, cfg.RPC.Port)).
				Bool("streaming", cfg.StreamEnabled()).
				Bool("traceEnabled", cfg.Trace.Enabled)
			if cfg.Trace.Enabled {
				traceDir := strings.TrimSpace(cfg.Trace.Disk.Dir)
				startupLog = startupLog.Str("traceDir", traceDir)
			}
			startupLog.Msg("Starting Hand services")

			modelClient, err := modelClientFactory.NewClient(
				modelClientRequest(
					modelclient.ModelRoleMain,
					cfg.Models.Main.Name,
					auth,
					cfg.ModelMaxRetriesEffective(),
				),
			)
			if err != nil {
				return err
			}

			summaryAuth, err := resolveSummaryAuth(cfg)
			if err != nil {
				return err
			}

			var summaryClient models.Client
			if config.ModelAuthEqual(auth, summaryAuth) {
				summaryClient = modelClient
			} else {
				summaryClient, err = modelClientFactory.NewClient(modelClientRequest(modelclient.ModelRoleSummary, cfg.SummaryModelEffective(), summaryAuth, cfg.ModelMaxRetriesEffective()))
				if err != nil {
					return err
				}
			}

			agent := newAgentRunner(ctx, cfg, modelClient, summaryClient)
			if err := agent.Start(ctx); err != nil {
				return err
			}

			return serveRPC(ctx, cfg, agent)
		},
	}
}

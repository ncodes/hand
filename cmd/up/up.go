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
	"github.com/wandxy/hand/internal/constants"
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
	colorGray              = "\x1b[90m"
	colorReset             = "\x1b[0m"
	startupLogoColumnWidth = 64
	startupColumnGap       = 3
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

	detailRows := getStartupDetailRows(cfg)
	panel := renderStartupBannerPanel(handBadge, detailRows, cfg.Log.NoColor)

	return "\n" + panel + "\n\n"
}

type startupDetailRow struct {
	label string
	value string
}

func getStartupDetailRows(cfg *config.Config) []startupDetailRow {
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

	rows := []startupDetailRow{
		{label: "Version", value: formatStartupVersion()},
		{label: "Instance", value: cfg.Name},
		{label: "Model", value: cfg.Models.Main.Name},
		{label: "Provider", value: cfg.Models.Main.Provider},
		{label: "Summary model", value: cfg.SummaryModelEffective()},
		{label: "Summary provider", value: cfg.SummaryProviderEffective()},
		{label: "Storage", value: getEffectiveStorageBackend(cfg)},
	}
	if cfg.SummaryModelAPIEffective() != cfg.Models.Main.API {
		rows = append(rows, startupDetailRow{label: "Summary API", value: cfg.SummaryModelAPIEffective()})
	}
	rows = append(rows,
		startupDetailRow{label: "Streaming", value: fmt.Sprintf("%t", cfg.StreamEnabled())},
		startupDetailRow{label: "RPC", value: fmt.Sprintf("%s:%d", cfg.RPC.Address, cfg.RPC.Port)},
		startupDetailRow{label: "Logs", value: fmt.Sprintf("%s (%s)", cfg.Log.Level, logStyle)},
		startupDetailRow{label: "Debug requests", value: debugRequests},
		startupDetailRow{label: "Traces", value: traceStatus},
		startupDetailRow{label: "Safety", value: handcli.SafetySummary(cfg)},
	)
	if cfg.Search.Vector.Enabled {
		rows = append(rows,
			startupDetailRow{label: "Embedding model", value: cfg.Models.Embedding.Name},
			startupDetailRow{label: "Embedding provider", value: cfg.ModelEmbeddingProviderEffective()},
			startupDetailRow{label: "Reranker", value: cfg.RerankerEffective()},
		)
	}

	return rows
}

func formatStartupVersion() string {
	version := strings.TrimSpace(constants.AppVersion)
	if version == "" {
		version = "dev"
	}

	commit := strings.TrimSpace(constants.CommitHash)
	if commit == "" {
		commit = "unknown"
	}

	return fmt.Sprintf("%s (commit %s)", version, commit)
}

func renderStartupBannerPanel(logo string, rows []startupDetailRow, noColor bool) string {
	logoLines := centerStartupBlockLines(splitStartupLines(logo), startupLogoColumnWidth)
	detailLines := renderStartupDetailLines(rows, noColor)
	height := max(len(logoLines), len(detailLines))
	logoLines = centerStartupBlockVertically(logoLines, height, startupLogoColumnWidth)
	detailLines = padStartupBlockVertically(detailLines, height)

	lines := make([]string, 0, height)
	gap := strings.Repeat(" ", startupColumnGap)
	divider := styleStartup("│", noColor)
	for index := range height {
		lines = append(lines, styleStartup(logoLines[index], noColor)+gap+divider+gap+detailLines[index])
	}

	return strings.Join(lines, "\n")
}

func renderStartupDetailLines(rows []startupDetailRow, noColor bool) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%s %s", styleLabel(row.label, noColor), row.value))
	}

	return lines
}

func splitStartupLines(value string) []string {
	if value == "" {
		return nil
	}

	return strings.Split(value, "\n")
}

func centerStartupBlockLines(lines []string, width int) []string {
	centered := make([]string, 0, len(lines))
	for _, line := range lines {
		centered = append(centered, centerStartupLine(line, width))
	}

	return centered
}

func centerStartupLine(line string, width int) string {
	lineWidth := len([]rune(line))
	if lineWidth >= width {
		return line
	}

	leftPadding := (width - lineWidth) / 2
	rightPadding := width - lineWidth - leftPadding
	return strings.Repeat(" ", leftPadding) + line + strings.Repeat(" ", rightPadding)
}

func centerStartupBlockVertically(lines []string, height int, width int) []string {
	if len(lines) >= height {
		return lines
	}

	topPadding := (height - len(lines)) / 2
	bottomPadding := height - len(lines) - topPadding
	padded := make([]string, 0, height)
	padded = appendStartupBlankLines(padded, topPadding, width)
	padded = append(padded, lines...)
	padded = appendStartupBlankLines(padded, bottomPadding, width)

	return padded
}

func padStartupBlockVertically(lines []string, height int) []string {
	if len(lines) >= height {
		return lines
	}

	padded := make([]string, 0, height)
	padded = append(padded, lines...)
	padded = appendStartupBlankLines(padded, height-len(lines), 0)

	return padded
}

func appendStartupBlankLines(lines []string, count int, width int) []string {
	for range count {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return lines
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

			log.Info().Msg("Configuration loaded")
			if cfg.Search.Vector.Enabled {
				log.Info().Msg("Vector retrieval configured")
			}

			log.Info().Msg("Starting Hand services")

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

package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"

	handagent "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/brand"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/diagnostics"
	"github.com/wandxy/hand/internal/gateway"
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

type closeableAgentRunner interface {
	Close() error
}

const (
	colorGray              = "\x1b[90m"
	colorReset             = "\x1b[0m"
	startupLogoColumnWidth = 64
	startupColumnGap       = 3
)

var startupLogoColors = []string{
	"\x1b[38;5;38m",
	"\x1b[38;5;44m",
	"\x1b[38;5;49m",
	"\x1b[38;5;48m",
	"\x1b[38;5;83m",
}

var handBadge = joinStartupBanner(brand.Mark, brand.Wordmark)

var startupOutput io.Writer = os.Stdout

var osStat = os.Stat

var daemonConfigWatchDebounce = 200 * time.Millisecond

var newConfigWatcher = newFSNotifyConfigWatcher

var createFSNotifyWatcher = fsnotify.NewWatcher

var mkdirAllConfigWatchDir = os.MkdirAll

var addConfigWatchDir = func(watcher *fsnotify.Watcher, path string) error {
	return watcher.Add(path)
}

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
	summaryClient,
	rerankerClient models.Client,
) agentRunner {
	return handagent.NewAgent(ctx, cfg, modelClient, summaryClient, rerankerClient)
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

var openRPCListener = openRPCListenerImpl

type gatewayManager interface {
	Start(context.Context, config.GatewayConfig, gateway.Responder) error
	Stop(context.Context) error
	Wait() <-chan error
}

var newGatewayManager = func() gatewayManager {
	return gateway.NewManager(gateway.Options{})
}

var stopGatewayTimeout = 5 * time.Second

type modelClientFactoryAPI interface {
	NewClient(modelclient.ClientRequest) (models.Client, error)
}

var modelClientFactory modelClientFactoryAPI = modelclient.NewDefaultClientFactory()

// resolveSummaryAuth resolves summary model credentials (hooked in tests).
var resolveSummaryAuth = func(cfg *config.Config) (config.ModelAuth, error) {
	return cfg.ResolveSummaryModelAuth()
}

var resolveRerankerAuth = func(cfg *config.Config) (config.ModelAuth, error) {
	return cfg.ResolveRerankerModelAuth()
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

func rerankerModelClientRequired(cfg *config.Config) bool {
	if cfg == nil || !cfg.Search.Vector.Enabled {
		return false
	}
	if cfg.Reranker.Enabled != nil && !*cfg.Reranker.Enabled {
		return false
	}
	if cfg.Search.EnableRerank != nil && !*cfg.Search.EnableRerank {
		return false
	}
	if cfg.RerankerEffective() == constants.RerankerLLM {
		return true
	}
	for _, override := range cfg.Reranker.Overrides {
		if cfg.RerankerOverrideEffective(override).Type == constants.RerankerLLM {
			return true
		}
	}

	return false
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
		startupDetailRow{label: "Gateway", value: getGatewayStartupSummary(cfg)},
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

func getGatewayStartupSummary(cfg *config.Config) string {
	if cfg == nil || !cfg.Gateway.Enabled {
		return "disabled"
	}

	parts := []string{fmt.Sprintf("%s:%d", cfg.Gateway.Address, cfg.Gateway.Port)}
	if cfg.Gateway.Telegram.Enabled {
		parts = append(parts, "telegram="+cfg.Gateway.Telegram.Mode)
	}
	if cfg.Gateway.Slack.Enabled {
		parts = append(parts, "slack="+cfg.Gateway.Slack.Mode)
	}

	return strings.Join(parts, " ")
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
	logoLines := splitStartupLines(logo)
	detailLines := renderStartupDetailLines(rows, noColor)
	height := max(len(logoLines), len(detailLines))
	logoLines = renderStartupLogoLines(logoLines, height, noColor)
	detailLines = padStartupBlockVertically(detailLines, height)

	lines := make([]string, 0, height)
	gap := strings.Repeat(" ", startupColumnGap)
	divider := styleStartup("│", noColor)
	for index := range height {
		lines = append(lines, logoLines[index]+gap+divider+gap+detailLines[index])
	}

	return strings.Join(lines, "\n")
}

func renderStartupLogoLines(lines []string, height int, noColor bool) []string {
	if len(lines) == 0 {
		return padStartupBlockVertically(nil, height)
	}

	topPadding := max((height-len(lines))/2, 0)
	rendered := make([]string, 0, height)
	rendered = appendStartupBlankLines(rendered, topPadding, startupLogoColumnWidth)
	for index, line := range lines {
		rendered = append(rendered, styleStartupLogoLine(centerStartupLine(line, startupLogoColumnWidth), index, noColor))
	}
	rendered = appendStartupBlankLines(rendered, height-len(rendered), startupLogoColumnWidth)

	return rendered
}

func styleStartupLogoLine(line string, index int, noColor bool) string {
	if noColor {
		return line
	}

	color := startupLogoColors[min(index, len(startupLogoColors)-1)]
	return color + line + colorReset
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

func centerStartupLine(line string, width int) string {
	lineWidth := len([]rune(line))
	if lineWidth >= width {
		return line
	}

	leftPadding := (width - lineWidth) / 2
	rightPadding := width - lineWidth - leftPadding
	return strings.Repeat(" ", leftPadding) + line + strings.Repeat(" ", rightPadding)
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

type daemonConfigSnapshot struct {
	cfg         *config.Config
	inputs      handcli.ConfigInputs
	fingerprint configFileFingerprint
}

type configFileFingerprint struct {
	modTime time.Time
	size    int64
}

func loadDaemonConfig(cmd *cli.Command) (daemonConfigSnapshot, error) {
	cfg, inputs, err := handcli.LoadConfig(cmd)
	if err != nil {
		return daemonConfigSnapshot{}, err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)
	handcli.AddStartupFilesystemRoots(cfg, inputs)
	report := diagnostics.Build(inputs.EnvPath, inputs.ConfigPath, cfg, nil)
	if report.HasFailures() {
		return daemonConfigSnapshot{}, errors.New(report.FirstFailure())
	}

	fingerprint, err := getConfigFileFingerprint(inputs.ConfigPath)
	if err != nil {
		return daemonConfigSnapshot{}, err
	}

	return daemonConfigSnapshot{
		cfg:         cfg,
		inputs:      inputs,
		fingerprint: fingerprint,
	}, nil
}

func getConfigFileFingerprint(path string) (configFileFingerprint, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return configFileFingerprint{}, errors.New("config path is required")
	}

	info, err := osStat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return configFileFingerprint{}, nil
		}

		return configFileFingerprint{}, err
	}

	return configFileFingerprint{modTime: info.ModTime(), size: info.Size()}, nil
}

func hasConfigFileChanged(path string, previous configFileFingerprint) (configFileFingerprint, bool, error) {
	current, err := getConfigFileFingerprint(path)
	if err != nil {
		return configFileFingerprint{}, false, err
	}

	return current, current != previous, nil
}

type configWatcher struct {
	events <-chan fsnotify.Event
	errors <-chan error
	close  func() error
}

func newFSNotifyConfigWatcher(configPath string) (configWatcher, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return configWatcher{}, errors.New("config path is required")
	}

	watcher, err := createFSNotifyWatcher()
	if err != nil {
		return configWatcher{}, err
	}

	configDir := filepath.Dir(configPath)
	if err := mkdirAllConfigWatchDir(configDir, 0o700); err != nil {
		_ = watcher.Close()
		return configWatcher{}, err
	}
	if err := addConfigWatchDir(watcher, configDir); err != nil {
		_ = watcher.Close()
		return configWatcher{}, err
	}

	return configWatcher{
		events: watcher.Events,
		errors: watcher.Errors,
		close:  watcher.Close,
	}, nil
}

func isConfigFileWatchEvent(event fsnotify.Event, configPath string) bool {
	if strings.TrimSpace(event.Name) == "" {
		return false
	}

	eventPath := filepath.Clean(event.Name)
	targetPath := filepath.Clean(configPath)
	if eventPath != targetPath {
		return false
	}

	reloadOps := fsnotify.Write | fsnotify.Create | fsnotify.Rename | fsnotify.Remove
	return event.Op&reloadOps != 0
}

func runDaemonWithConfigRestarts(ctx context.Context, cmd *cli.Command, debounce time.Duration) error {
	if debounce <= 0 {
		debounce = daemonConfigWatchDebounce
	}

	snapshot, err := loadDaemonConfig(cmd)
	if err != nil {
		return err
	}

	for {
		next, restart, err := runDaemonUntilConfigChange(ctx, cmd, snapshot, debounce)
		if err != nil || !restart {
			return err
		}

		snapshot = next
	}
}

func runDaemonUntilConfigChange(
	ctx context.Context,
	cmd *cli.Command,
	snapshot daemonConfigSnapshot,
	debounce time.Duration,
) (daemonConfigSnapshot, bool, error) {
	watcher, err := newConfigWatcher(snapshot.inputs.ConfigPath)
	if err != nil {
		return daemonConfigSnapshot{}, false, err
	}
	defer watcher.close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runDaemonOnce(runCtx, snapshot.cfg)
	}()

	var reload <-chan time.Time
	var timer *time.Timer
	defer stopConfigReloadTimer(timer)

	lastFingerprint := snapshot.fingerprint
	lastInvalidFingerprint := configFileFingerprint{size: -1}
	for {
		select {
		case err := <-done:
			return daemonConfigSnapshot{}, false, err
		case <-ctx.Done():
			err := waitForDaemonStop(cancel, done)
			return daemonConfigSnapshot{}, false, err
		case err, ok := <-watcher.errors:
			if !ok {
				watcher.errors = nil
				continue
			}
			log.Error().Err(err).Msg("Config file watcher failed")
		case event, ok := <-watcher.events:
			if !ok {
				watcher.events = nil
				continue
			}
			if !isConfigFileWatchEvent(event, snapshot.inputs.ConfigPath) {
				continue
			}
			timer, reload = resetConfigReloadTimer(timer, debounce)
		case <-reload:
			reload = nil
			fingerprint, changed, err := hasConfigFileChanged(snapshot.inputs.ConfigPath, lastFingerprint)
			if err != nil {
				if fingerprint != lastInvalidFingerprint {
					log.Error().Err(err).Msg("Config reload check failed")
					lastInvalidFingerprint = fingerprint
				}
				continue
			}
			if !changed {
				continue
			}

			next, err := loadDaemonConfig(cmd)
			if err != nil {
				if fingerprint != lastInvalidFingerprint {
					log.Error().Err(err).Msg("Config reload validation failed")
					lastInvalidFingerprint = fingerprint
				}
				lastFingerprint = fingerprint
				continue
			}

			log.Info().Msg("Configuration changed; restarting Hand services")
			cancel()
			if err := <-done; err != nil {
				return daemonConfigSnapshot{}, false, err
			}

			return next, true, nil
		}
	}
}

func waitForDaemonStop(cancel context.CancelFunc, done <-chan error) error {
	cancel()
	return <-done
}

func resetConfigReloadTimer(timer *time.Timer, debounce time.Duration) (*time.Timer, <-chan time.Time) {
	if debounce <= 0 {
		debounce = daemonConfigWatchDebounce
	}

	if timer == nil {
		timer = time.NewTimer(debounce)
		return timer, timer.C
	}

	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(debounce)

	return timer, timer.C
}

func stopConfigReloadTimer(timer *time.Timer) {
	if timer == nil {
		return
	}

	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func runDaemonOnce(ctx context.Context, cfg *config.Config) error {
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

	rerankerClient := summaryClient
	if rerankerModelClientRequired(cfg) {
		rerankerAuth, err := resolveRerankerAuth(cfg)
		if err != nil {
			return err
		}
		switch {
		case config.ModelAuthEqual(auth, rerankerAuth):
			rerankerClient = modelClient
		case config.ModelAuthEqual(summaryAuth, rerankerAuth):
			rerankerClient = summaryClient
		default:
			rerankerClient, err = modelClientFactory.NewClient(modelClientRequest(modelclient.ModelRoleReranker, cfg.RerankerModelEffective(), rerankerAuth, cfg.ModelMaxRetriesEffective()))
			if err != nil {
				return err
			}
		}
	}

	lis, err := openRPCListener(cfg)
	if err != nil {
		return err
	}
	defer lis.Close()

	agent := newAgentRunner(ctx, cfg, modelClient, summaryClient, rerankerClient)
	if err := agent.Start(ctx); err != nil {
		_ = lis.Close()
		return err
	}

	err = serveDaemonServices(ctx, cfg, agent, lis)
	if closer, ok := agent.(closeableAgentRunner); ok {
		if closeErr := closer.Close(); err == nil {
			if isMissingCredentialLockError(closeErr) {
				log.Debug().Err(closeErr).Msg("Ignoring missing credential lock during shutdown")
			} else {
				err = closeErr
			}
		}
	}

	return err
}

func serveDaemonServices(ctx context.Context, cfg *config.Config, agent agentRunner, lis net.Listener) error {
	if cfg == nil || !cfg.Gateway.Enabled {
		return serveRPC(ctx, cfg, agent, lis)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	manager := newGatewayManager()
	if err := manager.Start(runCtx, cfg.Gateway, agent); err != nil {
		_ = lis.Close()
		return err
	}

	rpcDone := make(chan error, 1)
	go func() {
		rpcDone <- serveRPC(runCtx, cfg, agent, lis)
	}()

	var err error
	select {
	case err = <-rpcDone:
		cancel()
		stopGatewayWithTimeout(manager)
		return err
	case err = <-manager.Wait():
		cancel()
		rpcErr := <-rpcDone
		if err != nil {
			return err
		}

		return rpcErr
	case <-ctx.Done():
		cancel()
		stopGatewayWithTimeout(manager)
		return <-rpcDone
	}
}

func stopGatewayWithTimeout(manager gatewayManager) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), stopGatewayTimeout)
	defer cancel()
	if err := manager.Stop(shutdownCtx); err != nil {
		log.Warn().Err(err).Msg("Gateway shutdown failed")
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

var serveRPC = func(ctx context.Context, cfg *config.Config, agent agentRunner, lis net.Listener) error {
	defer lis.Close()

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
		Msg("RPC server listening for daemon requests")

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
		Name:  "daemon",
		Usage: "Manage the Hand daemon",
		Flags: []cli.Flag{handcli.PersistentInstructFlag()},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the Hand daemon",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runDaemonWithConfigRestarts(ctx, cmd, daemonConfigWatchDebounce)
				},
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

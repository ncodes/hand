package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	cli "github.com/urfave/cli/v3"

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
	startTUIDaemonProcess         = startTUIDaemonProcessImpl
	getTUIDaemonExecutable        = os.Executable
	releaseTUIDaemonProcess       = func(process *os.Process) error { return process.Release() }
	daemonBootstrapInitialTimeout = defaultDaemonBootstrapInitialTimeout
	daemonBootstrapReadyTimeout   = defaultDaemonBootstrapReadyTimeout
	daemonBootstrapPollInterval   = defaultDaemonBootstrapPollInterval
)

func ensureTUIDaemonRunningImpl(
	ctx context.Context,
	cmd *cli.Command,
	cfg *config.Config,
	inputs handcli.ConfigInputs,
) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	if err := waitForTUIDaemonRPC(ctx, cfg, daemonBootstrapInitialTimeout); err == nil {
		return nil
	}

	args := getTUIDaemonStartArgs(cmd, inputs)
	if err := startTUIDaemonProcess(args); err != nil {
		return err
	}
	if err := waitForTUIDaemonRPC(ctx, cfg, daemonBootstrapReadyTimeout); err != nil {
		return fmt.Errorf("start Hand daemon: RPC did not become ready at %s:%d: %w",
			strings.TrimSpace(cfg.RPC.Address), cfg.RPC.Port, err)
	}

	return nil
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

func startTUIDaemonProcessImpl(args []string) error {
	executable, err := getTUIDaemonExecutable()
	if err != nil {
		return fmt.Errorf("resolve hand executable: %w", err)
	}

	command := exec.Command(executable, args...)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Hand daemon: %w", err)
	}

	if err := releaseTUIDaemonProcess(command.Process); err != nil {
		return fmt.Errorf("release Hand daemon process: %w", err)
	}

	return nil
}

type tuiDaemonForwardedFlag struct {
	name string
	kind string
}

var tuiDaemonForwardedFlags = []tuiDaemonForwardedFlag{
	{name: "name", kind: "string"},
	{name: "model.provider", kind: "string"},
	{name: "model.api-key", kind: "string"},
	{name: "model", kind: "string"},
	{name: "model.summary", kind: "string"},
	{name: "model.stream", kind: "bool"},
	{name: "model.base-url", kind: "string"},
	{name: "model.summary-provider", kind: "string"},
	{name: "model.summary-base-url", kind: "string"},
	{name: "model.summary-api", kind: "string"},
	{name: "model.api", kind: "string"},
	{name: "model.max-retries", kind: "int"},
	{name: "rpc.address", kind: "string"},
	{name: "rpc.port", kind: "int"},
	{name: "gateway.enabled", kind: "bool"},
	{name: "gateway.address", kind: "string"},
	{name: "gateway.port", kind: "int"},
	{name: "gateway.auth-token", kind: "string"},
	{name: "gateway.telegram.enabled", kind: "bool"},
	{name: "gateway.telegram.mode", kind: "string"},
	{name: "gateway.telegram.bot-token", kind: "string"},
	{name: "gateway.telegram.webhook-secret", kind: "string"},
	{name: "gateway.slack.enabled", kind: "bool"},
	{name: "gateway.slack.mode", kind: "string"},
	{name: "gateway.slack.response-mode", kind: "string"},
	{name: "gateway.slack.bot-token", kind: "string"},
	{name: "gateway.slack.app-token", kind: "string"},
	{name: "gateway.slack.signing-secret", kind: "string"},
	{name: "max-iterations", kind: "int"},
	{name: "log.level", kind: "string"},
	{name: "log.no-color", kind: "bool"},
	{name: "debug.requests", kind: "bool"},
	{name: "trace.enabled", kind: "bool"},
	{name: "trace.disk.enabled", kind: "bool"},
	{name: "trace.disk.dir", kind: "string"},
	{name: "trace.database.enabled", kind: "bool"},
	{name: "trace.database.max-events-per-session", kind: "int"},
	{name: "tui.thinking-composer", kind: "bool"},
	{name: "web.provider", kind: "string"},
	{name: "web.key", kind: "string"},
	{name: "web.base-url", kind: "string"},
	{name: "web.max-char-per-result", kind: "int"},
	{name: "web.max-extract-char-per-result", kind: "int"},
	{name: "web.max-extract-response-bytes", kind: "int"},
	{name: "web.cache-ttl", kind: "duration"},
	{name: "web.blocked-domains-enabled", kind: "bool"},
	{name: "web.blocked-domains", kind: "string"},
	{name: "web.blocked-domain-files", kind: "string"},
	{name: "web.native-allowed-hosts", kind: "string"},
	{name: "web.native-blocked-hosts", kind: "string"},
	{name: "web.native-allowed-host-files", kind: "string"},
	{name: "web.native-blocked-host-files", kind: "string"},
	{name: "web.extract-min-summarize-chars", kind: "int"},
	{name: "web.extract-max-summary-chars", kind: "int"},
	{name: "web.extract-max-summary-chunk-chars", kind: "int"},
	{name: "web.extract-refusal-threshold-chars", kind: "int"},
	{name: "rules.files", kind: "string"},
	{name: "platform", kind: "string"},
	{name: "fs.roots", kind: "string"},
	{name: "cap.fs", kind: "bool"},
	{name: "cap.net", kind: "bool"},
	{name: "cap.exec", kind: "bool"},
	{name: "cap.mem", kind: "bool"},
	{name: "cap.browser", kind: "bool"},
	{name: "exec.allow", kind: "string"},
	{name: "exec.ask", kind: "string"},
	{name: "exec.deny", kind: "string"},
	{name: "storage.backend", kind: "string"},
	{name: "memory.backend", kind: "string"},
	{name: "session.default-idle-expiry", kind: "duration"},
	{name: "session.archive-retention", kind: "duration"},
}

func getTUIDaemonStartArgs(cmd *cli.Command, inputs handcli.ConfigInputs) []string {
	args := make([]string, 0, 8)
	if profileName := strings.TrimSpace(inputs.Profile.Name); profileName != "" {
		args = append(args, "--profile", profileName)
	}
	if envPath := strings.TrimSpace(inputs.EnvPath); envPath != "" {
		args = append(args, "--env-file", envPath)
	}
	if configPath := strings.TrimSpace(inputs.ConfigPath); configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = appendTUIDaemonForwardedFlags(args, cmd)

	return append(args, "daemon", "start")
}

func appendTUIDaemonForwardedFlags(args []string, cmd *cli.Command) []string {
	if cmd == nil {
		return args
	}

	for _, flag := range tuiDaemonForwardedFlags {
		if !cmd.IsSet(flag.name) {
			continue
		}
		switch flag.kind {
		case "bool":
			args = append(args, fmt.Sprintf("--%s=%t", flag.name, cmd.Bool(flag.name)))
		case "int":
			args = append(args, fmt.Sprintf("--%s=%d", flag.name, cmd.Int(flag.name)))
		case "duration":
			args = append(args, fmt.Sprintf("--%s=%s", flag.name, cmd.Duration(flag.name)))
		default:
			args = append(args, fmt.Sprintf("--%s=%s", flag.name, cmd.String(flag.name)))
		}
	}

	return args
}

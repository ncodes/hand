package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	urfavecli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	clidaemon "github.com/wandxy/hand/internal/cli/daemon"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
	handruntime "github.com/wandxy/hand/internal/runtime"
	"github.com/wandxy/hand/pkg/logutils"
)

const (
	defaultDaemonBootstrapInitialTimeout = 250 * time.Millisecond
	defaultDaemonBootstrapReadyTimeout   = 10 * time.Second
	defaultDaemonBootstrapPollInterval   = 100 * time.Millisecond
)

var (
	checkDaemonRPC                = checkDaemonRPCImpl
	checkDaemonHealth             = checkDaemonHealthImpl
	probeActiveRuntime            = handruntime.Probe
	daemonStatusNow               = time.Now
	startDaemonRuntime            = startDaemonRuntimeImpl
	runDaemonRuntimeOnce          = RunDaemonOnce
	setDaemonOutput               = clidaemon.SetOutput
	runDaemonWithConfigRestarts   = clidaemon.RunWithConfigRestarts
	runDaemonOnce                 = clidaemon.RunOnce
	daemonBootstrapInitialTimeout = defaultDaemonBootstrapInitialTimeout
	daemonBootstrapReadyTimeout   = defaultDaemonBootstrapReadyTimeout
	daemonBootstrapPollInterval   = defaultDaemonBootstrapPollInterval
)

type DaemonStatus struct {
	State     string
	Health    string
	Profile   string
	PID       int
	Address   string
	Port      int
	StartedAt time.Time
	Uptime    time.Duration
}

func SetDaemonOutput(w io.Writer) io.Writer {
	return setDaemonOutput(w)
}

func RunDaemonWithConfigRestarts(ctx context.Context, cmd *urfavecli.Command) error {
	return runDaemonWithConfigRestarts(ctx, cmd, daemonDependencies())
}

func RunDaemonOnce(ctx context.Context, cfg *config.Config) error {
	return runDaemonOnce(ctx, cfg)
}

func GetDaemonStatus(ctx context.Context) (DaemonStatus, error) {
	probe := probeActiveRuntime(ctx, profile.Active())
	status := daemonStatusFromProbe(probe)
	if probe.State != handruntime.ProbeStateReady {
		if probe.Err != nil {
			return status, fmt.Errorf("daemon is %s: %w", probe.State, probe.Err)
		}

		return status, fmt.Errorf("daemon is %s", probe.State)
	}

	health, err := checkDaemonHealth(ctx, status.Address, status.Port)
	if err != nil {
		return status, fmt.Errorf("daemon health check failed: %w", err)
	}

	status.State = "running"
	status.Health = health
	return status, nil
}

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

	_, err := checkDaemonHealth(ctx, address, cfg.RPC.Port)
	return err
}

func checkDaemonHealthImpl(ctx context.Context, address string, port int) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", fmt.Errorf("rpc address is required")
	}
	if port <= 0 {
		return "", fmt.Errorf("rpc port must be greater than zero")
	}

	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", address, port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return "", err
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return "", fmt.Errorf("daemon health status is %s", resp.GetStatus())
	}

	return resp.GetStatus().String(), nil
}

func daemonStatusFromProbe(probe handruntime.ProbeResult) DaemonStatus {
	metadata := probe.Metadata
	status := DaemonStatus{
		State:     string(probe.State),
		Profile:   strings.TrimSpace(metadata.Profile),
		PID:       metadata.PID,
		Address:   strings.TrimSpace(metadata.RPC.Address),
		Port:      metadata.RPC.Port,
		StartedAt: metadata.StartedAt,
	}
	if status.Profile == "" {
		status.Profile = profile.DefaultName
	}
	if !status.StartedAt.IsZero() {
		status.Uptime = max(daemonStatusNow().Sub(status.StartedAt).Round(time.Second), 0)
	}

	return status
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

func daemonDependencies() clidaemon.Dependencies {
	return clidaemon.Dependencies{
		LoadConfig: func(cmd *urfavecli.Command) (*config.Config, clidaemon.ConfigInputs, error) {
			cfg, inputs, err := LoadConfig(cmd)
			return cfg, daemonConfigInputs(inputs), err
		},
		ApplyConfigOverrides: func(cmd *urfavecli.Command, cfg *config.Config) {
			ApplyConfigOverrides(cmd, cfg)
		},
		AddStartupFilesystemRoots: func(cfg *config.Config, inputs clidaemon.ConfigInputs) {
			AddStartupFilesystemRoots(cfg, ConfigInputs{
				Profile:    inputs.Profile,
				EnvPath:    inputs.EnvPath,
				ConfigPath: inputs.ConfigPath,
			})
		},
		SafetySummary: SafetySummary,
	}
}

func daemonConfigInputs(inputs ConfigInputs) clidaemon.ConfigInputs {
	return clidaemon.ConfigInputs{
		Profile:    inputs.Profile,
		EnvPath:    inputs.EnvPath,
		ConfigPath: inputs.ConfigPath,
	}
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/internal/profile"
	"github.com/wandxy/morph/pkg/stringx"
)

// RPC describes a daemon RPC endpoint.
type RPC struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// Metadata describes the running Morph daemon endpoint for one profile.
type Metadata struct {
	Profile   string    `json:"profile"`
	PID       int       `json:"pid"`
	RPC       RPC       `json:"rpc"`
	StartedAt time.Time `json:"started_at"`
}

// ProbeState identifies the result category for a read-only daemon probe.
type ProbeState string

const (
	ProbeStateReady   ProbeState = "ready"
	ProbeStateMissing ProbeState = "missing"
	ProbeStateInvalid ProbeState = "invalid"
	ProbeStateStale   ProbeState = "stale"
)

// ProbeResult describes daemon runtime readiness without mutating runtime metadata.
type ProbeResult struct {
	State    ProbeState
	Metadata Metadata
	Err      error
}

var (
	now           = time.Now
	processPID    = os.Getpid
	checkPID      = checkProcess
	dialRuntime   = dialRuntimeEndpoint
	marshalJSON   = json.MarshalIndent
	mkdirAll      = os.MkdirAll
	writeFile     = os.WriteFile
	readFile      = os.ReadFile
	removeFile    = os.Remove
	findProcess   = os.FindProcess
	signalProcess = func(process *os.Process, signal os.Signal) error {
		return process.Signal(signal)
	}
)

// WriteActive writes runtime metadata for the active profile.
func WriteActive(address string, port int) (Metadata, error) {
	active := profile.WithMetadataPaths(profile.Active())
	return Write(active, address, port)
}

// Write describes runtime metadata under the supplied profile home.
func Write(active profile.Profile, address string, port int) (Metadata, error) {
	active = profile.WithMetadataPaths(active)
	if stringx.String(active.RuntimePath).Trim() == "" {
		return Metadata{}, errors.New("profile runtime path is required")
	}

	metadata := Metadata{
		Profile:   stringx.String(active.Name).Trim(),
		PID:       processPID(),
		RPC:       RPC{Address: stringx.String(address).Trim(), Port: port},
		StartedAt: now().UTC(),
	}
	if metadata.Profile == "" {
		metadata.Profile = profile.DefaultName
	}
	if metadata.RPC.Address == "" {
		return Metadata{}, errors.New("runtime rpc address is required")
	}
	if metadata.RPC.Port <= 0 {
		return Metadata{}, errors.New("runtime rpc port must be greater than zero")
	}

	data, err := marshalJSON(metadata, "", "  ")
	if err != nil {
		return Metadata{}, err
	}
	data = append(data, '\n')

	if err := mkdirAll(filepath.Dir(active.RuntimePath), 0o700); err != nil {
		return Metadata{}, fmt.Errorf("create runtime metadata dir: %w", err)
	}
	if err := writeFile(active.RuntimePath, data, 0o600); err != nil {
		return Metadata{}, fmt.Errorf("write runtime metadata: %w", err)
	}

	profile.SetActive(active)
	return metadata, nil
}

// LoadActive loads runtime metadata from the active profile.
func LoadActive() (Metadata, error) {
	return Load(profile.Active())
}

// Load reads runtime metadata from the supplied profile.
func Load(active profile.Profile) (Metadata, error) {
	active = profile.WithMetadataPaths(active)
	if stringx.String(active.RuntimePath).Trim() == "" {
		return Metadata{}, errors.New("profile runtime path is required")
	}

	data, err := readFile(active.RuntimePath)
	if err != nil {
		return Metadata{}, err
	}

	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("parse runtime metadata: %w", err)
	}
	if stringx.String(metadata.RPC.Address).Trim() == "" {
		return Metadata{}, errors.New("runtime rpc address is required")
	}
	if metadata.RPC.Port <= 0 {
		return Metadata{}, errors.New("runtime rpc port must be greater than zero")
	}

	return metadata, nil
}

// Probe checks runtime metadata, PID, and endpoint reachability without removing stale metadata.
func Probe(ctx context.Context, active profile.Profile) ProbeResult {
	metadata, err := Load(active)
	if err != nil {
		if os.IsNotExist(err) {
			return ProbeResult{State: ProbeStateMissing, Err: fmt.Errorf("runtime metadata is not present")}
		}

		return ProbeResult{State: ProbeStateInvalid, Err: err}
	}
	if err := checkPID(metadata.PID); err != nil {
		return ProbeResult{State: ProbeStateStale, Metadata: metadata, Err: err}
	}
	if err := dialRuntime(ctx, metadata.RPC.Address, metadata.RPC.Port); err != nil {
		return ProbeResult{State: ProbeStateStale, Metadata: metadata, Err: err}
	}

	return ProbeResult{State: ProbeStateReady, Metadata: metadata}
}

// ResolveRPC returns the RPC endpoint for cfg, preferring profile runtime metadata unless RPC was explicitly configured.
func ResolveRPC(ctx context.Context, cmd *cli.Command, cfg *config.Config) (config.RPCConfig, error) {
	if cfg == nil {
		return config.RPCConfig{}, errors.New("config is required")
	}
	if hasExplicitRPC(cmd, cfg) {
		return cfg.RPC, nil
	}

	metadata, err := LoadActive()
	if err != nil {
		if os.IsNotExist(err) {
			return cfg.RPC, nil
		}
		return config.RPCConfig{}, err
	}
	if err := checkPID(metadata.PID); err != nil {
		removeActiveRuntimeMetadata()
		return cfg.RPC, nil
	}
	if err := dialRuntime(ctx, metadata.RPC.Address, metadata.RPC.Port); err != nil {
		removeActiveRuntimeMetadata()
		return cfg.RPC, nil
	}

	return config.RPCConfig{Address: metadata.RPC.Address, Port: metadata.RPC.Port}, nil
}

func removeActiveRuntimeMetadata() {
	active := profile.WithMetadataPaths(profile.Active())
	if stringx.String(active.RuntimePath).Trim() == "" {
		return
	}

	_ = removeFile(active.RuntimePath)
}

func hasExplicitRPC(cmd *cli.Command, cfg *config.Config) bool {
	if cmd != nil && (cmd.IsSet("rpc.address") || cmd.IsSet("rpc.port")) {
		return true
	}
	if stringx.String(os.Getenv("MORPH_RPC_ADDRESS")).Trim() != "" || stringx.String(os.Getenv("MORPH_RPC_PORT")).Trim() != "" {
		return true
	}

	return stringx.String(cfg.RPC.Address).Trim() != constants.DefaultRPCAddress || cfg.RPC.Port != constants.DefaultRPCPort
}

func checkProcess(pid int) error {
	if pid <= 0 {
		return errors.New("runtime pid is required")
	}

	process, err := findProcess(pid)
	if err != nil {
		return fmt.Errorf("runtime pid %d cannot be inspected: %w", pid, err)
	}
	if err := signalProcess(process, syscall.Signal(0)); err != nil {
		return fmt.Errorf("runtime pid %d is not running: %w", pid, err)
	}

	return nil
}

func dialRuntimeEndpoint(ctx context.Context, address string, port int) error {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", stringx.String(address).Trim(), port))
	if err != nil {
		return err
	}

	return conn.Close()
}

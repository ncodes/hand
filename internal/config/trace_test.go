package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
)

func TestLoad_UsesDebugTraceSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t,
		"HAND_TRACE_ENABLED",
		"HAND_TRACE_DISK_ENABLED",
		"HAND_TRACE_DISK_DIR",
		"HAND_TRACE_DATABASE_ENABLED",
		"HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION",
	)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  requests: false
trace:
  enabled: true
  disk:
    enabled: false
    dir: /tmp/explicit-hand-traces
  database:
    enabled: false
    maxEventsPerSession: 123
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.True(t, cfg.Trace.Enabled)
	require.False(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, "/tmp/explicit-hand-traces", cfg.Trace.Disk.Dir)
	require.False(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, 123, cfg.Trace.Database.MaxEventsPerSession)
}

func TestLoad_UsesDebugTraceSettingsFromEnvOverride(t *testing.T) {
	clearEnvKeys(t,
		"HAND_TRACE_ENABLED",
		"HAND_TRACE_DISK_ENABLED",
		"HAND_TRACE_DISK_DIR",
		"HAND_TRACE_DATABASE_ENABLED",
		"HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION",
	)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_TRACE_ENABLED=true
HAND_TRACE_DISK_ENABLED=false
HAND_TRACE_DISK_DIR=/tmp/env-disk-traces
HAND_TRACE_DATABASE_ENABLED=false
HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION=77
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  requests: false
trace:
  enabled: false
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.True(t, cfg.Trace.Enabled)
	require.False(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, "/tmp/env-disk-traces", cfg.Trace.Disk.Dir)
	require.False(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, 77, cfg.Trace.Database.MaxEventsPerSession)
}

func TestConfig_NormalizeDefaultsDebugTraceSinks(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.True(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, datadir.DebugTraceDir(), cfg.Trace.Disk.Dir)
	require.True(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, constants.DefaultTraceMaxEventsPerSession, cfg.Trace.Database.MaxEventsPerSession)
}

func TestConfig_NormalizeDefaultsDebugTraceDiskDirFromActiveProfile(t *testing.T) {
	setProfileHome(t, "/tmp/hand-home")
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "/tmp/hand-home/traces", cfg.Trace.Disk.Dir)
}

func TestConfig_NormalizeKeepsExplicitTraceDiskDir(t *testing.T) {
	cfg := &Config{
		Trace: TraceConfig{
			Disk: TraceDiskConfig{Dir: "/tmp/disk-traces"},
		},
	}

	cfg.Normalize()

	require.Equal(t, "/tmp/disk-traces", cfg.Trace.Disk.Dir)
}

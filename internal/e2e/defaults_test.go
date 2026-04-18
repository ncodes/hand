package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSpec_DefaultPaths(t *testing.T) {
	spec := DefaultSpec("/tmp/hand-home")

	require.NoError(t, spec.Validate())
	assert.Equal(t, EntrypointDirectAgent, spec.PrimaryEntrypoint)
	assert.Equal(t, EntrypointCommandRPC, spec.SecondaryEntrypoint)
	assert.True(t, spec.Config.AllowInMemory)
	assert.Equal(t, "/tmp/hand-home/workspace", spec.Isolation.WorkspaceDir)
	assert.Equal(t, "/tmp/hand-home/data", spec.Isolation.DataDir)
	assert.Equal(t, "/tmp/hand-home/data/state.db", spec.Isolation.StoragePath)
	assert.Equal(t, "/tmp/hand-home/traces", spec.Isolation.TraceDir)
}

func TestDefaultConfig_DefaultsAndOverrides(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := DefaultConfig(ConfigOptions{})
		require.NotNil(t, cfg)
		require.NotNil(t, cfg.Stream)
		assert.Equal(t, "Test Hand", cfg.Name)
		assert.Equal(t, "test-model", cfg.Model)
		assert.Equal(t, "sqlite", cfg.StorageBackend)
		assert.False(t, *cfg.Stream)
	})

	t.Run("overrides", func(t *testing.T) {
		cfg := DefaultConfig(ConfigOptions{
			Name:           "RPC Test",
			StorageBackend: "memory",
			Stream:         true,
		})
		require.NotNil(t, cfg)
		require.NotNil(t, cfg.Stream)
		assert.Equal(t, "RPC Test", cfg.Name)
		assert.Equal(t, "memory", cfg.StorageBackend)
		assert.True(t, *cfg.Stream)
	})

	t.Run("trims and falls back", func(t *testing.T) {
		cfg := DefaultConfig(ConfigOptions{
			Name:           "  ",
			StorageBackend: "  ",
		})
		require.NotNil(t, cfg)
		assert.Equal(t, "Test Hand", cfg.Name)
		assert.Equal(t, "sqlite", cfg.StorageBackend)
	})
}

func TestDefaultSpec_EmptyHomeStillBuildsExpectedLayout(t *testing.T) {
	spec := DefaultSpec("")

	assert.Equal(t, filepath.Join("", "workspace"), spec.Isolation.WorkspaceDir)
	assert.Equal(t, filepath.Join("", "data"), spec.Isolation.DataDir)
	assert.Equal(t, filepath.Join("", "data", "state.db"), spec.Isolation.StoragePath)
	assert.Equal(t, filepath.Join("", "traces"), spec.Isolation.TraceDir)
}

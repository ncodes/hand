package e2e

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
)

// ConfigOptions customizes the default e2e config.
type ConfigOptions struct {
	Name           string
	StorageBackend string
	Stream         bool
}

// DefaultSpec returns the default e2e scenario specification.
func DefaultSpec(home string) HarnessSpec {
	home = strings.TrimSpace(home)
	dataDir := filepath.Join(home, "data")

	return HarnessSpec{
		PrimaryEntrypoint:   EntrypointDirectAgent,
		SecondaryEntrypoint: EntrypointCommandRPC,
		Config:              ConfigInput{AllowInMemory: true},
		Isolation: Isolation{
			WorkspaceDir: filepath.Join(home, "workspace"),
			DataDir:      dataDir,
			StoragePath:  filepath.Join(dataDir, "state.db"),
			TraceDir:     filepath.Join(home, "traces"),
		},
	}
}

// DefaultConfig returns the default e2e harness configuration.
func DefaultConfig(opts ConfigOptions) *config.Config {
	stream := opts.Stream

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "Test Hand"
	}

	storageBackend := strings.TrimSpace(opts.StorageBackend)
	if storageBackend == "" {
		storageBackend = "sqlite"
	}

	return &config.Config{
		Name:    name,
		Models:  config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model", Stream: &stream}},
		Storage: config.StorageConfig{Backend: storageBackend},
		Session: config.SessionConfig{DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
	}
}

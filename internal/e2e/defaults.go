package e2e

import (
	"path/filepath"
	"time"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/pkg/str"
)

// ConfigOptions customizes the default e2e config.
type ConfigOptions struct {
	Name           string
	StorageBackend string
	Stream         bool
}

// DefaultSpec returns the default e2e scenario specification.
func DefaultSpec(home string) HarnessSpec {
	stringValue1 := str.String(home)
	home = stringValue1.Trim()
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
	stringValue2 := str.String(opts.Name)
	name := stringValue2.Trim()
	if name == "" {
		name = "Test Morph"
	}
	stringValue3 := str.String(opts.StorageBackend)
	storageBackend := stringValue3.Trim()
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

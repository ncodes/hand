package e2e

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
)

type ConfigOptions struct {
	Name           string
	StorageBackend string
	Stream         bool
}

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
		Name:                     name,
		Model:                    "test-model",
		Stream:                   &stream,
		StorageBackend:           storageBackend,
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
	}
}

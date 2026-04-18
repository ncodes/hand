package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

type RPCConfigOptions struct {
	Name     string
	Stream   bool
	Instruct string
	NoColor  bool
}

func NewDefaultRPCHarness(
	ctx context.Context,
	home string,
	client models.Client,
	cfg *config.Config,
) (*RPCHarness, error) {
	if cfg == nil {
		cfg = DefaultConfig(ConfigOptions{StorageBackend: "sqlite"})
	}

	return NewRPCHarness(ctx, HarnessOptions{
		Spec:        DefaultSpec(home),
		Config:      cfg,
		ModelClient: client,
	})
}

func WriteRPCConfigFile(dir, address string, port int, opts RPCConfigOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "yaml-agent"
	}

	content := fmt.Sprintf(
		`name: %s
model:
  verifyModel: false
  stream: %t
rpc:
  address: %s
  port: %d
log:
  noColor: %t
`,
		name,
		opts.Stream,
		strings.TrimSpace(address),
		port,
		opts.NoColor,
	)
	if strings.TrimSpace(opts.Instruct) != "" {
		content += "instruct: " + strings.TrimSpace(opts.Instruct) + "\n"
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}

	return path, nil
}

func MissingTools(names ...string) RequestAssert {
	missing := make([]string, len(names))
	copy(missing, names)
	slices.Sort(missing)

	return func(req models.Request) error {
		available := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			available = append(available, tool.Name)
		}

		for _, name := range missing {
			if slices.Contains(available, name) {
				return fmt.Errorf("expected tool %q to be unavailable, got tools %v", name, available)
			}
		}

		return nil
	}
}

func ToolMessagePresent(expectedID, expectedName string) RequestAssert {
	return func(req models.Request) error {
		for _, message := range req.Messages {
			if message.Role != handmsg.RoleTool {
				continue
			}
			if strings.TrimSpace(message.ToolCallID) != expectedID {
				continue
			}
			if strings.TrimSpace(message.Name) != expectedName {
				return fmt.Errorf("expected tool message name %q", expectedName)
			}
			return nil
		}

		return fmt.Errorf("expected tool message for tool call %q", expectedID)
	}
}

func ToolError(expectedID, expectedName, expectedCode, expectedMessage string) RequestAssert {
	return func(req models.Request) error {
		for _, message := range req.Messages {
			if message.Role != handmsg.RoleTool || strings.TrimSpace(message.ToolCallID) != expectedID {
				continue
			}
			if strings.TrimSpace(message.Name) != expectedName {
				return fmt.Errorf("expected tool message name %q", expectedName)
			}

			var payload struct {
				Name  string `json:"name"`
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
				return err
			}
			if strings.TrimSpace(payload.Name) != expectedName {
				return fmt.Errorf("expected tool payload name %q", expectedName)
			}
			if strings.TrimSpace(payload.Error.Code) != expectedCode {
				return fmt.Errorf("expected tool error code %q, got %q", expectedCode, payload.Error.Code)
			}
			if strings.TrimSpace(payload.Error.Message) != expectedMessage {
				return fmt.Errorf("expected tool error message %q, got %q", expectedMessage, payload.Error.Message)
			}

			return nil
		}

		return fmt.Errorf("expected tool error for tool call %q", expectedID)
	}
}

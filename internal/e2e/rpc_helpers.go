package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

var rpcHelperListen = net.Listen
var rpcHelperNewClient = rpcclient.NewClient

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

func ReserveRPCPort() (int, error) {
	lis, err := rpcHelperListen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer lis.Close()

	tcpAddr, ok := lis.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("rpc helper listener must be tcp")
	}

	return tcpAddr.Port, nil
}

func WaitForRPC(address string, port int, timeout time.Duration) (*rpcclient.Client, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := rpcHelperNewClient(context.Background(), rpcclient.Options{
			Address: strings.TrimSpace(address),
			Port:    port,
		})
		if err == nil {
			_, currentErr := client.CurrentSession(context.Background())
			if currentErr == nil {
				return client, nil
			}
			_ = client.Close()
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("rpc server did not become ready on %s:%d", strings.TrimSpace(address), port)
}

func WriteRPCConfigFile(dir, address string, port int, opts RPCConfigOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "yaml-agent"
	}

	content := fmt.Sprintf(
		`name: %s
models:
  verify: false
  main:
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
		content += "session:\n  instruct: " + strings.TrimSpace(opts.Instruct) + "\n"
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

func CombineChecks(checks ...RequestAssert) RequestAssert {
	return func(req models.Request) error {
		for _, check := range checks {
			if check == nil {
				continue
			}
			if err := check(req); err != nil {
				return err
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

func ToolOutputString(expectedID, expectedName string, check func(string) error) RequestAssert {
	return func(req models.Request) error {
		output, err := findToolEnvelopeOutput(req, expectedID, expectedName)
		if err != nil {
			return err
		}
		return check(output)
	}
}

func ToolOutputJSON(expectedID, expectedName string, check func(map[string]any) error) RequestAssert {
	return func(req models.Request) error {
		output, err := findToolEnvelopeOutput(req, expectedID, expectedName)
		if err != nil {
			return err
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			return err
		}

		return check(payload)
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

func findToolEnvelopeOutput(req models.Request, expectedID, expectedName string) (string, error) {
	for _, message := range req.Messages {
		if message.Role != handmsg.RoleTool || strings.TrimSpace(message.ToolCallID) != expectedID {
			continue
		}
		if strings.TrimSpace(message.Name) != expectedName {
			return "", fmt.Errorf("expected tool message name %q", expectedName)
		}

		var envelope struct {
			Name   string `json:"name"`
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(message.Content), &envelope); err != nil {
			return "", err
		}
		if strings.TrimSpace(envelope.Name) != expectedName {
			return "", fmt.Errorf("expected tool payload name %q", expectedName)
		}

		return strings.TrimSpace(envelope.Output), nil
	}

	return "", fmt.Errorf("expected tool output for tool call %q", expectedID)
}

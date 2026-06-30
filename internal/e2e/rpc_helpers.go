package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/wandxy/morph/internal/config"
	models "github.com/wandxy/morph/internal/model"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/stringx"
)

var rpcHelperListen = net.Listen
var rpcHelperNewClient = rpcclient.NewClient

// RPCConfigOptions controls rpc config.
type RPCConfigOptions struct {
	Name     string
	Stream   bool
	Instruct string
	NoColor  bool
}

// NewDefaultRPCHarness returns an RPC harness with default test dependencies.
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

// ReserveRPCPort reserves an available localhost port for an RPC test server.
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

// WaitForRPC waits until an RPC endpoint accepts connections.
func WaitForRPC(address string, port int, timeout time.Duration) (*rpcclient.Client, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := rpcHelperNewClient(context.Background(), rpcclient.Options{
			Address: stringx.String(address).Trim(),
			Port:    port,
		})
		if err == nil {
			_, currentErr := client.Session.Current(context.Background())
			if currentErr == nil {
				return client, nil
			}
			_ = client.Close()
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("rpc server did not become ready on %s:%d", stringx.String(address).Trim(), port)
}

// WriteRPCConfigFile writes a temporary RPC config file for tests.
func WriteRPCConfigFile(dir, address string, port int, opts RPCConfigOptions) (string, error) {
	name := stringx.String(opts.Name).Trim()
	if name == "" {
		name = "yaml-agent"
	}

	content := fmt.Sprintf(
		`name: %s
models:
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
		stringx.String(address).Trim(),
		port,
		opts.NoColor,
	)
	if stringx.String(opts.Instruct).Trim() != "" {
		content += "session:\n  instruct: " + stringx.String(opts.Instruct).Trim() + "\n"
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}

	return path, nil
}

// MissingTools returns the expected missing-tool names from err.
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

// CombineChecks joins multiple e2e assertions into one check.
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

// ToolMessagePresent checks that a tool message for name appears in the session.
func ToolMessagePresent(expectedID, expectedName string) RequestAssert {
	return func(req models.Request) error {
		for _, message := range req.Messages {
			if message.Role != morphmsg.RoleTool {
				continue
			}
			if stringx.String(message.ToolCallID).Trim() != expectedID {
				continue
			}
			if stringx.String(message.Name).Trim() != expectedName {
				return fmt.Errorf("expected tool message name %q", expectedName)
			}
			return nil
		}

		return fmt.Errorf("expected tool message for tool call %q", expectedID)
	}
}

// ToolOutputString returns a string field from a recorded tool output.
func ToolOutputString(expectedID, expectedName string, check func(string) error) RequestAssert {
	return func(req models.Request) error {
		output, err := getToolEnvelopeOutput(req, expectedID, expectedName)
		if err != nil {
			return err
		}
		return check(output)
	}
}

// ToolOutputJSON decodes recorded tool output into target.
func ToolOutputJSON(expectedID, expectedName string, check func(map[string]any) error) RequestAssert {
	return func(req models.Request) error {
		output, err := getToolEnvelopeOutput(req, expectedID, expectedName)
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

// ToolError returns the error text from a recorded tool output.
func ToolError(expectedID, expectedName, expectedCode, expectedMessage string) RequestAssert {
	return func(req models.Request) error {
		for _, message := range req.Messages {
			if message.Role != morphmsg.RoleTool || stringx.String(message.ToolCallID).Trim() != expectedID {
				continue
			}
			if stringx.String(message.Name).Trim() != expectedName {
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
			if stringx.String(payload.Name).Trim() != expectedName {
				return fmt.Errorf("expected tool payload name %q", expectedName)
			}
			if stringx.String(payload.Error.Code).Trim() != expectedCode {
				return fmt.Errorf("expected tool error code %q, got %q", expectedCode, payload.Error.Code)
			}
			if stringx.String(payload.Error.Message).Trim() != expectedMessage {
				return fmt.Errorf("expected tool error message %q, got %q", expectedMessage, payload.Error.Message)
			}

			return nil
		}

		return fmt.Errorf("expected tool error for tool call %q", expectedID)
	}
}

func getToolEnvelopeOutput(req models.Request, expectedID, expectedName string) (string, error) {
	for _, message := range req.Messages {
		if message.Role != morphmsg.RoleTool || stringx.String(message.ToolCallID).Trim() != expectedID {
			continue
		}
		if stringx.String(message.Name).Trim() != expectedName {
			return "", fmt.Errorf("expected tool message name %q", expectedName)
		}

		var envelope struct {
			Name   string `json:"name"`
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(message.Content), &envelope); err != nil {
			return "", err
		}
		if stringx.String(envelope.Name).Trim() != expectedName {
			return "", fmt.Errorf("expected tool payload name %q", expectedName)
		}

		return stringx.String(envelope.Output).Trim(), nil
	}

	return "", fmt.Errorf("expected tool output for tool call %q", expectedID)
}

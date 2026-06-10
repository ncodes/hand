package gateway

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/gateway/pairing"
)

var errGatewayTestWrite = errors.New("write failed")

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errGatewayTestWrite
}

type failAfterWrite struct {
	writes int
}

func (w *failAfterWrite) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > 1 {
		return 0, errGatewayTestWrite
	}

	return len(p), nil
}

func TestSetOutputReturnsPreviousAndDiscardsNil(t *testing.T) {
	originalOutput := gatewayOutput
	t.Cleanup(func() { gatewayOutput = originalOutput })
	var output bytes.Buffer

	previous := SetOutput(&output)
	require.Same(t, originalOutput, previous)
	previous = SetOutput(nil)
	require.Same(t, &output, previous)

	_, err := gatewayOutput.Write([]byte("discarded"))
	require.NoError(t, err)
	require.Empty(t, output.String())
}

func TestGatewayCommandShowsHelpWithoutSubcommand(t *testing.T) {
	var output bytes.Buffer
	cmd := NewCommand()
	cmd.Writer = &output

	err := cmd.Run(context.Background(), []string{"gateway"})

	require.NoError(t, err)
	require.Contains(t, output.String(), "Manage external gateway integrations")
}

func TestRuntimeCommandsCallRPCAndPrintStatus(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})

	var output bytes.Buffer
	gatewayOutput = &output
	stub := &agentstub.AgentServiceStub{
		GatewayStatusResult: rpcclient.GatewayStatus{
			State:        "running",
			Address:      "127.0.0.1",
			Port:         50052,
			TelegramMode: "polling",
			SlackMode:    "socket",
		},
	}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return stub, nil
	}

	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "status"}))
	require.Equal(t, "state=running address=127.0.0.1 port=50052 telegram=polling slack=socket\n", output.String())

	output.Reset()
	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "start"}))
	require.True(t, stub.GatewayStarted)
	require.Equal(t, "state=running address=127.0.0.1 port=50052 telegram=polling slack=socket\n", output.String())

	output.Reset()
	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "stop"}))
	require.True(t, stub.GatewayStopped)
	require.Equal(t, "state=running address=127.0.0.1 port=50052 telegram=polling slack=socket\n", output.String())

	output.Reset()
	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "restart"}))
	require.True(t, stub.GatewayRestarted)
	require.Equal(t, "state=running address=127.0.0.1 port=50052 telegram=polling slack=socket\n", output.String())
}

func TestRuntimeCommandPrintsSafeLastError(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})

	var output bytes.Buffer
	gatewayOutput = &output
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{
			GatewayStatusResult: rpcclient.GatewayStatus{
				State:        "failed",
				Address:      "127.0.0.1",
				Port:         50052,
				TelegramMode: "polling",
				SlackMode:    "socket",
				LastError:    "slack socket: [REDACTED]",
			},
		}, nil
	}

	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "status"}))
	require.Contains(t, output.String(), `last_error="slack socket: [REDACTED]"`)
}

func TestRuntimeCommandReturnsWriteError(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})
	gatewayOutput = errWriter{}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{
			GatewayStatusResult: rpcclient.GatewayStatus{State: "running"},
		}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "status"})
	require.ErrorIs(t, err, errGatewayTestWrite)

	gatewayOutput = &failAfterWrite{}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{
			GatewayStatusResult: rpcclient.GatewayStatus{
				State:     "failed",
				LastError: "safe error",
			},
		}, nil
	}

	err = NewCommand().Run(context.Background(), []string{"gateway", "status"})
	require.ErrorIs(t, err, errGatewayTestWrite)
}

func TestRuntimeCommandsReturnClientAndRPCErrors(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	t.Cleanup(func() { newClient = originalNewClient })
	clientErr := errors.New("client unavailable")
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return nil, clientErr
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "status"})
	require.ErrorIs(t, err, clientErr)

	rpcErr := errors.New("gateway rpc failed")
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{GatewayStatusErr: rpcErr}, nil
	}

	err = NewCommand().Run(context.Background(), []string{"gateway", "status"})
	require.ErrorIs(t, err, rpcErr)

	err = NewCommand().Run(context.Background(), []string{"gateway", "start"})
	require.ErrorIs(t, err, rpcErr)

	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return nil, clientErr
	}

	err = NewCommand().Run(context.Background(), []string{"gateway", "start"})
	require.ErrorIs(t, err, clientErr)
}

func TestPairingListCommandCallsRPC(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})

	var output bytes.Buffer
	gatewayOutput = &output
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	stub := &agentstub.AgentServiceStub{
		PairingRequests: []pairing.PendingRequest{{
			Source:      "telegram",
			SenderID:    "123",
			DisplayName: "Ada",
			ExpiresAt:   now,
		}},
		PairedSenders: []pairing.ApprovedSender{{
			Source:      "telegram",
			SenderID:    "456",
			DisplayName: "Grace",
		}},
	}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "list", "telegram"})

	require.NoError(t, err)
	require.Contains(t, output.String(), "pending\n")
	require.Contains(t, output.String(), "  source    sender id  name  expires\n")
	require.Contains(t, output.String(), "  telegram  123        Ada   2026-06-08T12:00:00Z\n")
	require.Contains(t, output.String(), "approved\n")
	require.Contains(t, output.String(), "  source    sender id  name\n")
	require.Contains(t, output.String(), "  telegram  456        Grace\n")
}

func TestPairingListCommandShowsNoneForEmptySections(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})

	var output bytes.Buffer
	gatewayOutput = &output
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "list"})

	require.NoError(t, err)
	require.Equal(t, "pending\n  none\n\napproved\n  none\n", output.String())
}

func TestPairingListCommandReturnsWriteError(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})
	gatewayOutput = errWriter{}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "list"})

	require.ErrorIs(t, err, errGatewayTestWrite)
}

func TestPairingApproveRevokeAndClearCommandsCallRPC(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})

	var output bytes.Buffer
	gatewayOutput = &output
	stub := &agentstub.AgentServiceStub{
		ApprovedPairing: rpcclient.GatewayPairedSender{Source: "telegram", SenderID: "123"},
		PairingApproved: true,
	}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return stub, nil
	}

	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "pairing", "approve", "telegram", "12345678"}))
	require.Equal(t, "telegram", stub.PairingSource)
	require.Equal(t, "12345678", stub.PairingCode)
	require.Contains(t, output.String(), "approved telegram 123\n")

	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "pairing", "revoke", "telegram", "123"}))
	require.Equal(t, "telegram", stub.RevokedPairingSource)
	require.Equal(t, "123", stub.RevokedPairingSender)

	require.NoError(t, NewCommand().Run(context.Background(), []string{"gateway", "pairing", "clear-pending", "telegram"}))
	require.Equal(t, "telegram", stub.ClearedPairingSource)
}

func TestPairingApproveRevokeAndClearCommandsReturnWriteErrors(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	originalOutput := gatewayOutput
	t.Cleanup(func() {
		newClient = originalNewClient
		gatewayOutput = originalOutput
	})
	gatewayOutput = errWriter{}
	stub := &agentstub.AgentServiceStub{
		ApprovedPairing: rpcclient.GatewayPairedSender{Source: "telegram", SenderID: "123"},
		PairingApproved: true,
	}
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return stub, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "approve", "telegram", "12345678"})
	require.ErrorIs(t, err, errGatewayTestWrite)

	err = NewCommand().Run(context.Background(), []string{"gateway", "pairing", "revoke", "telegram", "123"})
	require.ErrorIs(t, err, errGatewayTestWrite)

	err = NewCommand().Run(context.Background(), []string{"gateway", "pairing", "clear-pending"})
	require.ErrorIs(t, err, errGatewayTestWrite)
}

func TestPairingCommandsRejectMissingRequiredArgs(t *testing.T) {
	originalNewClient := newClient
	t.Cleanup(func() { newClient = originalNewClient })
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		t.Fatal("gateway command should reject missing args before opening RPC client")
		return nil, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "approve", "telegram"})
	require.EqualError(t, err, "source and code are required")

	err = NewCommand().Run(context.Background(), []string{"gateway", "pairing", "revoke", "telegram"})
	require.EqualError(t, err, "source and sender id are required")
}

func TestPairingApproveCommandRejectsMissingMatch(t *testing.T) {
	setGatewayTestProfile(t)
	originalNewClient := newClient
	t.Cleanup(func() { newClient = originalNewClient })
	newClient = func(context.Context, *config.Config) (gatewayClient, error) {
		return &agentstub.AgentServiceStub{}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"gateway", "pairing", "approve", "telegram", "12345678"})

	require.EqualError(t, err, "no pending gateway pairing matched code")
}

func setGatewayTestProfile(t *testing.T) {
	t.Helper()
	original := profile.Active()
	t.Cleanup(func() { profile.SetActive(original) })
	profile.SetActive(profile.Profile{
		Name:        "test",
		HomeDir:     t.TempDir(),
		RuntimePath: t.TempDir() + "/runtime.json",
	})
}

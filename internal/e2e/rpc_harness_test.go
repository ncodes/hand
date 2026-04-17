package e2e

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
	"google.golang.org/grpc"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewRPCHarness_RealClientChatSmoke(t *testing.T) {
	h, err := NewRPCHarness(context.Background(), HarnessOptions{
		Spec:        testHarnessSpec(t),
		Config:      testHarnessConfig(),
		ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "hello over rpc"}}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	client, err := h.Client(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	reply, err := client.Respond(context.Background(), "hello", rpcclient.RespondOptions{})
	require.NoError(t, err)
	assert.Equal(t, "hello over rpc", reply)
}

func TestNewRPCHarness_ErrorsAndHelpers(t *testing.T) {
	t.Run("base harness error", func(t *testing.T) {
		_, err := NewRPCHarness(context.Background(), HarnessOptions{})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e entrypoint is required")
	})

	t.Run("listen error", func(t *testing.T) {
		original := rpcListen
		rpcListen = func(string, string) (net.Listener, error) {
			return nil, errors.New("listen failed")
		}
		t.Cleanup(func() {
			rpcListen = original
		})

		_, err := NewRPCHarness(context.Background(), HarnessOptions{
			Spec:        testHarnessSpec(t),
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.Error(t, err)
		assert.EqualError(t, err, "listen failed")
	})

	t.Run("base harness without full service api", func(t *testing.T) {
		originalBase := newBaseHarness
		originalListen := rpcListen
		newBaseHarness = func(context.Context, HarnessOptions) (*Harness, error) {
			return &Harness{
				agent:      harnessAgentStub{reply: "ok"},
				restoreEnv: func() {},
			}, nil
		}
		rpcListen = func(string, string) (net.Listener, error) {
			return net.Listen("tcp", "127.0.0.1:0")
		}
		t.Cleanup(func() {
			newBaseHarness = originalBase
			rpcListen = originalListen
		})

		_, err := NewRPCHarness(context.Background(), HarnessOptions{
			Spec:        testHarnessSpec(t),
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e rpc harness requires a full agent service")
	})

	t.Run("non tcp listener", func(t *testing.T) {
		originalListen := rpcListen
		originalServe := grpcServe
		rpcListen = func(string, string) (net.Listener, error) {
			return stubListener{addr: stubAddr("pipe")}, nil
		}
		grpcServe = func(*grpc.Server, net.Listener) error { return nil }
		t.Cleanup(func() {
			rpcListen = originalListen
			grpcServe = originalServe
		})

		_, err := NewRPCHarness(context.Background(), HarnessOptions{
			Spec:        testHarnessSpec(t),
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e rpc listener must be tcp")
	})

	t.Run("client nil harness", func(t *testing.T) {
		client, err := (*RPCHarness)(nil).Client(context.Background())
		require.Error(t, err)
		assert.Nil(t, client)
		assert.EqualError(t, err, "e2e rpc harness is required")
	})

	t.Run("close nil harness", func(t *testing.T) {
		assert.NoError(t, (*RPCHarness)(nil).Close())
		assert.Empty(t, (*RPCHarness)(nil).Address())
		assert.Zero(t, (*RPCHarness)(nil).Port())
		assert.Empty(t, (*RPCHarness)(nil).ConfigFileContents())
	})

	t.Run("close returns serve error", func(t *testing.T) {
		originalServe := grpcServe
		grpcServe = func(*grpc.Server, net.Listener) error { return errors.New("serve failed") }
		t.Cleanup(func() {
			grpcServe = originalServe
		})

		h, err := NewRPCHarness(context.Background(), HarnessOptions{
			Spec:        testHarnessSpec(t),
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)
		err = h.Close()
		require.Error(t, err)
		assert.EqualError(t, err, "serve failed")
	})

	t.Run("close ignores server stopped", func(t *testing.T) {
		originalServe := grpcServe
		grpcServe = func(*grpc.Server, net.Listener) error { return grpc.ErrServerStopped }
		t.Cleanup(func() {
			grpcServe = originalServe
		})

		h, err := NewRPCHarness(context.Background(), HarnessOptions{
			Spec:        testHarnessSpec(t),
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, h.Close())
	})

	t.Run("config file contents", func(t *testing.T) {
		h := &RPCHarness{address: "127.0.0.1", port: -1234}
		assert.Contains(t, h.ConfigFileContents(), "verifyModel: false")
		assert.Contains(t, h.ConfigFileContents(), "address: 127.0.0.1")
		assert.Contains(t, h.ConfigFileContents(), "port: -1234")
		assert.Equal(t, "127.0.0.1", h.Address())
		assert.Equal(t, -1234, h.Port())
	})
}

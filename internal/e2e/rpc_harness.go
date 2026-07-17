package e2e

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"

	morphagent "github.com/wandxy/morph/internal/agent"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/pkg/str"
	"google.golang.org/grpc"

	"github.com/wandxy/morph/internal/rpc/server"
)

var rpcListen = net.Listen
var newBaseHarness = NewHarness
var grpcServe = func(srv *grpc.Server, lis net.Listener) error {
	return srv.Serve(lis)
}

// RPCHarness drives rpc e2e scenarios.
type RPCHarness struct {
	*Harness
	address string
	port    int
	server  *grpc.Server
	errMu   sync.Mutex
	err     error
}

// NewRPCHarness returns an RPC-backed e2e harness.
func NewRPCHarness(ctx context.Context, opts HarnessOptions) (*RPCHarness, error) {
	base, err := newBaseHarness(ctx, opts)
	if err != nil {
		return nil, err
	}

	lis, err := rpcListen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = base.Close()
		return nil, err
	}

	tcpAddr, ok := lis.Addr().(*net.TCPAddr)
	if !ok {
		_ = lis.Close()
		_ = base.Close()
		return nil, errors.New("e2e rpc listener must be tcp")
	}

	serviceAPI, ok := base.agent.(morphagent.ServiceAPI)
	if !ok {
		_ = lis.Close()
		_ = base.Close()
		return nil, errors.New("e2e rpc harness requires a full agent service")
	}
	grpcServer := server.New(serviceAPI, server.Options{
		Health:           true,
		PermissionPolicy: base.cfg.Permissions,
	})

	h := &RPCHarness{
		Harness: base,
		address: tcpAddr.IP.String(),
		port:    tcpAddr.Port,
		server:  grpcServer,
	}

	go func() {
		if serveErr := grpcServe(grpcServer, lis); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
			h.errMu.Lock()
			h.err = serveErr
			h.errMu.Unlock()
		}
	}()

	return h, nil
}

func (h *RPCHarness) Address() string {
	if h == nil {
		return ""
	}
	return h.address
}

func (h *RPCHarness) Port() int {
	if h == nil {
		return 0
	}
	return h.port
}

func (h *RPCHarness) Client(ctx context.Context) (*rpcclient.Client, error) {
	if h == nil {
		return nil, errors.New("e2e rpc harness is required")
	}
	return rpcclient.NewClient(normalizeHarnessContext(ctx), rpcclient.Options{
		Address: h.address,
		Port:    h.port,
	})
}

func (h *RPCHarness) Close() error {
	if h == nil {
		return nil
	}
	if h.server != nil {
		h.server.Stop()
	}
	if h.Harness != nil {
		_ = h.Harness.Close()
	}

	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.err
}

func (h *RPCHarness) ConfigFileContents() string {
	if h == nil {
		return ""
	}
	addressValue := str.String(h.address)
	return "rpc:\n" +
		"  address: " + addressValue.Trim() + "\n" +
		"  port: " + strconv.Itoa(h.port) + "\n"
}

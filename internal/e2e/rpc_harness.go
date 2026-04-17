package e2e

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/wandxy/hand/internal/agent"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/rpcserver"
	"google.golang.org/grpc"
)

var rpcListen = net.Listen
var newBaseHarness = NewHarness
var grpcServe = func(srv *grpc.Server, lis net.Listener) error {
	return srv.Serve(lis)
}

type RPCHarness struct {
	*Harness
	address string
	port    int
	server  *grpc.Server
	errMu   sync.Mutex
	err     error
}

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

	serviceAPI, ok := base.agent.(agent.ServiceAPI)
	if !ok {
		_ = lis.Close()
		_ = base.Close()
		return nil, errors.New("e2e rpc harness requires a full agent service")
	}
	server := rpcserver.New(serviceAPI, rpcserver.Options{Health: true})

	h := &RPCHarness{
		Harness: base,
		address: tcpAddr.IP.String(),
		port:    tcpAddr.Port,
		server:  server,
	}

	go func() {
		if serveErr := grpcServe(server, lis); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
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
	return "model:\n" +
		"  verifyModel: false\n" +
		"rpc:\n" +
		"  address: " + strings.TrimSpace(h.address) + "\n" +
		"  port: " + strconv.Itoa(h.port) + "\n"
}

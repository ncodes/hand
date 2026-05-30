package server

import (
	handagent "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/rpc"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Options configures the RPC server.
type Options struct {
	Health bool
}

// New returns a gRPC server registered with the Hand RPC services.
func New(service handagent.ServiceAPI, opts Options) *grpc.Server {
	server := grpc.NewServer()
	rpcService := rpc.NewService(service)
	handpb.RegisterHandServiceServer(server, rpcService)
	handpb.RegisterSessionServiceServer(server, rpcService)

	if opts.Health {
		healthcheck := health.NewServer()
		healthpb.RegisterHealthServer(server, healthcheck)
		healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	return server
}

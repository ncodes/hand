package server

import (
	morphagent "github.com/wandxy/morph/internal/agent"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/rpc"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Options configures the RPC server.
type Options struct {
	RuntimeModel         rpc.ModelRuntime
	Health               bool
	GatewayPairingSecret string
	GatewayConfig        config.GatewayConfig
	GatewayRuntime       rpc.GatewayRuntime
}

// New returns a gRPC server registered with the Morph RPC services.
func New(service morphagent.ServiceAPI, opts Options) *grpc.Server {
	server := grpc.NewServer()
	rpcService := rpc.NewServiceWithOptions(service, rpc.ServiceOptions{
		RuntimeModel:         opts.RuntimeModel,
		GatewayPairingSecret: opts.GatewayPairingSecret,
		GatewayConfig:        opts.GatewayConfig,
		GatewayRuntime:       opts.GatewayRuntime,
	})
	morphpb.RegisterMorphServiceServer(server, rpcService)
	morphpb.RegisterSessionServiceServer(server, rpcService)
	morphpb.RegisterModelServiceServer(server, rpcService)
	morphpb.RegisterGatewayServiceServer(server, rpcService)

	if opts.Health {
		healthcheck := health.NewServer()
		healthpb.RegisterHealthServer(server, healthcheck)
		healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	return server
}

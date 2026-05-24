package server

import (
	"github.com/wandxy/hand/internal/host"
	"github.com/wandxy/hand/internal/rpc"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Options struct {
	Health bool
}

func New(service host.ServiceAPI, opts Options) *grpc.Server {
	server := grpc.NewServer()
	handpb.RegisterHandServiceServer(server, rpc.NewService(service))

	if opts.Health {
		healthcheck := health.NewServer()
		healthpb.RegisterHealthServer(server, healthcheck)
		healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	return server
}

package server

import (
	morphagent "github.com/wandxy/morph/internal/agent"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/rpc"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	"github.com/wandxy/morph/internal/rpc/rpcauth"
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
	Automation           rpc.AutomationAPI
	Browser              rpc.BrowserAPI
	BrowserConfig        config.BrowserConfig
	BrowserCapability    bool
	ProfileName          string
	PermissionPolicy     permissions.Policy
	OwnerCredential      []byte
	OwnerPrincipal       string
}

// New returns a gRPC server registered with the Morph RPC services.
func New(service morphagent.ServiceAPI, opts Options) *grpc.Server {
	validator := rpcauth.NewValidator(opts.OwnerCredential)
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(ownerUnaryServerInterceptor(validator, opts.OwnerPrincipal)),
		grpc.ChainStreamInterceptor(ownerStreamServerInterceptor(validator, opts.OwnerPrincipal)),
	)
	rpcService := rpc.NewServiceWithOptions(service, rpc.ServiceOptions{
		RuntimeModel:         opts.RuntimeModel,
		GatewayPairingSecret: opts.GatewayPairingSecret,
		GatewayConfig:        opts.GatewayConfig,
		GatewayRuntime:       opts.GatewayRuntime,
		Automation:           opts.Automation,
		Browser:              opts.Browser,
		BrowserConfig:        opts.BrowserConfig,
		BrowserCapability:    opts.BrowserCapability,
		ProfileName:          opts.ProfileName,
		PermissionPolicy:     opts.PermissionPolicy,
	})
	morphpb.RegisterMorphServiceServer(server, rpcService)
	morphpb.RegisterSessionServiceServer(server, rpcService)
	morphpb.RegisterModelServiceServer(server, rpcService)
	morphpb.RegisterGatewayServiceServer(server, rpc.NewGatewayService(rpcService))
	morphpb.RegisterAutomationServiceServer(server, rpc.NewAutomationService(rpcService))
	morphpb.RegisterPermissionServiceServer(server, rpc.NewPermissionService(rpcService))
	morphpb.RegisterBrowserServiceServer(server, rpc.NewBrowserService(rpcService))

	if opts.Health {
		healthcheck := health.NewServer()
		healthpb.RegisterHealthServer(server, healthcheck)
		healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	return server
}

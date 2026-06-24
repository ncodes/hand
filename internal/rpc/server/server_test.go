package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentstub "github.com/wandxy/morph/internal/mocks/agentstub"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func TestNew_RegistersMorphServiceWithoutHealth(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, morphpb.MorphService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, morphpb.SessionService_ServiceDesc.ServiceName)
	require.NotContains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

func TestNew_RegistersHealthWhenEnabled(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{Health: true})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, morphpb.MorphService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, morphpb.SessionService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

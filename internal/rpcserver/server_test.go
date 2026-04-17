package rpcserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func TestNew_RegistersHandServiceWithoutHealth(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, handpb.HandService_ServiceDesc.ServiceName)
	require.NotContains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

func TestNew_RegistersHealthWhenEnabled(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{Health: true})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, handpb.HandService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

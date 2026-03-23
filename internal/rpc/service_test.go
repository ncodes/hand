package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

func TestNewService_ReturnsService(t *testing.T) {
	require.NotNil(t, NewService())
}

func TestService_EchoReturnsMessage(t *testing.T) {
	svc := NewService()

	resp, err := svc.Echo(context.Background(), &handpb.EchoRequest{Message: "hello"})

	require.NoError(t, err)
	require.Equal(t, "hello", resp.Message)
}

func TestService_EchoHandlesNilRequest(t *testing.T) {
	svc := NewService()

	resp, err := svc.Echo(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, resp.Message)
}

func TestService_EchoHandlesNilReceiver(t *testing.T) {
	var svc *Service

	resp, err := svc.Echo(context.Background(), &handpb.EchoRequest{Message: "hello"})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, resp.Message)
}

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService_AgentImplementsServiceAPI(t *testing.T) {
	require.Implements(t, (*ServiceAPI)(nil), (*Agent)(nil))
}

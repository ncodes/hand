package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/mocks"
)

func TestMemorySource_ReturnsEnvironmentMemoryProvider(t *testing.T) {
	provider := &memoryProviderStub{name: "memory"}

	require.Same(t, provider, NewMemorySource(&mocks.EnvironmentStub{Memory: provider}).MemoryProvider())
	require.Nil(t, NewMemorySource(nil).MemoryProvider())
}

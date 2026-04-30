package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewProvider_ReturnsConfiguredProvider(t *testing.T) {
	provider, err := NewProvider("", Options{})
	require.NoError(t, err)
	require.IsType(t, &NoopProvider{}, provider)

	provider, err = NewProvider(" memory ", Options{})
	require.NoError(t, err)
	require.IsType(t, &InMemoryProvider{}, provider)
}

func TestNewProvider_ReturnsUnknownProviderError(t *testing.T) {
	provider, err := NewProvider("other", Options{})
	require.ErrorIs(t, err, ErrUnknownProvider)
	require.Nil(t, provider)
}

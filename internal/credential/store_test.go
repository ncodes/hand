package credential

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionProviderRegistryReturnsRegisteredProvider(t *testing.T) {
	provider := &refreshProvider{}

	RegisterSubscriptionProvider("Test-Provider", provider)

	got, ok := GetSubscriptionProvider("test-provider")
	require.True(t, ok)
	require.Same(t, provider, got)
}

func TestSubscriptionProviderRegistryRejectsEmptyInput(t *testing.T) {
	RegisterSubscriptionProvider("", &refreshProvider{})
	RegisterSubscriptionProvider("missing-provider", nil)

	_, ok := GetSubscriptionProvider("")
	require.False(t, ok)
	_, ok = GetSubscriptionProvider("missing-provider")
	require.False(t, ok)
}

func TestIsExpired(t *testing.T) {
	require.False(t, IsExpired(StoredCredential{}))

	future := time.Now().Add(time.Hour)
	require.False(t, IsExpired(StoredCredential{ExpiresAt: &future}))

	past := time.Now().Add(-time.Hour)
	require.True(t, IsExpired(StoredCredential{ExpiresAt: &past}))
}

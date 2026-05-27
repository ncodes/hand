package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	appcredential "github.com/wandxy/hand/internal/credential"
)

func TestConfig_WebAPIKeyEffective_UsesConfigBeforeStoredAndEnvironment(t *testing.T) {
	t.Setenv("EXA_API_KEY", "env-key")
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{
			Type: appcredential.TypeAPIKey,
			Key:  "stored-key",
		}, nil
	})

	value, err := (&Config{Web: WebConfig{
		Provider: constants.WebProviderExa,
		APIKey:   " config-key ",
	}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Equal(t, "config-key", value)
}

func TestConfig_WebAPIKeyEffective_UsesStoredKeyBeforeEnvironment(t *testing.T) {
	t.Setenv("EXA_API_KEY", "env-key")
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{
			Type: appcredential.TypeAPIKey,
			Key:  " stored-key ",
		}, nil
	})

	value, err := (&Config{Web: WebConfig{Provider: constants.WebProviderExa}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Equal(t, "stored-key", value)
}

func TestConfig_WebAPIKeyEffective_UsesProviderEnvironmentBeforeGenericEnvironment(t *testing.T) {
	t.Setenv("HAND_WEB_API_KEY", "generic-key")
	t.Setenv("EXA_API_KEY", "provider-key")
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{}, nil
	})

	value, err := (&Config{Web: WebConfig{Provider: constants.WebProviderExa}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Equal(t, "provider-key", value)
}

func TestConfig_WebAPIKeyEffective_UsesGenericEnvironmentWhenProviderEnvironmentMissing(t *testing.T) {
	t.Setenv("HAND_WEB_API_KEY", "generic-key")
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderTavily, provider)
		return StoredModelCredential{}, nil
	})

	value, err := (&Config{Web: WebConfig{Provider: constants.WebProviderTavily}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Equal(t, "generic-key", value)
}

func TestConfig_WebAPIKeyEffective_IgnoresStoredOAuthAndEmptyStoredAPIKey(t *testing.T) {
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{
			Type:  appcredential.TypeOAuth,
			Token: "oauth-token",
		}, nil
	})

	value, err := (&Config{Web: WebConfig{Provider: constants.WebProviderExa}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Empty(t, value)

	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{Type: appcredential.TypeAPIKey}, nil
	})

	value, err = (&Config{Web: WebConfig{Provider: constants.WebProviderExa}}).WebAPIKeyEffective()

	require.NoError(t, err)
	require.Empty(t, value)
}

func TestConfig_WebAPIKeyEffective_ReturnsStoredCredentialError(t *testing.T) {
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, constants.WebProviderExa, provider)
		return StoredModelCredential{}, errors.New("stored failed")
	})

	_, err := (&Config{Web: WebConfig{Provider: constants.WebProviderExa}}).WebAPIKeyEffective()

	require.EqualError(t, err, "stored failed")
}

func TestConfig_WebCredentialHelpersCoverFallbackBranches(t *testing.T) {
	var cfg *Config

	value, err := cfg.WebAPIKeyEffective()
	require.NoError(t, err)
	require.Empty(t, value)

	value, err = ResolveWebProviderAPIKey("", " config-key ")
	require.NoError(t, err)
	require.Equal(t, "config-key", value)

	value, err = ResolveWebProviderAPIKey(" ", "")
	require.NoError(t, err)
	require.Empty(t, value)

	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, "custom", provider)
		return StoredModelCredential{}, nil
	})
	t.Setenv("HAND_WEB_API_KEY", "generic-key")
	value, err = ResolveWebProviderAPIKey("custom", "")
	require.NoError(t, err)
	require.Equal(t, "generic-key", value)

	require.ElementsMatch(t, []string{
		constants.WebProviderExa,
		constants.WebProviderFirecrawl,
		constants.WebProviderParallel,
		constants.WebProviderTavily,
	}, WebCredentialProviderIDs())
	require.True(t, IsWebCredentialProvider(" EXA "))
	require.False(t, IsWebCredentialProvider("native"))
	require.Equal(t, []string{"HAND_WEB_API_KEY"}, WebProviderAPIKeyEnv("custom"))
	require.Equal(t, []string{"HAND_EXA_API_KEY", "EXA_API_KEY", "HAND_WEB_API_KEY"}, WebProviderAPIKeyEnv(" EXA "))
	require.Equal(
		t,
		[]string{"HAND_FIRECRAWL_API_KEY", "FIRECRAWL_API_KEY", "HAND_WEB_API_KEY"},
		WebProviderAPIKeyEnv(constants.WebProviderFirecrawl),
	)
	require.Equal(
		t,
		[]string{"HAND_PARALLEL_API_KEY", "PARALLEL_API_KEY", "HAND_WEB_API_KEY"},
		WebProviderAPIKeyEnv(constants.WebProviderParallel),
	)
	require.Equal(
		t,
		[]string{"HAND_TAVILY_API_KEY", "TAVILY_API_KEY", "HAND_WEB_API_KEY"},
		WebProviderAPIKeyEnv(constants.WebProviderTavily),
	)

	require.Equal(t, "web-key", GetWebProviderConfigAPIKey(constants.WebProviderExa, &Config{
		Web: WebConfig{Provider: " EXA ", APIKey: " web-key "},
	}))
	require.Empty(t, GetWebProviderConfigAPIKey(constants.WebProviderExa, nil))
	require.Empty(t, GetWebProviderConfigAPIKey("native", &Config{
		Web: WebConfig{Provider: "native", APIKey: "web-key"},
	}))
	require.Empty(t, GetWebProviderConfigAPIKey(constants.WebProviderExa, &Config{
		Web: WebConfig{Provider: constants.WebProviderTavily, APIKey: "web-key"},
	}))
}

package config

import (
	"os"
	"slices"

	"github.com/wandxy/morph/internal/constants"
	appcredential "github.com/wandxy/morph/internal/credential"
	"github.com/wandxy/morph/pkg/str"
)

// WebCredentialSource describes web credential provenance without exposing values.
type WebCredentialSource struct {
	Configured bool
	Source     string
	Name       string
}

// WebAPIKeyEffective resolves the configured web provider API key.
func (c *Config) WebAPIKeyEffective() (string, error) {
	if c == nil {
		return "", nil
	}

	c.normalizeFields()
	return ResolveWebProviderAPIKey(c.Web.Provider, c.Web.APIKey)
}

// WebAPIKeySourceEffective resolves web credential provenance without returning the credential value.
func (c *Config) WebAPIKeySourceEffective() (WebCredentialSource, error) {
	if c == nil {
		return WebCredentialSource{}, nil
	}

	c.normalizeFields()
	return ResolveWebProviderAPIKeySource(c.Web.Provider, c.Web.APIKey)
}

// ResolveWebProviderAPIKey resolves a web provider API key from config, stored, then environment sources.
func ResolveWebProviderAPIKey(provider string, configAPIKey string) (string, error) {
	stringValue1 := str.String(configAPIKey)
	configAPIKey = stringValue1.Trim()
	if configAPIKey != "" {
		return configAPIKey, nil
	}
	stringValue2 := str.String(provider)
	provider = stringValue2.Normalized()
	if provider == "" {
		return "", nil
	}

	if value, ok, err := loadStoredWebProviderAPIKey(provider); ok || err != nil {
		return value, err
	}
	if value, _ := getWebProviderEnvAPIKey(provider); value != "" {
		return value, nil
	}

	return "", nil
}

// ResolveWebProviderAPIKeySource resolves web credential provenance without exposing the credential value.
func ResolveWebProviderAPIKeySource(provider string, configAPIKey string) (WebCredentialSource, error) {
	stringValue3 := str.String(configAPIKey)
	if stringValue3.Trim() != "" {
		return WebCredentialSource{Configured: true, Source: "config"}, nil
	}
	stringValue4 := str.String(provider)
	provider = stringValue4.Normalized()
	if provider == "" {
		return WebCredentialSource{}, nil
	}

	if _, ok, err := loadStoredWebProviderAPIKey(provider); ok || err != nil {
		if err != nil {
			return WebCredentialSource{}, err
		}

		return WebCredentialSource{Configured: true, Source: "stored", Name: provider}, nil
	}
	if _, envName := getWebProviderEnvAPIKey(provider); envName != "" {
		return WebCredentialSource{Configured: true, Source: "environment", Name: envName}, nil
	}

	return WebCredentialSource{}, nil
}

// WebCredentialProviderIDs returns providers that can resolve credentials through web provider config.
func WebCredentialProviderIDs() []string {
	return []string{
		constants.WebProviderExa,
		constants.WebProviderFirecrawl,
		constants.WebProviderParallel,
		constants.WebProviderTavily,
	}
}

// IsWebCredentialProvider reports whether provider is a known web credential provider.
func IsWebCredentialProvider(provider string) bool {
	stringValue5 := str.String(provider)
	provider = stringValue5.Normalized()
	return slices.Contains(WebCredentialProviderIDs(), provider)
}

// WebProviderAPIKeyEnv returns the environment variable names checked for provider.
func WebProviderAPIKeyEnv(provider string) []string {
	stringValue6 := str.String(provider)
	switch stringValue6.Normalized() {
	case constants.WebProviderFirecrawl:
		return []string{"MORPH_FIRECRAWL_API_KEY", "FIRECRAWL_API_KEY", "MORPH_WEB_API_KEY"}
	case constants.WebProviderParallel:
		return []string{"MORPH_PARALLEL_API_KEY", "PARALLEL_API_KEY", "MORPH_WEB_API_KEY"}
	case constants.WebProviderTavily:
		return []string{"MORPH_TAVILY_API_KEY", "TAVILY_API_KEY", "MORPH_WEB_API_KEY"}
	case constants.WebProviderExa:
		return []string{"MORPH_EXA_API_KEY", "EXA_API_KEY", "MORPH_WEB_API_KEY"}
	default:
		return []string{"MORPH_WEB_API_KEY"}
	}
}

func loadStoredWebProviderAPIKey(provider string) (string, bool, error) {
	credential, err := loadStoredProviderToken(provider)
	if err != nil {
		return "", false, err
	}
	stringValue7 := str.String(credential.Type)
	if stringValue7.Normalized() != appcredential.TypeAPIKey {
		return "", false, nil
	}
	stringValue8 := str.String(credential.Key)
	if value := stringValue8.Trim(); value != "" {
		return value, true, nil
	}

	return "", false, nil
}

func getWebProviderEnvAPIKey(provider string) (string, string) {
	return getCredentialFromEnv(WebProviderAPIKeyEnv(provider))
}

// GetWebProviderConfigAPIKey returns the configured web API key when provider is the active web provider.
func GetWebProviderConfigAPIKey(provider string, cfg *Config) string {
	if cfg == nil {
		return ""
	}
	if !IsWebCredentialProvider(provider) {
		return ""
	}
	stringValue9 := str.String(cfg.Web.Provider)
	stringValue10 := str.String(provider)
	if stringValue9.Normalized() != stringValue10.Normalized() {
		return ""
	}
	stringValue11 := str.String(cfg.Web.APIKey)
	return stringValue11.Trim()
}

func getCredentialFromEnv(keys []string) (string, string) {
	for _, key := range keys {
		stringValue12 := str.String(key)
		key = stringValue12.Trim()
		if key == "" {
			continue
		}
		stringValue13 := str.String(os.Getenv(key))
		if value := stringValue13.Trim(); value != "" {
			return value, key
		}
	}
	return "", ""
}

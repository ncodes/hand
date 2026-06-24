package config

import (
	"os"
	"slices"
	"strings"

	"github.com/wandxy/morph/internal/constants"
	appcredential "github.com/wandxy/morph/internal/credential"
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
	configAPIKey = strings.TrimSpace(configAPIKey)
	if configAPIKey != "" {
		return configAPIKey, nil
	}

	provider = strings.TrimSpace(strings.ToLower(provider))
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
	if strings.TrimSpace(configAPIKey) != "" {
		return WebCredentialSource{Configured: true, Source: "config"}, nil
	}

	provider = strings.TrimSpace(strings.ToLower(provider))
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
	provider = strings.TrimSpace(strings.ToLower(provider))
	return slices.Contains(WebCredentialProviderIDs(), provider)
}

// WebProviderAPIKeyEnv returns the environment variable names checked for provider.
func WebProviderAPIKeyEnv(provider string) []string {
	switch strings.TrimSpace(strings.ToLower(provider)) {
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
	if strings.TrimSpace(strings.ToLower(credential.Type)) != appcredential.TypeAPIKey {
		return "", false, nil
	}
	if value := strings.TrimSpace(credential.Key); value != "" {
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
	if strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) != strings.TrimSpace(strings.ToLower(provider)) {
		return ""
	}

	return strings.TrimSpace(cfg.Web.APIKey)
}

func getCredentialFromEnv(keys []string) (string, string) {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, key
		}
	}
	return "", ""
}

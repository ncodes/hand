package config

import (
	"os"
	"slices"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	appcredential "github.com/wandxy/hand/internal/credential"
)

// WebAPIKeyEffective resolves the configured web provider API key.
func (c *Config) WebAPIKeyEffective() (string, error) {
	if c == nil {
		return "", nil
	}

	c.normalizeFields()
	return ResolveWebProviderAPIKey(c.Web.Provider, c.Web.APIKey)
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
		return []string{"HAND_FIRECRAWL_API_KEY", "FIRECRAWL_API_KEY", "HAND_WEB_API_KEY"}
	case constants.WebProviderParallel:
		return []string{"HAND_PARALLEL_API_KEY", "PARALLEL_API_KEY", "HAND_WEB_API_KEY"}
	case constants.WebProviderTavily:
		return []string{"HAND_TAVILY_API_KEY", "TAVILY_API_KEY", "HAND_WEB_API_KEY"}
	case constants.WebProviderExa:
		return []string{"HAND_EXA_API_KEY", "EXA_API_KEY", "HAND_WEB_API_KEY"}
	default:
		return []string{"HAND_WEB_API_KEY"}
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

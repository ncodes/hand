package config

import (
	"fmt"
	"strings"

	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func isValidModelID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	if strings.Count(value, "/") > 1 {
		return false
	}

	segments := strings.SplitSeq(value, "/")
	for segment := range segments {
		if strings.TrimSpace(segment) == "" {
			return false
		}
	}

	return true
}

func applyRegistryModelMetadata(cfg *Config, requestedContextLength int) {
	if cfg == nil {
		return
	}

	model, ok := modelRegistry.GetModel(cfg.Models.Main.Provider, cfg.Models.Main.Name)
	if !ok || model.ContextWindow <= 0 {
		return
	}

	if requestedContextLength <= 0 || requestedContextLength > model.ContextWindow {
		cfg.Models.Main.ContextLength = model.ContextWindow
	}
}

func validateProviderAPI(field string, providerID string, apiID string) error {
	providerID = strings.TrimSpace(strings.ToLower(providerID))
	apiID = strings.TrimSpace(strings.ToLower(apiID))
	if _, ok := modelRegistry.GetAPI(apiID); !ok {
		return fmt.Errorf("%s must be one of: %s", field, getModelAPIList(nil))
	}
	if !modelRegistry.SupportsProviderAPI(providerID, apiID) {
		return fmt.Errorf("%s %q is not supported by provider %q", field, apiID, providerID)
	}

	return nil
}

func validateModelRoleAPI(field string, apiID string, allowedAPIs map[string]struct{}) error {
	apiID = strings.TrimSpace(strings.ToLower(apiID))
	if _, ok := allowedAPIs[apiID]; ok {
		return nil
	}

	return fmt.Errorf("%s must be one of: %s", field, getModelAPIList(allowedAPIs))
}

func validateRegistryModel(
	field string,
	providerID string,
	apiID string,
	modelID string,
	allowedAPIs map[string]struct{},
) error {
	if modelID == "" {
		return nil
	}

	provider, ok := modelRegistry.GetProvider(providerID)
	if !ok {
		return fmt.Errorf("%s provider must be one of: %s", field, getModelProviderList())
	}

	model, known := modelRegistry.GetModel(provider.ID, modelID)
	if !known {
		if provider.RequiresKnownModel || !provider.SupportsModels {
			return fmt.Errorf("%s %q is not registered for provider %q", field, modelID, provider.ID)
		}
		return nil
	}

	if len(allowedAPIs) != 0 {
		if _, ok := allowedAPIs[apiID]; !ok {
			return fmt.Errorf("%s %q is not compatible with this model role", field, modelID)
		}
		if _, ok := allowedAPIs[model.API]; !ok {
			return fmt.Errorf("%s %q is not compatible with this model role", field, modelID)
		}
	}

	return nil
}

func modelGenerationAPIs() map[string]struct{} {
	return map[string]struct{}{
		modelprovider.APIOpenAICompletions: {},
		modelprovider.APIOpenAIResponses:   {},
		modelprovider.APIAnthropicMessages: {},
	}
}

func modelEmbeddingAPIs() map[string]struct{} {
	return map[string]struct{}{
		modelprovider.APIOpenAIEmbeddings: {},
	}
}

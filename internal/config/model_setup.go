package config

import (
	"strings"

	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
)

func ModelSetupEmbeddingUpdates(provider string, baseURL ...string) []ConfigUpdate {
	provider = strings.TrimSpace(strings.ToLower(provider))

	switch provider {
	case constants.ModelProviderOpenRouter:
		return []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultProfileEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOpenRouterEmbeddings},
			{Path: "models.embedding.baseURL", Value: ""},
		}
	case constants.ModelProviderOpenAI:
		return []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultProfileEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOpenAIEmbeddings},
			{Path: "models.embedding.baseURL", Value: ""},
		}
	case constants.ModelProviderOllama:
		updates := []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultOllamaEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOllamaEmbeddings},
		}
		if len(baseURL) > 0 {
			if value := strings.TrimSpace(baseURL[0]); value != "" {
				updates = append(updates, ConfigUpdate{Path: "models.embedding.baseURL", Value: value})
			}
		}

		return updates
	default:
		return []ConfigUpdate{
			{Path: "search.vector.enabled", Value: "false"},
		}
	}
}

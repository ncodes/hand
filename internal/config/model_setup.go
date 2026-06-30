package config

import (
	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	"github.com/wandxy/morph/pkg/stringx"
)

func ModelSetupEmbeddingUpdates(provider string, baseURL ...string) []ConfigUpdate {
	provider = stringx.String(provider).Normalized()

	switch provider {
	case constants.ModelProviderOpenRouter:
		return []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultProfileEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOpenRouterEmbeddings},
			{Path: "models.embedding.baseUrl", Value: ""},
		}
	case constants.ModelProviderOpenAI:
		return []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultProfileEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOpenAIEmbeddings},
			{Path: "models.embedding.baseUrl", Value: ""},
		}
	case constants.ModelProviderOllama:
		updates := []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultOllamaEmbeddingModel},
			{Path: "models.embedding.api", Value: modelprovider.APIOllamaEmbeddings},
		}
		if len(baseURL) > 0 {
			if value := stringx.String(baseURL[0]).Trim(); value != "" {
				updates = append(updates, ConfigUpdate{Path: "models.embedding.baseUrl", Value: value})
			}
		}

		return updates
	default:
		return []ConfigUpdate{
			{Path: "search.vector.enabled", Value: "false"},
		}
	}
}

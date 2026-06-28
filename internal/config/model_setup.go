package config

import (
	"strings"

	"github.com/wandxy/morph/internal/constants"
)

func ModelSetupEmbeddingUpdates(provider string) []ConfigUpdate {
	provider = strings.TrimSpace(strings.ToLower(provider))

	switch provider {
	case constants.ModelProviderOpenRouter, constants.ModelProviderOpenAI:
		return []ConfigUpdate{
			{Path: "models.embedding.provider", Value: provider},
			{Path: "models.embedding.name", Value: constants.DefaultProfileEmbeddingModel},
		}
	default:
		return []ConfigUpdate{
			{Path: "search.vector.enabled", Value: "false"},
		}
	}
}

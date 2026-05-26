package provider

import "github.com/wandxy/hand/internal/constants"

func defaultModels() []ModelDefinition {
	return []ModelDefinition{
		{
			ID:            constants.DefaultModel,
			Name:          "GPT-4o mini",
			Owner:         constants.ModelProviderOpenAI,
			Provider:      constants.ModelProviderOpenAI,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultProfileEmbeddingModel,
			Name:          "Text Embedding 3 Small",
			Owner:         constants.ModelProviderOpenAI,
			Provider:      constants.ModelProviderOpenAI,
			API:           APIOpenAIEmbeddings,
			Input:         []InputKind{InputText},
			ContextWindow: 8191,
		},
		{
			ID:            constants.DefaultProfileModel,
			Name:          "MiniMax M2.7",
			Owner:         "minimax",
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultModel,
			Name:          "GPT-4o mini via OpenRouter",
			Owner:         constants.ModelProviderOpenAI,
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultProfileEmbeddingModel,
			Name:          "Text Embedding 3 Small via OpenRouter",
			Owner:         constants.ModelProviderOpenAI,
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIEmbeddings,
			Input:         []InputKind{InputText},
			ContextWindow: 8191,
		},
		{
			ID:            "claude-sonnet-4-5",
			Name:          "Claude Sonnet 4.5",
			Owner:         constants.ModelProviderAnthropic,
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     64000,
		},
		{
			ID:            "claude-opus-4-1",
			Name:          "Claude Opus 4.1",
			Owner:         constants.ModelProviderAnthropic,
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     32000,
		},
		{
			ID:            "claude-3-haiku-20240307",
			Name:          "Claude 3 Haiku",
			Owner:         constants.ModelProviderAnthropic,
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     4096,
		},
	}
}

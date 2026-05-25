package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
)

func TestDefaultRegistry_RegistersBuiltInAPIs(t *testing.T) {
	registry := DefaultRegistry()

	completions, ok := registry.GetAPI(APIOpenAICompletions)
	require.True(t, ok)
	require.Equal(t, APIOpenAICompletions, completions.ID)

	embeddings, ok := registry.GetAPI(APIOpenAIEmbeddings)
	require.True(t, ok)
	require.Equal(t, APIOpenAIEmbeddings, embeddings.ID)

	require.ElementsMatch(t, []string{
		APIOpenAICompletions,
		APIOpenAIResponses,
		APIOpenAIEmbeddings,
		APIAnthropicMessages,
	}, registry.GetAPIIDs())

	anthropic, ok := registry.GetAPI(APIAnthropicMessages)
	require.True(t, ok)
	require.Equal(t, APIAnthropicMessages, anthropic.ID)
}

func TestDefaultRegistry_RegistersBuiltInProviders(t *testing.T) {
	registry := DefaultRegistry()

	openrouter, ok := registry.GetProvider(constants.ModelProviderOpenRouter)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, openrouter.DefaultAPI)
	require.Equal(t, constants.DefaultOpenRouterResponsesBaseURL, registry.GetBaseURL("openrouter", ""))
	require.Equal(t, constants.DefaultOpenRouterBaseURL, registry.GetBaseURL("openrouter", APIOpenAICompletions))
	require.Equal(t, constants.DefaultOpenRouterResponsesBaseURL, registry.GetBaseURL("openrouter", APIOpenAIResponses))
	require.Equal(t, constants.DefaultOpenRouterEmbeddingsBaseURL, registry.GetBaseURL("openrouter", APIOpenAIEmbeddings))
	require.Equal(t, []string{"OPENROUTER_API_KEY"}, openrouter.APIKeyEnv)

	openai, ok := registry.GetProvider(constants.ModelProviderOpenAI)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, openai.DefaultAPI)
	require.Equal(t, constants.DefaultOpenAIBaseURL, registry.GetBaseURL("openai", ""))
	require.Equal(t, constants.DefaultOpenAIBaseURL, registry.GetBaseURL("openai", APIOpenAICompletions))
	require.Equal(t, constants.DefaultOpenAIBaseURL, registry.GetBaseURL("openai", APIOpenAIResponses))
	require.Equal(t, constants.DefaultOpenAIEmbeddingsBaseURL, registry.GetBaseURL("openai", APIOpenAIEmbeddings))
	require.Equal(t, []string{"OPENAI_API_KEY"}, openai.APIKeyEnv)

	anthropic, ok := registry.GetProvider(constants.ModelProviderAnthropic)
	require.True(t, ok)
	require.Equal(t, APIAnthropicMessages, anthropic.DefaultAPI)
	require.Equal(t, constants.DefaultAnthropicBaseURL, registry.GetBaseURL("anthropic", ""))
	require.Equal(t, constants.DefaultAnthropicBaseURL, registry.GetBaseURL("anthropic", APIAnthropicMessages))
	require.Equal(t, []string{"ANTHROPIC_API_KEY"}, anthropic.APIKeyEnv)

	copilot, ok := registry.GetProvider(constants.ModelProviderGitHubCopilot)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, copilot.DefaultAPI)
	require.Equal(t, []string{"COPILOT_GITHUB_TOKEN"}, copilot.APIKeyEnv)
	require.True(t, copilot.SupportsOAuth)

	require.ElementsMatch(t, []string{
		constants.ModelProviderAnthropic,
		constants.ModelProviderGitHubCopilot,
		constants.ModelProviderOpenAI,
		constants.ModelProviderOpenRouter,
	}, registry.GetProviderIDs())
}

func TestDefaultRegistry_RegistersBuiltInModelsByProvider(t *testing.T) {
	registry := DefaultRegistry()

	openAIModel, ok := registry.GetModel("openai", constants.DefaultModel)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, openAIModel.API)
	require.Equal(t, []InputKind{InputText, InputImage}, openAIModel.Input)
	require.Equal(t, constants.DefaultContextLength, openAIModel.ContextWindow)

	openRouterModel, ok := registry.GetModel("openrouter", constants.DefaultProfileModel)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, openRouterModel.API)
	require.Equal(t, []InputKind{InputText}, openRouterModel.Input)

	openRouterOpenAIModel, ok := registry.GetModel("openrouter", constants.DefaultModel)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, openRouterOpenAIModel.API)
	require.Equal(t, []InputKind{InputText, InputImage}, openRouterOpenAIModel.Input)

	embedding, ok := registry.GetModel("openrouter", constants.DefaultProfileEmbeddingModel)
	require.True(t, ok)
	require.Equal(t, APIOpenAIEmbeddings, embedding.API)
	require.Equal(t, []InputKind{InputText}, embedding.Input)

	sonnet, ok := registry.GetModel("anthropic", "anthropic/claude-sonnet-4-5")
	require.True(t, ok)
	require.Equal(t, "Claude Sonnet 4.5", sonnet.Name)
	require.Equal(t, APIAnthropicMessages, sonnet.API)
	require.Equal(t, []InputKind{InputText, InputImage}, sonnet.Input)
	require.Equal(t, 200000, sonnet.ContextWindow)
	require.Equal(t, 64000, sonnet.MaxTokens)

	opus, ok := registry.GetModel("anthropic", "anthropic/claude-opus-4-1")
	require.True(t, ok)
	require.Equal(t, APIAnthropicMessages, opus.API)

	haiku, ok := registry.GetModel("anthropic", "anthropic/claude-3-haiku-20240307")
	require.True(t, ok)
	require.Equal(t, APIAnthropicMessages, haiku.API)
}

func TestRegistry_ReturnsCopies(t *testing.T) {
	registry := DefaultRegistry()

	provider, ok := registry.GetProvider("openrouter")
	require.True(t, ok)
	provider.BaseURLs[APIOpenAICompletions] = "changed"
	provider.APIKeyEnv[0] = "CHANGED"

	provider, ok = registry.GetProvider("openrouter")
	require.True(t, ok)
	require.Equal(t, constants.DefaultOpenRouterBaseURL, provider.BaseURLs[APIOpenAICompletions])
	require.Nil(t, provider.Headers)
	require.Equal(t, []string{"OPENROUTER_API_KEY"}, provider.APIKeyEnv)

	model, ok := registry.GetModel("openai", constants.DefaultModel)
	require.True(t, ok)
	model.Input[0] = InputImage

	model, ok = registry.GetModel("openai", constants.DefaultModel)
	require.True(t, ok)
	require.Equal(t, []InputKind{InputText, InputImage}, model.Input)
}

func TestRegistry_NilReceiverLookupsReturnFalse(t *testing.T) {
	var registry *Registry

	_, ok := registry.GetAPI(APIOpenAICompletions)
	require.False(t, ok)
	_, ok = registry.GetProvider(constants.ModelProviderOpenAI)
	require.False(t, ok)
	_, ok = registry.GetModel(constants.ModelProviderOpenAI, constants.DefaultModel)
	require.False(t, ok)
	require.Nil(t, registry.GetProviderIDs())
	require.Nil(t, registry.GetAPIIDs())
	require.Empty(t, registry.GetBaseURL(constants.ModelProviderOpenAI, APIOpenAIResponses))
}

func TestRegistry_MissingLookupsReturnFalse(t *testing.T) {
	registry := DefaultRegistry()

	_, ok := registry.GetAPI("missing")
	require.False(t, ok)
	_, ok = registry.GetProvider("missing")
	require.False(t, ok)
	_, ok = registry.GetModel("missing", constants.DefaultModel)
	require.False(t, ok)
	_, ok = registry.GetModel(constants.ModelProviderOpenAI, "missing")
	require.False(t, ok)
	require.Empty(t, registry.GetBaseURL("missing", APIOpenAIResponses))
	require.Empty(t, registry.GetBaseURL(constants.ModelProviderOpenAI, "missing"))
	require.False(t, registry.SupportsProviderAPI("missing", APIOpenAIResponses))
	require.False(t, registry.SupportsProviderAPI(constants.ModelProviderOpenAI, "missing"))
}

func TestRegistry_SupportsProviderAPI(t *testing.T) {
	registry := DefaultRegistry()

	require.True(t, registry.SupportsProviderAPI(constants.ModelProviderOpenRouter, ""))
	require.True(t, registry.SupportsProviderAPI(constants.ModelProviderOpenRouter, APIOpenAICompletions))
	require.True(t, registry.SupportsProviderAPI(constants.ModelProviderOpenAI, APIOpenAIEmbeddings))
	require.True(t, registry.SupportsProviderAPI(constants.ModelProviderAnthropic, APIAnthropicMessages))
	require.False(t, registry.SupportsProviderAPI(constants.ModelProviderAnthropic, APIOpenAIResponses))
}

func TestRegistry_NormalizesDefinitionsAndSkipsIncompleteEntries(t *testing.T) {
	registry := NewRegistry(
		[]APIDefinition{
			{ID: " Custom-API "},
			{ID: "   "},
		},
		[]ProviderDefinition{
			{
				ID:         " Custom ",
				DefaultAPI: " Custom-API ",
				BaseURLs: map[string]string{
					" Custom-API ": " https://custom.example/v1 ",
					"   ":          "ignored",
					" blank ":      " ",
				},
				Headers: map[string]string{
					" X-Custom ": " value ",
					" ignored ":  " ",
				},
			},
			{ID: "empty"},
			{
				ID: "blank-maps",
				BaseURLs: map[string]string{
					"blank": " ",
				},
				Headers: map[string]string{
					"blank": " ",
				},
			},
			{ID: "   "},
		},
		[]ModelDefinition{
			{ID: " model-one ", Provider: " Custom ", API: " Custom-API ", Input: []InputKind{InputText}},
			{ID: "", Provider: "custom", API: "custom-api"},
			{ID: "missing-provider"},
		},
	)

	api, ok := registry.GetAPI("custom-api")
	require.True(t, ok)
	require.Equal(t, "custom-api", api.ID)

	provider, ok := registry.GetProvider("custom")
	require.True(t, ok)
	require.Equal(t, "custom-api", provider.DefaultAPI)
	require.Equal(t, "https://custom.example/v1", provider.BaseURLs["custom-api"])
	require.NotContains(t, provider.BaseURLs, "")
	require.NotContains(t, provider.BaseURLs, "blank")
	require.Equal(t, "value", provider.Headers["x-custom"])
	require.NotContains(t, provider.Headers, "ignored")
	require.Equal(t, "https://custom.example/v1", registry.GetBaseURL("custom", ""))
	emptyProvider, ok := registry.GetProvider("empty")
	require.True(t, ok)
	require.Nil(t, emptyProvider.BaseURLs)
	blankMapsProvider, ok := registry.GetProvider("blank-maps")
	require.True(t, ok)
	require.Nil(t, blankMapsProvider.BaseURLs)
	require.Nil(t, blankMapsProvider.Headers)

	model, ok := registry.GetModel("custom", "model-one")
	require.True(t, ok)
	require.Equal(t, "custom-api", model.API)
	require.Equal(t, []InputKind{InputText}, model.Input)
	_, ok = registry.GetModel("custom", "")
	require.False(t, ok)
}

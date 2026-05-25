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
	require.Equal(t, constants.DefaultModelAPIModeCompletions, completions.RequestMode)

	responses, ok := registry.GetRequestModeAPI(constants.DefaultModelAPIModeResponses)
	require.True(t, ok)
	require.Equal(t, APIOpenAIResponses, responses.ID)
	defaultAPI, ok := registry.GetRequestModeAPI("")
	require.True(t, ok)
	require.Equal(t, APIOpenAICompletions, defaultAPI.ID)

	embeddings, ok := registry.GetAPI(APIOpenAIEmbeddings)
	require.True(t, ok)
	require.Equal(t, "embeddings", embeddings.RequestMode)

	require.ElementsMatch(t, []string{
		APIOpenAICompletions,
		APIOpenAIResponses,
		APIOpenAIEmbeddings,
	}, registry.GetAPIIDs())
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

	require.ElementsMatch(t, []string{
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
	_, ok = registry.GetRequestModeAPI(constants.DefaultModelAPIModeCompletions)
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
	_, ok = registry.GetRequestModeAPI("missing")
	require.False(t, ok)
	_, ok = registry.GetProvider("missing")
	require.False(t, ok)
	_, ok = registry.GetModel("missing", constants.DefaultModel)
	require.False(t, ok)
	_, ok = registry.GetModel(constants.ModelProviderOpenAI, "missing")
	require.False(t, ok)
	require.Empty(t, registry.GetBaseURL("missing", APIOpenAIResponses))
	require.Empty(t, registry.GetBaseURL(constants.ModelProviderOpenAI, "missing"))
}

func TestRegistry_NormalizesDefinitionsAndSkipsIncompleteEntries(t *testing.T) {
	registry := NewRegistry(
		[]APIDefinition{
			{ID: " Custom-API ", RequestMode: " Custom-Mode "},
			{ID: "   ", RequestMode: "ignored"},
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
	require.Equal(t, "custom-mode", api.RequestMode)
	api, ok = registry.GetRequestModeAPI("custom-mode")
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

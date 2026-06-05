package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func TestListOptions_FiltersGenerationModelsAndOrdersDisplayDefaultFirst(t *testing.T) {
	options := ListOptions(OptionQuery{
		Provider: constants.ModelProviderOpenAI,
		Current:  constants.DefaultModel,
	})

	require.NotEmpty(t, options)
	require.Equal(t, "gpt-5.5", options[0].ID)
	require.True(t, options[0].DisplayDefault)
	current := findModelOption(t, options, constants.DefaultModel)
	require.True(t, current.Current)
	for _, option := range options {
		require.NotEqual(t, modelprovider.APIOpenAIEmbeddings, option.API)
		require.NotEmpty(t, option.Input)
	}
}

func TestListOptions_FiltersOAuthOnlyModels(t *testing.T) {
	options := ListOptions(OptionQuery{
		Provider:  constants.ModelProviderOpenAICodex,
		Current:   "gpt-5.4",
		OAuthOnly: true,
	})

	require.NotEmpty(t, options)
	require.Equal(t, "gpt-5.5", options[0].ID)
	require.True(t, options[0].DisplayDefault)
	current := findModelOption(t, options, "gpt-5.4")
	require.True(t, current.Current)
	for _, option := range options {
		require.True(t, option.SupportsOAuth)
	}
}

func TestListOptions_ExcludesOpenAIPlatformModelsFromOAuthOnly(t *testing.T) {
	require.Empty(t, ListOptions(OptionQuery{
		Provider:  constants.ModelProviderOpenAI,
		Current:   "gpt-5.2",
		OAuthOnly: true,
	}))
}

func TestListOptions_FiltersAnthropicOAuthOnlyModels(t *testing.T) {
	options := ListOptions(OptionQuery{
		Provider:  constants.ModelProviderAnthropic,
		Current:   "claude-sonnet-4-6",
		OAuthOnly: true,
	})

	require.NotEmpty(t, options)
	require.Equal(t, "claude-sonnet-4-6", options[0].ID)
	require.True(t, options[0].DisplayDefault)
	require.True(t, options[0].Current)
	require.Len(t, options, 3)

	ids := make([]string, 0, len(options))
	for _, option := range options {
		require.True(t, option.SupportsOAuth, option.ID)
		ids = append(ids, option.ID)
	}
	require.ElementsMatch(t, []string{
		"claude-haiku-4-5",
		"claude-opus-4-7",
		"claude-sonnet-4-6",
	}, ids)
}

func TestListOptions_OrdersOpenRouterDisplayDefaultFirst(t *testing.T) {
	options := ListOptions(OptionQuery{
		Provider: constants.ModelProviderOpenRouter,
		Current:  "openai/gpt-5.5",
	})

	require.NotEmpty(t, options)
	require.Equal(t, constants.DefaultProfileModel, options[0].ID)
	require.True(t, options[0].DisplayDefault)
	current := findModelOption(t, options, "openai/gpt-5.5")
	require.True(t, current.Current)
}

func TestListOptions_ReturnsEmptyForMissingProviderOrRegistry(t *testing.T) {
	require.Empty(t, ListOptions(OptionQuery{Provider: "missing"}))
	require.Empty(t, ListOptions(OptionQuery{
		Provider: "missing",
		Registry: modelprovider.NewRegistry(nil, nil, nil),
	}))
}

func findModelOption(t *testing.T, options []Option, id string) Option {
	t.Helper()

	for _, option := range options {
		if option.ID == id {
			return option
		}
	}

	t.Fatalf("model option %q not found", id)
	return Option{}
}

func TestListProviders_ReturnsGenerationProviderCountsAndOrdersByDisplayIndex(t *testing.T) {
	options := ListProviders(ProviderQuery{
		Current: constants.ModelProviderOpenRouter,
		Auth: map[string]string{
			constants.ModelProviderOpenRouter: "api-key",
		},
	})

	require.NotEmpty(t, options)
	require.Equal(t, constants.ModelProviderOpenAI, options[0].ID)
	require.Equal(t, 0, options[0].DisplayIndex)
	require.True(t, options[0].HasDisplayIndex)

	openrouter := findProviderOption(t, options, constants.ModelProviderOpenRouter)
	require.Equal(t, "OpenRouter", openrouter.Name)
	require.Equal(t, "api-key", openrouter.Type)
	require.Equal(t, "api-key", openrouter.AuthType)
	require.True(t, openrouter.Current)
	require.True(t, openrouter.SupportsAPIKey)
	require.False(t, openrouter.SupportsOAuth)
	require.Greater(t, openrouter.ModelCount, 0)
}

func TestListProviders_ReturnsEmptyForNilRegistry(t *testing.T) {
	options := ListProviders(ProviderQuery{
		Registry: modelprovider.NewRegistry(nil, nil, nil),
	})

	require.Empty(t, options)
}

func TestListProviders_FiltersByAuthMethod(t *testing.T) {
	oauthOptions := ListProviders(ProviderQuery{OAuthOnly: true})
	require.NotEmpty(t, oauthOptions)
	require.Equal(t, constants.ModelProviderOpenAICodex, oauthOptions[0].ID)
	require.Equal(t, constants.ModelProviderAnthropic, oauthOptions[1].ID)
	require.Equal(t, constants.ModelProviderGitHubCopilot, oauthOptions[2].ID)
	oauthIDs := map[string]bool{}
	for _, option := range oauthOptions {
		require.True(t, option.SupportsOAuth, option.ID)
		oauthIDs[option.ID] = true
	}
	require.True(t, oauthIDs[constants.ModelProviderAnthropic])
	require.True(t, oauthIDs[constants.ModelProviderOpenAICodex])
	require.True(t, oauthIDs[constants.ModelProviderGitHubCopilot])
	require.False(t, oauthIDs[constants.ModelProviderOpenRouter])

	apiKeyOptions := ListProviders(ProviderQuery{APIKeyOnly: true})
	require.NotEmpty(t, apiKeyOptions)
	require.Equal(t, constants.ModelProviderOpenAI, apiKeyOptions[0].ID)
	require.Equal(t, constants.ModelProviderAnthropic, apiKeyOptions[1].ID)
	apiKeyIDs := map[string]bool{}
	for _, option := range apiKeyOptions {
		require.True(t, option.SupportsAPIKey, option.ID)
		apiKeyIDs[option.ID] = true
	}
	require.True(t, apiKeyIDs[constants.ModelProviderAnthropic])
	require.True(t, apiKeyIDs[constants.ModelProviderOpenAI])
	require.True(t, apiKeyIDs[constants.ModelProviderOpenRouter])
	require.False(t, apiKeyIDs[constants.ModelProviderOpenAICodex])
	require.True(t, apiKeyIDs[constants.ModelProviderGitHubCopilot])
}

func findProviderOption(t *testing.T, options []ProviderOption, id string) ProviderOption {
	t.Helper()

	for _, option := range options {
		if option.ID == id {
			return option
		}
	}

	t.Fatalf("provider option %q not found", id)
	return ProviderOption{}
}

func TestListProviders_FormatsProviderTypes(t *testing.T) {
	registry := modelprovider.NewRegistry(nil, []modelprovider.ProviderDefinition{
		{ID: "oauth", DisplayName: "OAuth", SupportsModels: true, SupportsOAuth: true},
		{ID: "both", DisplayName: "Both", SupportsModels: true, SupportsAPIKey: true, SupportsOAuth: true},
		{ID: "none", DisplayName: "None", SupportsModels: true},
		{ID: "tools-only", DisplayName: "Tools Only", SupportsModels: false, SupportsAPIKey: true},
		{ID: "embedding-only", DisplayName: "Embedding Only", SupportsModels: true, SupportsAPIKey: true},
	}, []modelprovider.ModelDefinition{
		{ID: "m1", Provider: "oauth", API: modelprovider.APIOpenAIResponses, Input: []modelprovider.InputKind{modelprovider.InputText}},
		{ID: "m2", Provider: "both", API: modelprovider.APIOpenAIResponses, Input: []modelprovider.InputKind{modelprovider.InputText}},
		{ID: "m3", Provider: "none", API: modelprovider.APIOpenAIResponses, Input: []modelprovider.InputKind{modelprovider.InputText}},
		{ID: "m4", Provider: "embedding-only", API: modelprovider.APIOpenAIEmbeddings, Input: []modelprovider.InputKind{modelprovider.InputText}},
	})

	options := ListProviders(ProviderQuery{Registry: registry})

	require.Len(t, options, 3)
	types := map[string]string{}
	for _, option := range options {
		types[option.ID] = option.Type
	}
	require.Equal(t, "oauth", types["oauth"])
	require.Equal(t, "api-key/oauth", types["both"])
	require.Equal(t, "none", types["none"])
	require.Empty(t, types["tools-only"])
	require.Empty(t, types["embedding-only"])
}

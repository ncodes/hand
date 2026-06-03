package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func TestListOptions_FiltersGenerationModelsAndOrdersCurrentFirst(t *testing.T) {
	options := ListOptions(OptionQuery{
		Provider: constants.ModelProviderOpenAI,
		Current:  constants.DefaultModel,
	})

	require.NotEmpty(t, options)
	require.Equal(t, constants.DefaultModel, options[0].ID)
	require.True(t, options[0].Current)
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
	require.Equal(t, "gpt-5.4", options[0].ID)
	require.True(t, options[0].Current)
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

func TestListOptions_ReturnsEmptyForMissingProviderOrRegistry(t *testing.T) {
	require.Empty(t, ListOptions(OptionQuery{Provider: "missing"}))
	require.Empty(t, ListOptions(OptionQuery{
		Provider: "missing",
		Registry: modelprovider.NewRegistry(nil, nil, nil),
	}))
}

func TestListProviders_ReturnsGenerationProviderCountsAndOrdersCurrentFirst(t *testing.T) {
	options := ListProviders(ProviderQuery{
		Current: constants.ModelProviderOpenRouter,
		Auth: map[string]string{
			constants.ModelProviderOpenRouter: "api-key",
		},
	})

	require.NotEmpty(t, options)
	require.Equal(t, constants.ModelProviderOpenRouter, options[0].ID)
	require.Equal(t, "OpenRouter", options[0].Name)
	require.Equal(t, "api-key", options[0].Type)
	require.Equal(t, "api-key", options[0].AuthType)
	require.True(t, options[0].Current)
	require.True(t, options[0].SupportsAPIKey)
	require.False(t, options[0].SupportsOAuth)
	require.Greater(t, options[0].ModelCount, 0)
}

func TestListProviders_ReturnsEmptyForNilRegistry(t *testing.T) {
	options := ListProviders(ProviderQuery{
		Registry: modelprovider.NewRegistry(nil, nil, nil),
	})

	require.Empty(t, options)
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

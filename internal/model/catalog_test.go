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

func TestListOptions_ReturnsEmptyForMissingProviderOrRegistry(t *testing.T) {
	require.Empty(t, ListOptions(OptionQuery{Provider: "missing"}))
	require.Empty(t, ListOptions(OptionQuery{
		Provider: "missing",
		Registry: modelprovider.NewRegistry(nil, nil, nil),
	}))
}

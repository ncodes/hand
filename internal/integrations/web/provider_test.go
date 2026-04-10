package web

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
)

func TestResolveOptions_UsesExplicitConfig(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider: "exa",
		WebAPIKey:   "exa-config-key",
		WebBaseURL:  "https://exa.example",
	})
	require.NoError(t, err)
	require.Equal(t, ProviderExa, opts.Provider)
	require.Equal(t, "exa-config-key", opts.APIKey)
	require.Equal(t, "https://exa.example", opts.BaseURL)
}

func TestResolveOptions_UsesDetectedProviderFallback(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider: ProviderParallel,
		WebAPIKey:   "parallel-key",
	})
	require.NoError(t, err)
	require.Equal(t, ProviderParallel, opts.Provider)
	require.Equal(t, "parallel-key", opts.APIKey)
	require.Equal(t, parallelDefaultBaseURL, opts.BaseURL)
}

func TestResolveOptions_UsesConfiguredBaseURL(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider: ProviderTavily,
		WebAPIKey:   "generic-key",
		WebBaseURL:  "https://web.example",
	})
	require.NoError(t, err)
	require.Equal(t, ProviderTavily, opts.Provider)
	require.Equal(t, "generic-key", opts.APIKey)
	require.Equal(t, "https://web.example", opts.BaseURL)
}

func TestResolveOptions_UsesConfiguredFirecrawlBaseURL(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider: ProviderFirecrawl,
		WebBaseURL:  "http://localhost:3002",
	})
	require.NoError(t, err)
	require.Equal(t, ProviderFirecrawl, opts.Provider)
	require.Equal(t, "http://localhost:3002", opts.BaseURL)
}

func TestResolveOptions_UsesConfiguredProviderWithoutAmbientEnvironment(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider: ProviderExa,
		WebAPIKey:   "generic-key",
	})
	require.NoError(t, err)
	require.Equal(t, ProviderExa, opts.Provider)
	require.Equal(t, "generic-key", opts.APIKey)
}

func TestResolveOptions_RejectsUnsupportedProvider(t *testing.T) {
	_, err := ResolveOptions(&config.Config{WebProvider: "unknown"})
	require.ErrorIs(t, err, ErrUnsupportedProvider)
}

func TestResolveOptions_RejectsMissingProviderConfiguration(t *testing.T) {
	_, err := ResolveOptions(&config.Config{})
	require.ErrorIs(t, err, ErrProviderNotConfigured)
}

func TestNewProvider_BuildsFirecrawlFromBaseURLOnly(t *testing.T) {
	provider, err := NewProvider(&config.Config{
		WebProvider: ProviderFirecrawl,
		WebBaseURL:  "http://localhost:3002",
	})
	require.NoError(t, err)

	httpProvider, ok := provider.(*HTTPProvider)
	require.True(t, ok)
	require.Equal(t, ProviderFirecrawl, httpProvider.Provider)
	require.Equal(t, "http://localhost:3002", httpProvider.BaseURL)
}

func TestNewProvider_BuildsParallelFromEnvironmentKey(t *testing.T) {
	provider, err := NewProvider(&config.Config{
		WebProvider: ProviderParallel,
		WebAPIKey:   "parallel-key",
	})
	require.NoError(t, err)

	httpProvider, ok := provider.(*HTTPProvider)
	require.True(t, ok)
	require.Equal(t, ProviderParallel, httpProvider.Provider)
	require.Equal(t, "parallel-key", httpProvider.APIKey)
	require.Equal(t, parallelDefaultBaseURL, httpProvider.BaseURL)
}

func TestNewProvider_BuildsTavilyFromEnvironmentKey(t *testing.T) {
	provider, err := NewProvider(&config.Config{
		WebProvider: ProviderTavily,
		WebAPIKey:   "tavily-key",
	})
	require.NoError(t, err)

	httpProvider, ok := provider.(*HTTPProvider)
	require.True(t, ok)
	require.Equal(t, ProviderTavily, httpProvider.Provider)
	require.Equal(t, tavilyDefaultBaseURL, httpProvider.BaseURL)
}

func TestNewProvider_BuildsExaFromEnvironmentKey(t *testing.T) {
	provider, err := NewProvider(&config.Config{
		WebProvider: ProviderExa,
		WebAPIKey:   "exa-key",
	})
	require.NoError(t, err)

	httpProvider, ok := provider.(*HTTPProvider)
	require.True(t, ok)
	require.Equal(t, ProviderExa, httpProvider.Provider)
	require.Equal(t, exaDefaultBaseURL, httpProvider.BaseURL)
}

func TestNewProvider_ReturnsCredentialErrors(t *testing.T) {
	_, err := NewProvider(&config.Config{WebProvider: ProviderParallel})
	require.EqualError(t, err, "parallel requires web API key")

	_, err = NewProvider(&config.Config{WebProvider: ProviderTavily})
	require.EqualError(t, err, "tavily requires web API key")

	_, err = NewProvider(&config.Config{WebProvider: ProviderExa})
	require.EqualError(t, err, "exa requires web API key")
}

package web

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
)

func TestResolveOptions_UsesExplicitConfig(t *testing.T) {
	opts, err := ResolveOptions(&config.Config{
		WebProvider:                "exa",
		WebAPIKey:                  "exa-config-key",
		WebBaseURL:                 "https://exa.example",
		WebMaxCharPerResult:        3200,
		WebMaxExtractCharPerResult: 12000,
		WebMaxExtractResponseBytes: 64000,
		WebNativeAllowedHosts:      []string{"allowed.example"},
		WebNativeBlockedHosts:      []string{"blocked.example"},
		WebNativeAllowedHostFiles:  []string{"allow.txt"},
		WebNativeBlockedHostFiles:  []string{"deny.txt"},
	})
	require.NoError(t, err)
	require.Equal(t, ProviderExa, opts.Provider)
	require.Equal(t, "exa-config-key", opts.APIKey)
	require.Equal(t, "https://exa.example", opts.BaseURL)
	require.Equal(t, 3200, opts.MaxCharPerResult)
	require.Equal(t, 12000, opts.MaxExtractCharPerResult)
	require.Equal(t, 64000, opts.MaxExtractResponseBytes)
	require.Equal(t, []string{"allowed.example"}, opts.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example"}, opts.NativeBlockedHosts)
	require.Equal(t, []string{"allow.txt"}, opts.NativeAllowedHostFiles)
	require.Equal(t, []string{"deny.txt"}, opts.NativeBlockedHostFiles)
}

func TestOptionsNormalize_CleansFieldsAndNegativeLimit(t *testing.T) {
	opts := Options{
		Provider:                " EXA ",
		APIKey:                  " key ",
		BaseURL:                 " https://exa.example ",
		MaxCharPerResult:        -10,
		MaxExtractCharPerResult: -20,
		MaxExtractResponseBytes: -30,
		NativeAllowedHosts:      []string{" allowed.example ", "allowed.example", ""},
		NativeBlockedHosts:      []string{" blocked.example ", "blocked.example", ""},
		NativeAllowedHostFiles:  []string{" allow.txt ", "allow.txt", ""},
		NativeBlockedHostFiles:  []string{" deny.txt ", "deny.txt", ""},
	}.Normalize()

	require.Equal(t, ProviderExa, opts.Provider)
	require.Equal(t, "key", opts.APIKey)
	require.Equal(t, "https://exa.example", opts.BaseURL)
	require.Zero(t, opts.MaxCharPerResult)
	require.Zero(t, opts.MaxExtractCharPerResult)
	require.Zero(t, opts.MaxExtractResponseBytes)
	require.Equal(t, []string{"allowed.example"}, opts.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example"}, opts.NativeBlockedHosts)
	require.Equal(t, []string{"allow.txt"}, opts.NativeAllowedHostFiles)
	require.Equal(t, []string{"deny.txt"}, opts.NativeBlockedHostFiles)
}

func TestExtractOptionsNormalize_CleansFieldsAndNegativeLimit(t *testing.T) {
	opts := ExtractOptions{Format: " TEXT ", MaxChars: -10, Query: " docs "}.Normalize()
	require.Equal(t, "text", opts.Format)
	require.Equal(t, "docs", opts.Query)
	require.Zero(t, opts.MaxChars)

	opts = ExtractOptions{Format: "html", MaxChars: 10}.Normalize()
	require.Empty(t, opts.Format)
	require.Equal(t, 10, opts.MaxChars)
}

func TestWithExtractOptions_RoundTripsNormalizedOptions(t *testing.T) {
	ctx := WithExtractOptions(context.Background(), ExtractOptions{Format: " MARKDOWN ", MaxChars: 12, Query: " specs "})

	require.Equal(t, ExtractOptions{Format: "markdown", MaxChars: 12, Query: "specs"}, ExtractOptionsFromContext(ctx))
	require.Equal(t, ExtractOptions{}, ExtractOptionsFromContext(context.Background()))
	require.Equal(t, ExtractOptions{}, ExtractOptionsFromContext(nil))
}

func TestExtractCharLimit_UsesRequestLimitWhenPresent(t *testing.T) {
	ctx := WithExtractOptions(context.Background(), ExtractOptions{MaxChars: 12})

	require.Equal(t, 12, extractCharLimit(ctx, 50))
	require.Equal(t, 50, extractCharLimit(context.Background(), 50))
}

func TestExtractFormat_UsesRequestFormatWhenPresent(t *testing.T) {
	ctx := WithExtractOptions(context.Background(), ExtractOptions{Format: "text"})

	require.Equal(t, "text", extractFormat(ctx, "markdown"))
	require.Equal(t, "markdown", extractFormat(context.Background(), "markdown"))
}

func TestExtractQuery_UsesRequestQueryWhenPresent(t *testing.T) {
	ctx := WithExtractOptions(context.Background(), ExtractOptions{Query: "release notes"})

	require.Equal(t, "release notes", extractQuery(ctx))
	require.Empty(t, extractQuery(context.Background()))
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
	require.Equal(t, config.DefaultWebMaxCharPerResult, opts.MaxCharPerResult)
	require.Equal(t, config.DefaultWebMaxExtractCharPerResult, opts.MaxExtractCharPerResult)
	require.Equal(t, config.DefaultWebMaxExtractResponseBytes, opts.MaxExtractResponseBytes)
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

func TestFillProviderDefaults_LeavesUnsupportedProviderUnchanged(t *testing.T) {
	opts := fillProviderDefaults(Options{Provider: "custom", BaseURL: "https://custom.example"})
	require.Equal(t, "custom", opts.Provider)
	require.Equal(t, "https://custom.example", opts.BaseURL)
}

func TestFillProviderDefaults_AppliesKnownProviderDefaults(t *testing.T) {
	testCases := []struct {
		name     string
		opts     Options
		expected string
	}{
		{name: "firecrawl", opts: Options{Provider: ProviderFirecrawl}, expected: firecrawlDefaultBaseURL},
		{name: "parallel", opts: Options{Provider: ProviderParallel}, expected: parallelDefaultBaseURL},
		{name: "tavily", opts: Options{Provider: ProviderTavily}, expected: tavilyDefaultBaseURL},
		{name: "exa", opts: Options{Provider: ProviderExa}, expected: exaDefaultBaseURL},
		{name: "native", opts: Options{Provider: ProviderNative}, expected: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := fillProviderDefaults(tc.opts)
			require.Equal(t, tc.expected, opts.BaseURL)
		})
	}
}

func TestTruncateToMaxChars_TrimsAndClamps(t *testing.T) {
	require.Equal(t, "", truncateToMaxChars("   ", 10))
	require.Equal(t, "hello", truncateToMaxChars(" hello ", 10))
	require.Equal(t, "hello world", truncateToMaxChars(" hello world ", 0))
	require.Equal(t, "hello", truncateToMaxChars("hello world", 5))
}

func TestLimitExtractContent_AppliesByteLimitBeforeCharLimit(t *testing.T) {
	content, truncated, downloadTruncated := limitExtractContent(" abcdef ", 4, 10)
	require.Equal(t, "abcd", content)
	require.True(t, truncated)
	require.True(t, downloadTruncated)
}

func TestLimitExtractContent_AppliesCharLimitWithoutDownloadTruncation(t *testing.T) {
	content, truncated, downloadTruncated := limitExtractContent("abcdef", 10, 4)
	require.Equal(t, "abcd", content)
	require.True(t, truncated)
	require.False(t, downloadTruncated)
}

func TestLimitExtractContent_TrimsPartialUTF8Rune(t *testing.T) {
	content, truncated, downloadTruncated := limitExtractContent("éclair", 1, 10)
	require.Equal(t, "", content)
	require.True(t, truncated)
	require.True(t, downloadTruncated)
}

func TestTruncateContent_ReportsTruncation(t *testing.T) {
	content, truncated := truncateContent("   ", 5)
	require.Empty(t, content)
	require.False(t, truncated)

	content, truncated = truncateContent(" hello ", 10)
	require.Equal(t, "hello", content)
	require.False(t, truncated)

	content, truncated = truncateContent(" hello world ", 0)
	require.Equal(t, "hello world", content)
	require.False(t, truncated)

	content, truncated = truncateContent("hello world", 5)
	require.Equal(t, "hello", content)
	require.True(t, truncated)
}

func TestNewProvider_ReturnsUnsupportedProviderError(t *testing.T) {
	_, err := NewProvider(&config.Config{WebProvider: "custom"})
	require.ErrorIs(t, err, ErrUnsupportedProvider)
}

func TestNewProvider_ReturnsNotConfiguredError(t *testing.T) {
	_, err := NewProvider(nil)
	require.ErrorIs(t, err, ErrProviderNotConfigured)
}

func TestNewProviderFromOptions_ReturnsUnsupportedProviderError(t *testing.T) {
	_, err := newProvider(Options{Provider: "custom"})
	require.ErrorIs(t, err, ErrUnsupportedProvider)
}

func TestNewProvider_BuildsConcreteProviders(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "firecrawl",
			cfg:  &config.Config{WebProvider: ProviderFirecrawl, WebBaseURL: "http://localhost:3002"},
		},
		{
			name: "parallel",
			cfg:  &config.Config{WebProvider: ProviderParallel, WebAPIKey: "parallel-key"},
		},
		{
			name: "tavily",
			cfg:  &config.Config{WebProvider: ProviderTavily, WebAPIKey: "tavily-key"},
		},
		{
			name: "exa",
			cfg:  &config.Config{WebProvider: ProviderExa, WebAPIKey: "exa-key"},
		},
		{
			name: "native",
			cfg:  &config.Config{WebProvider: ProviderNative},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewProvider(tc.cfg)
			require.NoError(t, err)
			require.NotNil(t, provider)
		})
	}
}

func TestNewProviderFromOptions_BuildsConcreteProviders(t *testing.T) {
	testCases := []struct {
		name string
		opts Options
	}{
		{
			name: "firecrawl",
			opts: Options{Provider: ProviderFirecrawl, BaseURL: "http://localhost:3002"},
		},
		{
			name: "parallel",
			opts: Options{Provider: ProviderParallel, APIKey: "parallel-key"},
		},
		{
			name: "tavily",
			opts: Options{Provider: ProviderTavily, APIKey: "tavily-key"},
		},
		{
			name: "exa",
			opts: Options{Provider: ProviderExa, APIKey: "exa-key"},
		},
		{
			name: "native",
			opts: Options{Provider: ProviderNative},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := newProvider(tc.opts)
			require.NoError(t, err)
			require.NotNil(t, provider)
		})
	}
}

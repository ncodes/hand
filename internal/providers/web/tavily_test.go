package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/pkg/logutils"
)

// real tavily provider test
func TestRealTavilyProvider_Search(t *testing.T) {
	provider, err := NewTavily(Options{APIKey: "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"})
	require.NoError(t, err)
	results, err := provider.Search(context.Background(), "nairaland", 10)
	require.NoError(t, err)
	logutils.PrettyPrint(results)
}

func TestNewTavily_BuildsFromAPIKeyOnly(t *testing.T) {
	provider, err := NewTavily(Options{APIKey: "tavily-key"})
	require.NoError(t, err)

	tavilyProvider, ok := provider.(*TavilyProvider)
	require.True(t, ok)
	require.Equal(t, tavilyDefaultBaseURL, tavilyProvider.client.baseURL)
	require.Equal(t, defaultMaxCharPerResult, tavilyProvider.maxCharsPerResult)
}

func TestNewTavily_PreservesConfiguredBaseURL(t *testing.T) {
	provider, err := NewTavily(Options{APIKey: "tavily-key", BaseURL: "https://tavily.example"})
	require.NoError(t, err)

	tavilyProvider, ok := provider.(*TavilyProvider)
	require.True(t, ok)
	require.Equal(t, "https://tavily.example", tavilyProvider.client.baseURL)
}

func TestNewTavily_UsesConfiguredMaxCharPerResult(t *testing.T) {
	provider, err := NewTavily(Options{APIKey: "tavily-key", MaxCharPerResult: 222})
	require.NoError(t, err)

	tavilyProvider, ok := provider.(*TavilyProvider)
	require.True(t, ok)
	require.Equal(t, 222, tavilyProvider.maxCharsPerResult)
}

func TestNewTavily_ReturnsCredentialError(t *testing.T) {
	_, err := NewTavily(Options{})
	require.EqualError(t, err, "tavily requires web API key")
}

func TestTavilyProvider_SearchNormalizesResults(t *testing.T) {
	var captured struct {
		Path              string
		Authorization     string
		Query             string `json:"query"`
		SearchDepth       string `json:"search_depth"`
		MaxResults        int    `json:"max_results"`
		IncludeRawContent bool   `json:"include_raw_content"`
		IncludeImages     bool   `json:"include_images"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Tavily result", "url": "https://example.com/tavily", "content": "A summary"},
			},
		}))
	}))
	defer server.Close()

	provider := &TavilyProvider{
		client: &httpClient{
			apiKey:  "tavily-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "tavily search", 2)
	require.NoError(t, err)
	require.Equal(t, "/search", captured.Path)
	require.Equal(t, "Bearer tavily-key", captured.Authorization)
	require.Equal(t, "tavily search", captured.Query)
	require.Equal(t, "basic", captured.SearchDepth)
	require.Equal(t, 2, captured.MaxResults)
	require.False(t, captured.IncludeRawContent)
	require.False(t, captured.IncludeImages)
	require.Equal(t, []SearchResult{{
		Title:    "Tavily result",
		URL:      "https://example.com/tavily",
		Snippet:  "A summary",
		Position: 1,
	}}, results)
}

func TestTavilyProvider_SearchTruncatesSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Tavily result", "url": "https://example.com/tavily", "content": "123456789"},
			},
		}))
	}))
	defer server.Close()

	provider := &TavilyProvider{
		client: &httpClient{
			apiKey:  "tavily-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
		maxCharsPerResult: 6,
	}

	results, err := provider.Search(context.Background(), "tavily search", 2)
	require.NoError(t, err)
	require.Equal(t, "123456", results[0].Snippet)
}

func TestTavilyProvider_SearchReturnsClientErrors(t *testing.T) {
	provider := &TavilyProvider{client: &httpClient{}}

	_, err := provider.Search(context.Background(), "tavily search", 2)
	require.EqualError(t, err, "web provider base URL is required")
}

func TestTavilyProvider_ExtractReturnsNotImplemented(t *testing.T) {
	results, err := (&TavilyProvider{}).Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, errProviderMethodNotImplemented)
	require.Nil(t, results)
}

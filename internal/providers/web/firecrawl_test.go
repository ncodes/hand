package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFirecrawl_BuildsFromBaseURLOnly(t *testing.T) {
	provider, err := NewFirecrawl(Options{BaseURL: "http://localhost:3002"})
	require.NoError(t, err)

	firecrawlProvider, ok := provider.(*FirecrawlProvider)
	require.True(t, ok)
	require.NotNil(t, firecrawlProvider.client)
	require.Equal(t, "http://localhost:3002", firecrawlProvider.client.baseURL)
	require.Equal(t, defaultMaxCharPerResult, firecrawlProvider.maxCharsPerResult)
}

func TestNewFirecrawl_BuildsFromAPIKeyOnly(t *testing.T) {
	provider, err := NewFirecrawl(Options{APIKey: "firecrawl-key"})
	require.NoError(t, err)

	firecrawlProvider, ok := provider.(*FirecrawlProvider)
	require.True(t, ok)
	require.Equal(t, "firecrawl-key", firecrawlProvider.client.apiKey)
	require.Equal(t, firecrawlDefaultBaseURL, firecrawlProvider.client.baseURL)
	require.Equal(t, defaultMaxCharPerResult, firecrawlProvider.maxCharsPerResult)
}

func TestNewFirecrawl_ReturnsCredentialError(t *testing.T) {
	_, err := NewFirecrawl(Options{})
	require.EqualError(t, err, "firecrawl requires web API key or base URL")
}

func TestFirecrawlProvider_SearchNormalizesResults(t *testing.T) {
	var captured struct {
		Path          string
		Authorization string
		Query         string `json:"query"`
		Limit         int    `json:"limit"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		captured.Path = r.URL.Path
		captured.Authorization = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"web": []map[string]any{
					{"title": "Docs", "url": "https://example.com/docs", "description": "Official documentation"},
				},
			},
		}))
	}))
	defer server.Close()

	provider := &FirecrawlProvider{
		client: &httpClient{
			apiKey:  "firecrawl-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "golang docs", 4)
	require.NoError(t, err)
	require.Equal(t, "/v2/search", captured.Path)
	require.Equal(t, "Bearer firecrawl-key", captured.Authorization)
	require.Equal(t, "golang docs", captured.Query)
	require.Equal(t, 4, captured.Limit)
	require.Equal(t, []SearchResult{{
		Title:    "Docs",
		URL:      "https://example.com/docs",
		Snippet:  "Official documentation",
		Position: 1,
	}}, results)
}

func TestFirecrawlProvider_SearchFallsBackToTopLevelWebAndSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"web": []map[string]any{
				{"title": "Docs", "url": "https://example.com/docs", "snippet": "Top-level snippet"},
			},
		}))
	}))
	defer server.Close()

	provider := &FirecrawlProvider{
		client: &httpClient{
			apiKey:  "firecrawl-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "golang docs", 4)
	require.NoError(t, err)
	require.Equal(t, []SearchResult{{
		Title:    "Docs",
		URL:      "https://example.com/docs",
		Snippet:  "Top-level snippet",
		Position: 1,
	}}, results)
}

func TestFirecrawlProvider_SearchTruncatesSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"web": []map[string]any{
				{"title": "Docs", "url": "https://example.com/docs", "snippet": "123456789"},
			},
		}))
	}))
	defer server.Close()

	provider := &FirecrawlProvider{
		client: &httpClient{
			apiKey:  "firecrawl-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
		maxCharsPerResult: 5,
	}

	results, err := provider.Search(context.Background(), "golang docs", 4)
	require.NoError(t, err)
	require.Equal(t, "12345", results[0].Snippet)
}

func TestFirecrawlProvider_SearchReturnsClientErrors(t *testing.T) {
	provider := &FirecrawlProvider{client: &httpClient{}}

	_, err := provider.Search(context.Background(), "golang docs", 4)
	require.EqualError(t, err, "web provider base URL is required")
}

func TestFirecrawlProvider_ExtractReturnsNotImplemented(t *testing.T) {
	results, err := (&FirecrawlProvider{}).Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, errProviderMethodNotImplemented)
	require.Nil(t, results)
}

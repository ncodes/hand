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
	require.Zero(t, firecrawlProvider.maxCharsPerResult)
	require.Zero(t, firecrawlProvider.maxExtractCharsPerResult)
	require.Zero(t, firecrawlProvider.maxExtractResponseBytes)
}

func TestNewFirecrawl_BuildsFromAPIKeyOnly(t *testing.T) {
	provider, err := NewFirecrawl(Options{APIKey: "firecrawl-key"})
	require.NoError(t, err)

	firecrawlProvider, ok := provider.(*FirecrawlProvider)
	require.True(t, ok)
	require.Equal(t, "firecrawl-key", firecrawlProvider.client.apiKey)
	require.Equal(t, firecrawlDefaultBaseURL, firecrawlProvider.client.baseURL)
	require.Zero(t, firecrawlProvider.maxCharsPerResult)
	require.Zero(t, firecrawlProvider.maxExtractCharsPerResult)
	require.Zero(t, firecrawlProvider.maxExtractResponseBytes)
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

func TestFirecrawlProvider_ExtractNormalizesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var body struct {
			URL             string   `json:"url"`
			Formats         []string `json:"formats"`
			OnlyMainContent bool     `json:"onlyMainContent"`
			Parsers         []string `json:"parsers"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "https://example.com", body.URL)
		require.Equal(t, []string{"markdown"}, body.Formats)
		require.True(t, body.OnlyMainContent)
		require.Equal(t, []string{"pdf"}, body.Parsers)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"markdown": "Extracted content",
				"metadata": map[string]any{
					"sourceURL": "https://example.com",
					"title":     "Example",
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
		maxExtractCharsPerResult: 100,
	}

	results, err := provider.Extract(context.Background(), []string{"https://example.com"})
	require.NoError(t, err)
	require.Equal(t, []ExtractResult{{
		URL:           "https://example.com",
		Title:         "Example",
		Content:       "Extracted content",
		ContentFormat: "markdown",
	}}, results)
}

func TestFirecrawlProvider_ExtractPreservesPerURLFailures(t *testing.T) {
	provider := &FirecrawlProvider{client: &httpClient{}}

	results, err := provider.Extract(context.Background(), []string{"https://example.com"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "https://example.com", results[0].URL)
	require.Equal(t, "markdown", results[0].ContentFormat)
	require.NotEmpty(t, results[0].Error)
}

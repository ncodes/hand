package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewExa_BuildsFromAPIKeyOnly(t *testing.T) {
	provider, err := NewExa(Options{APIKey: "exa-key"})
	require.NoError(t, err)

	exaProvider, ok := provider.(*ExaProvider)
	require.True(t, ok)
	require.Equal(t, exaDefaultBaseURL, exaProvider.client.baseURL)
	require.Equal(t, defaultMaxCharPerResult, exaProvider.maxCharsPerResult)
}

func TestNewExa_PreservesConfiguredBaseURL(t *testing.T) {
	provider, err := NewExa(Options{APIKey: "exa-key", BaseURL: "https://exa.example"})
	require.NoError(t, err)

	exaProvider, ok := provider.(*ExaProvider)
	require.True(t, ok)
	require.Equal(t, "https://exa.example", exaProvider.client.baseURL)
}

func TestNewExa_UsesConfiguredMaxCharPerResult(t *testing.T) {
	provider, err := NewExa(Options{APIKey: "exa-key", MaxCharPerResult: 400})
	require.NoError(t, err)

	exaProvider, ok := provider.(*ExaProvider)
	require.True(t, ok)
	require.Equal(t, 400, exaProvider.maxCharsPerResult)
}

func TestNewExa_ReturnsCredentialError(t *testing.T) {
	_, err := NewExa(Options{})
	require.EqualError(t, err, "exa requires web API key")
}

func TestExaProvider_SearchNormalizesResults(t *testing.T) {
	var captured struct {
		Path       string
		APIKey     string
		Query      string `json:"query"`
		NumResults int    `json:"numResults"`
		Contents   struct {
			Highlights struct {
				MaxCharacters int `json:"maxCharacters"`
			} `json:"highlights"`
		} `json:"contents"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		captured.Path = r.URL.Path
		captured.APIKey = r.Header.Get("x-api-key")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Exa result", "url": "https://example.com/exa", "highlights": []string{"Extracted snippet"}},
			},
		}))
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "exa search", 6)
	require.NoError(t, err)
	require.Equal(t, "/search", captured.Path)
	require.Equal(t, "exa-key", captured.APIKey)
	require.Equal(t, "exa search", captured.Query)
	require.Equal(t, 6, captured.NumResults)
	require.Equal(t, defaultMaxCharPerResult, captured.Contents.Highlights.MaxCharacters)
	require.Equal(t, []SearchResult{{
		Title:    "Exa result",
		URL:      "https://example.com/exa",
		Snippet:  "Extracted snippet",
		Position: 1,
	}}, results)
}

func TestExaProvider_SearchUsesConfiguredHighlightLimit(t *testing.T) {
	var captured struct {
		Contents struct {
			Highlights struct {
				MaxCharacters int `json:"maxCharacters"`
			} `json:"highlights"`
		} `json:"contents"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}}))
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
		maxCharsPerResult: 300,
	}

	_, err := provider.Search(context.Background(), "exa search", 2)
	require.NoError(t, err)
	require.Equal(t, 300, captured.Contents.Highlights.MaxCharacters)
}

func TestExaProvider_SearchFallsBackToText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Exa result", "url": "https://example.com/exa", "text": "text fallback"},
			},
		}))
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "exa search", 6)
	require.NoError(t, err)
	require.Equal(t, "text fallback", results[0].Snippet)
}

func TestExaProvider_SearchFallsBackToSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Exa result", "url": "https://example.com/exa", "summary": "summary fallback"},
			},
		}))
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "exa search", 6)
	require.NoError(t, err)
	require.Equal(t, "summary fallback", results[0].Snippet)
}

func TestExaProvider_SearchTruncatesSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Exa result", "url": "https://example.com/exa", "summary": "123456789"},
			},
		}))
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
		maxCharsPerResult: 7,
	}

	results, err := provider.Search(context.Background(), "exa search", 6)
	require.NoError(t, err)
	require.Equal(t, "1234567", results[0].Snippet)
}

func TestExaProvider_HeadersUseAPIKeyHeader(t *testing.T) {
	provider := &ExaProvider{client: &httpClient{apiKey: " exa-key "}}
	require.Equal(t, map[string]string{"x-api-key": "exa-key"}, provider.exaHeaders())
	require.Nil(t, (&ExaProvider{}).exaHeaders())
}

func TestExaProvider_SearchReturnsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad credentials", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := &ExaProvider{
		client: &httpClient{
			apiKey:  "exa-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	_, err := provider.Search(context.Background(), "exa search", 2)
	require.EqualError(t, err, "web provider request failed: bad credentials")
}

func TestExaProvider_ExtractReturnsNotImplemented(t *testing.T) {
	results, err := (&ExaProvider{}).Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, errProviderMethodNotImplemented)
	require.Nil(t, results)
}

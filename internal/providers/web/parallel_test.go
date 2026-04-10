package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewParallel_BuildsFromAPIKeyOnly(t *testing.T) {
	provider, err := NewParallel(Options{APIKey: "parallel-key"})
	require.NoError(t, err)

	parallelProvider, ok := provider.(*ParallelProvider)
	require.True(t, ok)
	require.Equal(t, parallelDefaultBaseURL, parallelProvider.client.baseURL)
	require.Equal(t, defaultMaxCharPerResult, parallelProvider.maxCharsPerResult)
}

func TestNewParallel_PreservesConfiguredBaseURL(t *testing.T) {
	provider, err := NewParallel(Options{APIKey: "parallel-key", BaseURL: "https://parallel.example"})
	require.NoError(t, err)

	parallelProvider, ok := provider.(*ParallelProvider)
	require.True(t, ok)
	require.Equal(t, "https://parallel.example", parallelProvider.client.baseURL)
}

func TestNewParallel_UsesConfiguredMaxCharPerResult(t *testing.T) {
	provider, err := NewParallel(Options{APIKey: "parallel-key", MaxCharPerResult: 333})
	require.NoError(t, err)

	parallelProvider, ok := provider.(*ParallelProvider)
	require.True(t, ok)
	require.Equal(t, 333, parallelProvider.maxCharsPerResult)
}

func TestNewParallel_ReturnsCredentialError(t *testing.T) {
	_, err := NewParallel(Options{})
	require.EqualError(t, err, "parallel requires web API key")
}

func TestParallelProvider_SearchNormalizesResults(t *testing.T) {
	var captured struct {
		Path       string
		APIKey     string
		Objective  string   `json:"objective"`
		Queries    []string `json:"search_queries"`
		MaxResults int      `json:"max_results"`
		Excerpts   struct {
			MaxCharsPerResult int `json:"max_chars_per_result"`
		} `json:"excerpts"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		captured.Path = r.URL.Path
		captured.APIKey = r.Header.Get("x-api-key")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Parallel result", "url": "https://example.com/parallel", "excerpts": []string{"first excerpt", "second excerpt"}},
			},
		}))
	}))
	defer server.Close()

	provider := &ParallelProvider{
		client: &httpClient{
			apiKey:  "parallel-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "parallel search", 3)
	require.NoError(t, err)
	require.Equal(t, "/v1beta/search", captured.Path)
	require.Equal(t, "parallel-key", captured.APIKey)
	require.Equal(t, "parallel search", captured.Objective)
	require.Equal(t, []string{"parallel search"}, captured.Queries)
	require.Equal(t, 3, captured.MaxResults)
	require.Equal(t, defaultMaxCharPerResult, captured.Excerpts.MaxCharsPerResult)
	require.Equal(t, []SearchResult{{
		Title:    "Parallel result",
		URL:      "https://example.com/parallel",
		Snippet:  "first excerpt second excerpt",
		Position: 1,
	}}, results)
}

func TestParallelProvider_SearchFallsBackToSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Parallel result", "url": "https://example.com/parallel", "snippet": "single snippet"},
			},
		}))
	}))
	defer server.Close()

	provider := &ParallelProvider{
		client: &httpClient{
			apiKey:  "parallel-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
	}

	results, err := provider.Search(context.Background(), "parallel search", 3)
	require.NoError(t, err)
	require.Equal(t, "single snippet", results[0].Snippet)
}

func TestParallelProvider_SearchTruncatesSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Parallel result", "url": "https://example.com/parallel", "snippet": "123456789"},
			},
		}))
	}))
	defer server.Close()

	provider := &ParallelProvider{
		client: &httpClient{
			apiKey:  "parallel-key",
			baseURL: server.URL,
			client:  server.Client(),
		},
		maxCharsPerResult: 4,
	}

	results, err := provider.Search(context.Background(), "parallel search", 3)
	require.NoError(t, err)
	require.Equal(t, "1234", results[0].Snippet)
}

func TestParallelProvider_SearchReturnsClientErrors(t *testing.T) {
	provider := &ParallelProvider{client: &httpClient{}}

	_, err := provider.Search(context.Background(), "parallel search", 3)
	require.EqualError(t, err, "web provider base URL is required")
}

func TestParallelProvider_SearchReturnsTransportErrors(t *testing.T) {
	provider := &ParallelProvider{
		client: &httpClient{
			baseURL: "http://127.0.0.1:1",
			client:  &http.Client{},
		},
	}

	_, err := provider.Search(context.Background(), "parallel search", 3)
	require.Error(t, err)
	var opErr *net.OpError
	require.True(t, errors.As(err, &opErr))
}

func TestParallelProvider_ExtractReturnsNotImplemented(t *testing.T) {
	results, err := (&ParallelProvider{}).Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, errProviderMethodNotImplemented)
	require.Nil(t, results)
}

func TestParallelProvider_ParallelHeaders(t *testing.T) {
	require.Nil(t, (*ParallelProvider)(nil).parallelHeaders())
	require.Nil(t, (&ParallelProvider{}).parallelHeaders())
	require.Nil(t, (&ParallelProvider{client: &httpClient{}}).parallelHeaders())
	require.Equal(t, map[string]string{
		"x-api-key": "parallel-key",
	}, (&ParallelProvider{client: &httpClient{apiKey: " parallel-key "}}).parallelHeaders())
}

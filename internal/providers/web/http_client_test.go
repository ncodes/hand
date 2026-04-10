package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPClient_PostJSONRequiresBaseURL(t *testing.T) {
	client := &httpClient{}

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.EqualError(t, err, "web provider base URL is required")
}

func TestHTTPClient_PostJSONUsesDefaultClientWhenClientIsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true}))
	}))
	defer server.Close()

	client := &httpClient{baseURL: server.URL}
	var payload map[string]any

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &payload)
	require.NoError(t, err)
	require.Equal(t, true, payload["ok"])
}

func TestHTTPClient_PostJSONReturnsStatusBodyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := &httpClient{baseURL: server.URL, client: server.Client()}

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.EqualError(t, err, "web provider request failed: bad request")
}

func TestHTTPClient_PostJSONReturnsStatusWhenBodyIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := &httpClient{baseURL: server.URL, client: server.Client()}

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.EqualError(t, err, "web provider request failed: 502 Bad Gateway")
}

func TestHTTPClient_PostJSONReturnsDecodeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	client := &httpClient{baseURL: server.URL, client: server.Client()}

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.Error(t, err)
}

func TestHTTPClient_PostJSONReturnsMarshalErrors(t *testing.T) {
	client := &httpClient{baseURL: "https://example.com"}

	err := client.postJSON(context.Background(), "/search", map[string]any{"bad": make(chan int)}, nil, &map[string]any{})
	require.Error(t, err)
}

func TestHTTPClient_PostJSONReturnsRequestConstructionErrors(t *testing.T) {
	client := &httpClient{baseURL: "https://example.com"}

	err := client.postJSON(context.Background(), "\n", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.Error(t, err)
}

func TestHTTPClient_PostJSONReturnsClientErrors(t *testing.T) {
	client := &httpClient{
		baseURL: "https://example.com",
		client: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return nil, errors.New("transport failed")
			}),
		},
	}

	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, nil, &map[string]any{})
	require.EqualError(t, err, "Post \"https://example.com/search\": transport failed")
}

func TestHTTPClient_AuthorizationHeadersHandlesBlankAPIKey(t *testing.T) {
	require.Nil(t, (&httpClient{}).authorizationHeaders())

	headers := (&httpClient{apiKey: " exa-key "}).authorizationHeaders()
	require.Equal(t, map[string]string{"Authorization": "Bearer exa-key"}, headers)
}

func TestHTTPClient_PostJSONUsesCustomClientTransport(t *testing.T) {
	client := &httpClient{
		baseURL: "https://example.com",
		client: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "https://example.com/search", r.URL.String())
				require.Empty(t, r.Header.Get("X-Empty"))
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			}),
		},
	}

	var payload map[string]any
	err := client.postJSON(context.Background(), "/search", map[string]any{"query": "golang"}, map[string]string{"X-Empty": "   "}, &payload)
	require.NoError(t, err)
	require.Equal(t, true, payload["ok"])
}

func TestFirstNonEmpty_ReturnsEmptyWhenNoValueQualifies(t *testing.T) {
	require.Equal(t, "", firstNonEmpty("", "   "))
}

func TestFirstHighlight_ReturnsFirstNonEmptyHighlight(t *testing.T) {
	require.Equal(t, "match", firstHighlight([]string{"   ", "match", "later"}))
	require.Equal(t, "", firstHighlight([]string{"", "  "}))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

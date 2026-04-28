package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingProvider_EmbedReturnsValidatedEmbeddings(t *testing.T) {
	var captured embeddingProviderRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/embeddings", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		_, _ = w.Write([]byte(`{
			"model":"text-embedding-test",
			"data":[
				{"index":0,"embedding":[1,0,0]},
				{"index":1,"embedding":[0,1,0]}
			]
		}`))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{})
	result, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "left", Text: "first"},
			{ID: "right", Text: "second"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "text-embedding-test", captured.Model)
	require.Equal(t, []string{"first", "second"}, captured.Input)
	require.Equal(t, "float", captured.EncodingFormat)
	require.Equal(t, EmbeddingResult{
		Model:      "text-embedding-test",
		Dimensions: 3,
		Items: []Embedding{
			{ID: "left", ContentHash: VectorContentHash("first"), Vector: []float64{1, 0, 0}},
			{ID: "right", ContentHash: VectorContentHash("second"), Vector: []float64{0, 1, 0}},
		},
	}, result)
}

func TestEmbeddingProvider_EmbedAcceptsProviderPrefixedModelAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingProviderRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "provider/text-embedding-model", req.Model)

		_, _ = w.Write([]byte(`{
			"model":"text-embedding-model",
			"data":[{"index":0,"embedding":[1,0]}]
		}`))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{})
	result, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "provider/text-embedding-model",
		Inputs: []EmbeddingInput{{ID: "one", Text: "first"}},
	})

	require.NoError(t, err)
	require.Equal(t, "provider/text-embedding-model", result.Model)
	require.Equal(t, []float64{1, 0}, result.Items[0].Vector)
}

func TestEmbeddingProvider_EmbedsInBatches(t *testing.T) {
	var batches [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingProviderRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		batches = append(batches, req.Input)

		data := make([]embeddingProviderResponseData, 0, len(req.Input))
		for idx := range req.Input {
			data = append(data, embeddingProviderResponseData{Index: idx, Embedding: []float64{float64(idx + 1), 1}})
		}
		require.NoError(t, json.NewEncoder(w).Encode(embeddingProviderResponse{
			Model: req.Model,
			Data:  data,
		}))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{
		MaxInputsPerBatch: 2,
	})
	result, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "one", Text: "one"},
			{ID: "two", Text: "two"},
			{ID: "three", Text: "three"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, [][]string{{"one", "two"}, {"three"}}, batches)
	require.Equal(t, []string{"one", "two", "three"}, []string{
		result.Items[0].ID,
		result.Items[1].ID,
		result.Items[2].ID,
	})
	require.Equal(t, 2, result.Dimensions)
}

func TestEmbeddingProvider_ReturnsErrorWhenBatchDimensionsChange(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		vector := []float64{1}
		if attempts == 2 {
			vector = []float64{1, 2}
		}

		require.NoError(t, json.NewEncoder(w).Encode(embeddingProviderResponse{
			Model: "text-embedding-test",
			Data:  []embeddingProviderResponseData{{Index: 0, Embedding: vector}},
		}))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{
		MaxInputsPerBatch: 1,
	})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "one", Text: "one"},
			{ID: "two", Text: "two"},
		},
	})

	require.EqualError(t, err, "embedding dimensions changed between batches")
}

func TestEmbeddingProvider_RetriesTransientFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusTooManyRequests)
			return
		}

		_, _ = w.Write([]byte(`{"model":"text-embedding-test","data":[{"index":0,"embedding":[1]}]}`))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{
		MaxRetries: 1,
	})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestEmbeddingProvider_ReturnsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.EqualError(t, err, "embedding request failed: bad request")
}

func TestEmbeddingProvider_UsesStatusWhenProviderErrorBodyIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{
		MaxRetries: 1,
	})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.EqualError(t, err, "embedding request failed: 500 Internal Server Error")
}

func TestEmbeddingProvider_RejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		inputs []EmbeddingInput
		err    string
	}{
		{
			name:   "missing model",
			body:   `{"data":[{"index":0,"embedding":[1]}]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "embedding result model is required",
		},
		{
			name:   "wrong count",
			body:   `{"model":"text-embedding-test","data":[]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "embedding result count must match input count",
		},
		{
			name:   "model mismatch",
			body:   `{"model":"other-model","data":[{"index":0,"embedding":[1]}]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "embedding result model must match request model",
		},
		{
			name:   "missing vector",
			body:   `{"model":"text-embedding-test","data":[{"index":0,"embedding":[]}]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "embedding vector is required",
		},
		{
			name: "duplicate index",
			body: `{"model":"text-embedding-test","data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`,
			inputs: []EmbeddingInput{
				{ID: "one", Text: "one"},
				{ID: "two", Text: "two"},
			},
			err: "embedding response index 0 is duplicated",
		},
		{
			name:   "out of range index",
			body:   `{"model":"text-embedding-test","data":[{"index":2,"embedding":[1]}]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "embedding response index 2 is out of range",
		},
		{
			name: "changed batch dimensions",
			body: `{"model":"text-embedding-test","data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1,2]}]}`,
			inputs: []EmbeddingInput{
				{ID: "one", Text: "one"},
				{ID: "two", Text: "two"},
			},
			err: "embedding vector dimensions do not match result dimensions",
		},
		{
			name:   "non finite vector value",
			body:   `{"model":"text-embedding-test","data":[{"index":0,"embedding":[1e999]}]}`,
			inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
			err:    "json: cannot unmarshal number 1e999 into Go struct field embeddingProviderResponseData.data.embedding of type float64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{})
			_, err := provider.Embed(context.Background(), EmbeddingRequest{
				Model:  "text-embedding-test",
				Inputs: tt.inputs,
			})

			require.EqualError(t, err, tt.err)
		})
	}
}

func TestEmbeddingProvider_ReturnsMalformedJSONErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{`))
	}))
	defer server.Close()

	provider := newTestEmbeddingProvider(t, server.URL+"/embeddings", EmbeddingProviderOptions{})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.EqualError(t, err, "unexpected EOF")
}

func TestEmbeddingProvider_EnforcesInputTextLimit(t *testing.T) {
	provider := newTestEmbeddingProvider(t, "http://example.test/embeddings", EmbeddingProviderOptions{
		MaxInputTextBytes: 3,
	})

	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "four"}},
	})

	require.EqualError(t, err, `embedding input "one" exceeds 3 bytes`)
}

func TestNewEmbeddingProvider_ValidatesOptions(t *testing.T) {
	_, err := NewEmbeddingProvider(EmbeddingProviderOptions{})
	require.EqualError(t, err, "embedding provider is required")

	_, err = NewEmbeddingProvider(EmbeddingProviderOptions{Provider: "openai", APIKey: "key"})
	require.EqualError(t, err, "embedding endpoint URL is required")

	_, err = NewEmbeddingProvider(EmbeddingProviderOptions{Provider: "openai"})
	require.EqualError(t, err, "embedding endpoint URL is required")

	_, err = NewEmbeddingProvider(EmbeddingProviderOptions{
		Provider:    "openai",
		EndpointURL: "https://api.openai.com/v1/embeddings",
	})
	require.EqualError(t, err, "embedding API key is required")

	_, err = NewEmbeddingProvider(EmbeddingProviderOptions{
		Provider:    "openai",
		APIKey:      "key",
		EndpointURL: "https://api.openai.com/v1/embeddings",
		MaxRetries:  -1,
	})
	require.EqualError(t, err, "embedding max retries must be non-negative")

	provider, err := NewEmbeddingProvider(EmbeddingProviderOptions{
		Provider:    "openai",
		APIKey:      "key",
		EndpointURL: "https://api.openai.com/v1/embeddings/",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/embeddings", provider.endpointURL)
	require.Equal(t, defaultEmbeddingMaxRetries, provider.maxRetries)

	provider, err = NewEmbeddingProvider(EmbeddingProviderOptions{
		Provider:    "openrouter",
		APIKey:      "key",
		EndpointURL: "https://openrouter.ai/api/v1/embeddings",
	})
	require.NoError(t, err)
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", provider.endpointURL)
}

func TestEmbeddingProvider_RejectsInvalidRequests(t *testing.T) {
	var provider *EmbeddingProvider
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{{
			ID:         "one",
			Text:       "one",
			SourceKind: SourceKindMemoryItem,
		}},
	})
	require.EqualError(t, err, "embedding provider is required")

	provider = newTestEmbeddingProvider(t, "http://example.test/embeddings", EmbeddingProviderOptions{})
	_, err = provider.Embed(context.Background(), EmbeddingRequest{})
	require.EqualError(t, err, "embedding model is required")
}

func TestEmbeddingProvider_RejectsInvalidEndpointURL(t *testing.T) {
	provider := newTestEmbeddingProvider(t, "://bad", EmbeddingProviderOptions{})
	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.ErrorContains(t, err, `missing protocol scheme`)
}

func TestEmbeddingProvider_ReturnsClientErrors(t *testing.T) {
	provider := newTestEmbeddingProvider(t, "http://example.test/embeddings", EmbeddingProviderOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})},
		MaxRetries: 0,
	})

	_, err := provider.Embed(context.Background(), EmbeddingRequest{
		Model:  "text-embedding-test",
		Inputs: []EmbeddingInput{{ID: "one", Text: "one"}},
	})

	require.ErrorContains(t, err, "network down")
}

func newTestEmbeddingProvider(
	t *testing.T,
	endpointURL string,
	opts EmbeddingProviderOptions,
) *EmbeddingProvider {
	t.Helper()

	opts.Provider = "openai"
	opts.APIKey = "test-key"
	opts.EndpointURL = endpointURL
	opts.Timeout = time.Second
	provider, err := NewEmbeddingProvider(opts)
	require.NoError(t, err)

	return provider
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

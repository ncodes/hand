package webextract

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	webprovider "github.com/wandxy/hand/internal/providers/web"
	"github.com/wandxy/hand/internal/tools"
)

type stubProvider struct {
	extract func(context.Context, []string) ([]webprovider.ExtractResult, error)
}

func (stubProvider) Search(context.Context, string, int) ([]webprovider.SearchResult, error) {
	return nil, errors.New("unexpected search call")
}

func (s stubProvider) Extract(ctx context.Context, urls []string) ([]webprovider.ExtractResult, error) {
	return s.extract(ctx, urls)
}

func registerTool(t *testing.T, provider webprovider.Provider, options ...Options) tools.Registry {
	t.Helper()

	registry := tools.NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(Definition(provider, options...)))

	return registry
}

func TestWebExtract_RejectsMalformedInput(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
}

func TestWebExtract_RejectsEmptyURLs(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":[]}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "urls is required", toolErr.Message)
}

func TestWebExtract_RejectsBlankURL(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["https://example.com","  "]}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "url at index 1 is required", toolErr.Message)
}

func TestWebExtract_RejectsTooManyURLs(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["1","2","3","4","5","6"]}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "too many urls", toolErr.Message)
}

func TestWebExtract_ReturnsProviderResults(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(_ context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Equal(t, []string{"https://example.com"}, urls)
			return []webprovider.ExtractResult{{
				URL:               "https://example.com",
				Title:             "Example",
				Content:           "Hello",
				ContentFormat:     "markdown",
				Truncated:         true,
				DownloadTruncated: true,
			}}, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["https://example.com"]}`})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 1)
	require.Equal(t, "Example", payload.Results[0].Title)
	require.True(t, payload.Results[0].Truncated)
	require.True(t, payload.Results[0].DownloadTruncated)
	require.Contains(t, result.Output, `"download_truncated":true`)
}

func TestWebExtract_AppliesPerCallMaxChars(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(ctx context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Equal(t, []string{"https://example.com"}, urls)
			require.Equal(t, webprovider.ExtractOptions{MaxChars: 3}, webprovider.ExtractOptionsFromContext(ctx))
			return []webprovider.ExtractResult{{
				URL:           "https://example.com",
				Content:       "abc",
				ContentFormat: "text",
				Truncated:     true,
			}}, nil
		},
	}, Options{MaxExtractCharPerResult: 10})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"max_chars":3}`,
	})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 1)
	require.Equal(t, "abc", payload.Results[0].Content)
	require.True(t, payload.Results[0].Truncated)
	require.False(t, payload.Results[0].DownloadTruncated)
}

func TestWebExtract_ClampsPerCallMaxCharsToConfiguredMax(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(ctx context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Equal(t, []string{"https://example.com"}, urls)
			require.Equal(t, webprovider.ExtractOptions{MaxChars: 4}, webprovider.ExtractOptionsFromContext(ctx))
			return []webprovider.ExtractResult{{
				URL:           "https://example.com",
				Content:       "abcd",
				ContentFormat: "text",
				Truncated:     true,
			}}, nil
		},
	}, Options{MaxExtractCharPerResult: 4})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"max_chars":10}`,
	})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 1)
	require.Equal(t, "abcd", payload.Results[0].Content)
	require.True(t, payload.Results[0].Truncated)
}

func TestWebExtract_RejectsInvalidMaxChars(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"max_chars":0}`,
	})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "max_chars must be greater than zero", toolErr.Message)
}

func TestWebExtract_AppliesTextFormat(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(ctx context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Equal(t, []string{"https://example.com"}, urls)
			require.Equal(t, webprovider.ExtractOptions{Format: "text"}, webprovider.ExtractOptionsFromContext(ctx))
			return []webprovider.ExtractResult{{
				URL:           "https://example.com",
				Content:       "Title\n\nRead docs and ship it.",
				ContentFormat: "text",
			}}, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"format":"text"}`,
	})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 1)
	require.Equal(t, "text", payload.Results[0].ContentFormat)
	require.Equal(t, "Title\n\nRead docs and ship it.", payload.Results[0].Content)
}

func TestWebExtract_AppliesExtractModeAlias(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(ctx context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Equal(t, []string{"https://example.com"}, urls)
			require.Equal(t, webprovider.ExtractOptions{Format: "markdown"}, webprovider.ExtractOptionsFromContext(ctx))
			return []webprovider.ExtractResult{{
				URL:           "https://example.com",
				Content:       "plain content",
				ContentFormat: "markdown",
			}}, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"extract_mode":"markdown"}`,
	})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 1)
	require.Equal(t, "markdown", payload.Results[0].ContentFormat)
	require.Equal(t, "plain content", payload.Results[0].Content)
}

func TestWebExtract_RejectsInvalidFormat(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"format":"html"}`,
	})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "format must be text or markdown", toolErr.Message)
}

func TestWebExtract_RejectsConflictingFormatAliases(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			t.Fatal("extract should not be called")
			return nil, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "web_extract",
		Input: `{"urls":["https://example.com"],"format":"text","extract_mode":"markdown"}`,
	})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "format and extract_mode must match when both are provided", toolErr.Message)
}

func TestWebExtract_PreservesPartialFailures(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(_ context.Context, urls []string) ([]webprovider.ExtractResult, error) {
			require.Len(t, urls, 2)
			return []webprovider.ExtractResult{
				{URL: "https://ok.example", Content: "ok", ContentFormat: "markdown"},
				{URL: "https://bad.example", ContentFormat: "markdown", Error: "fetch failed"},
			}, nil
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["https://ok.example","https://bad.example"]}`})
	require.NoError(t, err)

	var payload struct {
		Results []webprovider.ExtractResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Results, 2)
	require.Equal(t, "fetch failed", payload.Results[1].Error)
}

func TestWebExtract_ReturnsProviderErrorsAsToolErrors(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			return nil, errors.New("provider failed")
		},
	})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["https://example.com"]}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "tool_error", toolErr.Code)
	require.Equal(t, "provider failed", toolErr.Message)
}

func TestWebExtract_ReturnsErrorWhenProviderIsNil(t *testing.T) {
	registry := registerTool(t, nil)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "web_extract", Input: `{"urls":["https://example.com"]}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "tool_error", toolErr.Code)
	require.Equal(t, "web extract provider is not configured", toolErr.Message)
}

func TestWebExtract_RequiresNetworkCapability(t *testing.T) {
	registry := registerTool(t, stubProvider{
		extract: func(context.Context, []string) ([]webprovider.ExtractResult, error) {
			return nil, nil
		},
	})

	withNetwork, err := registry.Resolve(tools.Policy{Capabilities: tools.Capabilities{Network: true}})
	require.NoError(t, err)
	require.Len(t, withNetwork, 1)
	require.Equal(t, "web_extract", withNetwork[0].Name)

	withoutNetwork, err := registry.Resolve(tools.Policy{})
	require.NoError(t, err)
	require.Empty(t, withoutNetwork)
}

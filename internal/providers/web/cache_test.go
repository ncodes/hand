package web

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cacheStubProvider struct {
	searchCalls  int
	extractCalls int
	search       func(context.Context, string, int) ([]SearchResult, error)
	extract      func(context.Context, []string) ([]ExtractResult, error)
}

func (p *cacheStubProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	p.searchCalls++
	return p.search(ctx, query, count)
}

func (p *cacheStubProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	p.extractCalls++
	return p.extract(ctx, urls)
}

func TestCachedProvider_SearchCachesSuccessfulResults(t *testing.T) {
	now := time.Unix(100, 0)
	stub := &cacheStubProvider{
		search: func(context.Context, string, int) ([]SearchResult, error) {
			return []SearchResult{{Title: "Go", URL: "https://go.dev", Position: 1}}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
		Now:          func() time.Time { return now },
	})

	first, err := provider.Search(context.Background(), "golang", 5)
	require.NoError(t, err)
	first[0].Title = "mutated"

	second, err := provider.Search(context.Background(), "golang", 5)
	require.NoError(t, err)

	require.Equal(t, 1, stub.searchCalls)
	require.Equal(t, "Go", second[0].Title)
}

func TestCachedProvider_SearchExpires(t *testing.T) {
	now := time.Unix(100, 0)
	stub := &cacheStubProvider{
		search: func(context.Context, string, int) ([]SearchResult, error) {
			return []SearchResult{{Title: "result"}}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
		Now:          func() time.Time { return now },
	})

	_, err := provider.Search(context.Background(), "golang", 5)
	require.NoError(t, err)
    
	now = now.Add(time.Minute)
	_, err = provider.Search(context.Background(), "golang", 5)
	require.NoError(t, err)

	require.Equal(t, 2, stub.searchCalls)
}

func TestCachedProvider_SearchDoesNotCacheErrors(t *testing.T) {
	stub := &cacheStubProvider{
		search: func(context.Context, string, int) ([]SearchResult, error) {
			return nil, errors.New("provider failed")
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	_, err := provider.Search(context.Background(), "golang", 5)
	require.EqualError(t, err, "provider failed")
	_, err = provider.Search(context.Background(), "golang", 5)
	require.EqualError(t, err, "provider failed")

	require.Equal(t, 2, stub.searchCalls)
}

func TestCachedProvider_SearchPreservesEmptyResultSlices(t *testing.T) {
	stub := &cacheStubProvider{
		search: func(context.Context, string, int) ([]SearchResult, error) {
			return []SearchResult{}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	first, err := provider.Search(context.Background(), "nothing", 5)
	require.NoError(t, err)
	second, err := provider.Search(context.Background(), "nothing", 5)
	require.NoError(t, err)

	require.NotNil(t, first)
	require.Empty(t, first)
	require.NotNil(t, second)
	require.Empty(t, second)
	require.Equal(t, 1, stub.searchCalls)
}

func TestCachedProvider_ExtractCachesSuccessfulResultsAndPreservesOrder(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(_ context.Context, urls []string) ([]ExtractResult, error) {
			results := make([]ExtractResult, 0, len(urls))
			for _, url := range urls {
				results = append(results, ExtractResult{
					URL:           url,
					Title:         "title " + url,
					Content:       "content " + url,
					ContentFormat: "text",
				})
			}
			return results, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	ctx := WithExtractOptions(context.Background(), ExtractOptions{Format: "text", MaxChars: 100, Query: "docs"})
	first, err := provider.Extract(ctx, []string{"https://a.example", "https://b.example"})
	require.NoError(t, err)
	first[0].Title = "mutated"

	second, err := provider.Extract(ctx, []string{"https://b.example", "https://a.example"})
	require.NoError(t, err)

	require.Equal(t, 1, stub.extractCalls)
	require.Equal(t, []string{"https://b.example", "https://a.example"}, []string{second[0].URL, second[1].URL})
	require.Equal(t, "title https://a.example", second[1].Title)
}

func TestCachedProvider_ExtractExpires(t *testing.T) {
	now := time.Unix(100, 0)
	stub := &cacheStubProvider{
		extract: func(_ context.Context, urls []string) ([]ExtractResult, error) {
			return []ExtractResult{{
				URL:           urls[0],
				Content:       "content",
				ContentFormat: "text",
			}}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
		Now:          func() time.Time { return now },
	})

	_, err := provider.Extract(context.Background(), []string{"https://example.com"})
	require.NoError(t, err)
	now = now.Add(time.Minute)
	_, err = provider.Extract(context.Background(), []string{"https://example.com"})
	require.NoError(t, err)

	require.Equal(t, 2, stub.extractCalls)
}

func TestCachedProvider_ExtractDoesNotCacheProviderErrors(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(context.Context, []string) ([]ExtractResult, error) {
			return nil, errors.New("provider failed")
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	_, err := provider.Extract(context.Background(), []string{"https://example.com"})
	require.EqualError(t, err, "provider failed")
	_, err = provider.Extract(context.Background(), []string{"https://example.com"})
	require.EqualError(t, err, "provider failed")

	require.Equal(t, 2, stub.extractCalls)
}

func TestCachedProvider_ExtractReturnsNilForEmptyURLList(t *testing.T) {
	stub := &cacheStubProvider{}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	results, err := provider.Extract(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, results)
	require.Zero(t, stub.extractCalls)
}

func TestCachedProvider_ExtractCachesSuccessfulSiblingsOnly(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(_ context.Context, urls []string) ([]ExtractResult, error) {
			results := make([]ExtractResult, 0, len(urls))
			for _, url := range urls {
				result := ExtractResult{URL: url, ContentFormat: "text", Content: "ok"}
				if url == "https://bad.example" {
					result.Error = "failed"
				}
				results = append(results, result)
			}
			return results, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	_, err := provider.Extract(context.Background(), []string{"https://ok.example", "https://bad.example"})
	require.NoError(t, err)
	second, err := provider.Extract(context.Background(), []string{"https://ok.example", "https://bad.example"})
	require.NoError(t, err)

	require.Equal(t, 2, stub.extractCalls)
	require.Equal(t, "https://ok.example", second[0].URL)
	require.Equal(t, "https://bad.example", second[1].URL)
	require.Equal(t, "failed", second[1].Error)
}

func TestCachedProvider_ExtractMarksMissingProviderResults(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(context.Context, []string) ([]ExtractResult, error) {
			return []ExtractResult{{
				URL:           "https://ok.example",
				Content:       "ok",
				ContentFormat: "text",
			}}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	results, err := provider.Extract(context.Background(), []string{"https://ok.example", "https://missing.example"})
	require.NoError(t, err)

	require.Equal(t, []ExtractResult{
		{URL: "https://ok.example", Content: "ok", ContentFormat: "text"},
		{URL: "https://missing.example", Error: "web extraction provider returned no result"},
	}, results)
}

func TestCachedProvider_ExtractIgnoresExtraProviderResults(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(context.Context, []string) ([]ExtractResult, error) {
			return []ExtractResult{
				{URL: "https://ok.example", Content: "ok", ContentFormat: "text"},
				{URL: "https://extra.example", Content: "extra", ContentFormat: "text"},
			}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	results, err := provider.Extract(context.Background(), []string{"https://ok.example"})
	require.NoError(t, err)

	require.Equal(t, []ExtractResult{
		{URL: "https://ok.example", Content: "ok", ContentFormat: "text"},
	}, results)
}

func TestCachedProvider_ExtractKeyIncludesOptions(t *testing.T) {
	stub := &cacheStubProvider{
		extract: func(ctx context.Context, urls []string) ([]ExtractResult, error) {
			opts := ExtractOptionsFromContext(ctx)
			return []ExtractResult{{
				URL:           urls[0],
				Content:       opts.Format + ":" + opts.Query,
				ContentFormat: "text",
			}}, nil
		},
	}
	provider := NewCachedProvider(stub, CacheOptions{
		ProviderName: "exa",
		TTL:          time.Minute,
	})

	ctxA := WithExtractOptions(context.Background(), ExtractOptions{Format: "text", MaxChars: 100, Query: "docs"})
	ctxB := WithExtractOptions(context.Background(), ExtractOptions{Format: "markdown", MaxChars: 100, Query: "docs"})
	ctxC := WithExtractOptions(context.Background(), ExtractOptions{Format: "text", MaxChars: 100, Query: "api"})
	ctxD := WithExtractOptions(context.Background(), ExtractOptions{Format: "text", MaxChars: 200, Query: "docs"})

	for _, ctx := range []context.Context{ctxA, ctxA, ctxB, ctxC, ctxD} {
		_, err := provider.Extract(ctx, []string{"https://example.com"})
		require.NoError(t, err)
	}

	require.Equal(t, 4, stub.extractCalls)
}

func TestNewCachedProvider_ReturnsOriginalProviderWhenDisabled(t *testing.T) {
	stub := &cacheStubProvider{}

	require.Same(t, stub, NewCachedProvider(stub, CacheOptions{}))
	require.Nil(t, NewCachedProvider(nil, CacheOptions{TTL: time.Minute}))
}

func TestCachedProvider_ReturnsErrorWhenProviderIsMissing(t *testing.T) {
	var provider *cachedProvider

	_, err := provider.Search(context.Background(), "golang", 5)
	require.ErrorIs(t, err, ErrProviderNotConfigured)
	_, err = provider.Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, ErrProviderNotConfigured)

	provider = &cachedProvider{}
	_, err = provider.Search(context.Background(), "golang", 5)
	require.ErrorIs(t, err, ErrProviderNotConfigured)
	_, err = provider.Extract(context.Background(), []string{"https://example.com"})
	require.ErrorIs(t, err, ErrProviderNotConfigured)
}

func TestCloneResults_PreservesNilSlices(t *testing.T) {
	require.Nil(t, cloneSearchResults(nil))
	require.Nil(t, cloneExtractResults(nil))
}

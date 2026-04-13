package web

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/hand/pkg/logutils"
)

var webLog = logutils.InitLogger("providers.web")

type CacheOptions struct {
	ProviderName string
	TTL          time.Duration
	Now          func() time.Time
}

type cachedProvider struct {
	provider     Provider
	providerName string
	ttl          time.Duration
	now          func() time.Time
	mu           sync.Mutex
	search       map[string]cachedSearch
	extract      map[string]cachedExtract
}

type cachedSearch struct {
	expiresAt time.Time
	results   []SearchResult
}

type cachedExtract struct {
	expiresAt time.Time
	result    ExtractResult
}

func NewCachedProvider(provider Provider, opts CacheOptions) Provider {
	if provider == nil || opts.TTL <= 0 {
		return provider
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &cachedProvider{
		provider:     provider,
		providerName: strings.TrimSpace(strings.ToLower(opts.ProviderName)),
		ttl:          opts.TTL,
		now:          now,
		search:       make(map[string]cachedSearch),
		extract:      make(map[string]cachedExtract),
	}
}

func (p *cachedProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	if p == nil || p.provider == nil {
		return nil, ErrProviderNotConfigured
	}

	key := p.searchKey(query, count)
	if results, ok := p.cachedSearch(key); ok {
		p.logCacheHit("search")
		return results, nil
	}

	results, err := p.provider.Search(ctx, query, count)
	if err != nil {
		return nil, err
	}

	p.storeSearch(key, results)
	return cloneSearchResults(results), nil
}

func (p *cachedProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	if p == nil || p.provider == nil {
		return nil, ErrProviderNotConfigured
	}
	if len(urls) == 0 {
		return nil, nil
	}

	results := make([]ExtractResult, len(urls))
	missURLs := make([]string, 0, len(urls))
	missIndexes := make([]int, 0, len(urls))
	missKeys := make([]string, 0, len(urls))
	for idx, rawURL := range urls {
		key := p.extractKey(ctx, rawURL)
		if result, ok := p.cachedExtract(key); ok {
			p.logCacheHit("extract")
			results[idx] = result
			continue
		}

		missURLs = append(missURLs, rawURL)
		missIndexes = append(missIndexes, idx)
		missKeys = append(missKeys, key)
	}

	if len(missURLs) == 0 {
		return results, nil
	}

	fetched, err := p.provider.Extract(ctx, missURLs)
	if err != nil {
		return nil, err
	}

	for idx, result := range fetched {
		if idx >= len(missIndexes) {
			break
		}

		results[missIndexes[idx]] = result
		if strings.TrimSpace(result.Error) == "" {
			p.storeExtract(missKeys[idx], result)
		}
	}
	for idx := len(fetched); idx < len(missIndexes); idx++ {
		results[missIndexes[idx]] = ExtractResult{
			URL:   strings.TrimSpace(missURLs[idx]),
			Error: "web extraction provider returned no result",
		}
	}

	return cloneExtractResults(results), nil
}

func (p *cachedProvider) cachedSearch(key string) ([]SearchResult, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.search[key]
	if !ok {
		return nil, false
	}
	if !p.now().Before(entry.expiresAt) {
		delete(p.search, key)
		return nil, false
	}

	return cloneSearchResults(entry.results), true
}

func (p *cachedProvider) storeSearch(key string, results []SearchResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.search[key] = cachedSearch{
		expiresAt: p.now().Add(p.ttl),
		results:   cloneSearchResults(results),
	}
}

func (p *cachedProvider) cachedExtract(key string) (ExtractResult, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.extract[key]
	if !ok {
		return ExtractResult{}, false
	}
	if !p.now().Before(entry.expiresAt) {
		delete(p.extract, key)
		return ExtractResult{}, false
	}

	return entry.result, true
}

func (p *cachedProvider) storeExtract(key string, result ExtractResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.extract[key] = cachedExtract{
		expiresAt: p.now().Add(p.ttl),
		result:    result,
	}
}

func (p *cachedProvider) searchKey(query string, count int) string {
	return strings.Join([]string{
		p.providerName,
		strings.TrimSpace(query),
		strconv.Itoa(count),
	}, "\x00")
}

func (p *cachedProvider) extractKey(ctx context.Context, rawURL string) string {
	opts := ExtractOptionsFromContext(ctx)
	return strings.Join([]string{
		p.providerName,
		strings.TrimSpace(rawURL),
		opts.Format,
		strconv.Itoa(opts.MaxChars),
		opts.Query,
	}, "\x00")
}

func (p *cachedProvider) logCacheHit(operation string) {
	webLog.Info().
		Str("provider", p.providerName).
		Str("operation", operation).
		Msg("web provider cache hit")
}

func cloneSearchResults(results []SearchResult) []SearchResult {
	if results == nil {
		return nil
	}

	cloned := make([]SearchResult, len(results))
	copy(cloned, results)
	return cloned
}

func cloneExtractResults(results []ExtractResult) []ExtractResult {
	if results == nil {
		return nil
	}

	cloned := make([]ExtractResult, len(results))
	copy(cloned, results)
	return cloned
}

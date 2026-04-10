package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const firecrawlDefaultBaseURL = "https://api.firecrawl.dev"

type FirecrawlProvider struct {
	client            *httpClient
	maxCharsPerResult int
}

func NewFirecrawl(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" && opts.BaseURL == "" {
		return nil, errors.New("firecrawl requires web API key or base URL")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = firecrawlDefaultBaseURL
	}

	return &FirecrawlProvider{
		client: &httpClient{
			apiKey:  opts.APIKey,
			baseURL: opts.BaseURL,
			client:  http.DefaultClient,
		},
		maxCharsPerResult: maxCharPerResult(opts.MaxCharPerResult),
	}, nil
}

func (p *FirecrawlProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	type firecrawlResult struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Snippet     string `json:"snippet"`
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Web []firecrawlResult `json:"web"`
		} `json:"data"`
		Web []firecrawlResult `json:"web"`
	}

	if err := p.client.postJSON(ctx, "/v2/search", map[string]any{
		"query": query,
		"limit": count,
	}, p.client.authorizationHeaders(), &response); err != nil {
		return nil, err
	}

	rawResults := response.Data.Web
	if len(rawResults) == 0 {
		rawResults = response.Web
	}

	results := make([]SearchResult, 0, len(rawResults))
	for idx, result := range rawResults {
		results = append(results, SearchResult{
			Title:    strings.TrimSpace(result.Title),
			URL:      strings.TrimSpace(result.URL),
			Snippet:  truncateToMaxChars(firstNonEmpty(result.Description, result.Snippet), p.resolvedMaxCharsPerResult()),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (*FirecrawlProvider) Extract(context.Context, []string) ([]ExtractResult, error) {
	return nil, errProviderMethodNotImplemented
}

func (p *FirecrawlProvider) resolvedMaxCharsPerResult() int {
	if p == nil {
		return defaultMaxCharPerResult
	}

	return maxCharPerResult(p.maxCharsPerResult)
}

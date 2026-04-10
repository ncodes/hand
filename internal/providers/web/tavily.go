package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const tavilyDefaultBaseURL = "https://api.tavily.com"

type TavilyProvider struct {
	client            *httpClient
	maxCharsPerResult int
}

func NewTavily(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("tavily requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = tavilyDefaultBaseURL
	}

	return &TavilyProvider{
		client: &httpClient{
			apiKey:  opts.APIKey,
			baseURL: opts.BaseURL,
			client:  http.DefaultClient,
		},
		maxCharsPerResult: maxCharPerResult(opts.MaxCharPerResult),
	}, nil
}

func (p *TavilyProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	var response struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := p.client.postJSON(ctx, "/search", map[string]any{
		"query":               query,
		"search_depth":        "basic",
		"max_results":         count,
		"include_raw_content": false,
		"include_images":      false,
	}, p.client.authorizationHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(response.Results))
	for idx, result := range response.Results {
		results = append(results, SearchResult{
			Title:    strings.TrimSpace(result.Title),
			URL:      strings.TrimSpace(result.URL),
			Snippet:  truncateToMaxChars(result.Content, p.resolvedMaxCharsPerResult()),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (*TavilyProvider) Extract(context.Context, []string) ([]ExtractResult, error) {
	return nil, errProviderMethodNotImplemented
}

func (p *TavilyProvider) resolvedMaxCharsPerResult() int {
	if p == nil {
		return defaultMaxCharPerResult
	}

	return maxCharPerResult(p.maxCharsPerResult)
}

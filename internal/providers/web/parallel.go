package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const parallelDefaultBaseURL = "https://api.parallel.ai"

type ParallelProvider struct {
	client            *httpClient
	maxCharsPerResult int
}

func NewParallel(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("parallel requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = parallelDefaultBaseURL
	}

	return &ParallelProvider{
		client: &httpClient{
			apiKey:  opts.APIKey,
			baseURL: opts.BaseURL,
			client:  http.DefaultClient,
		},
		maxCharsPerResult: maxCharPerResult(opts.MaxCharPerResult),
	}, nil
}

func (p *ParallelProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	var response struct {
		Results []struct {
			Title    string   `json:"title"`
			URL      string   `json:"url"`
			Excerpts []string `json:"excerpts"`
			Snippet  string   `json:"snippet"`
		} `json:"results"`
	}

	if err := p.client.postJSON(ctx, "/v1beta/search", map[string]any{
		"search_queries": []string{query},
		"objective":      query,
		"max_results":    count,
		"excerpts": map[string]any{
			"max_chars_per_result": p.resolvedMaxCharsPerResult(),
		},
	}, p.parallelHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(response.Results))
	for idx, result := range response.Results {
		results = append(results, SearchResult{
			Title:    strings.TrimSpace(result.Title),
			URL:      strings.TrimSpace(result.URL),
			Snippet:  truncateToMaxChars(firstNonEmpty(strings.Join(result.Excerpts, " "), result.Snippet), p.resolvedMaxCharsPerResult()),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (*ParallelProvider) Extract(context.Context, []string) ([]ExtractResult, error) {
	return nil, errProviderMethodNotImplemented
}

func (p *ParallelProvider) parallelHeaders() map[string]string {
	if p == nil || p.client == nil || strings.TrimSpace(p.client.apiKey) == "" {
		return nil
	}

	return map[string]string{
		"x-api-key": strings.TrimSpace(p.client.apiKey),
	}
}

func (p *ParallelProvider) resolvedMaxCharsPerResult() int {
	if p == nil {
		return defaultMaxCharPerResult
	}

	return maxCharPerResult(p.maxCharsPerResult)
}

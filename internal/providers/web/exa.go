package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const exaDefaultBaseURL = "https://api.exa.ai"

type ExaProvider struct {
	client            *httpClient
	maxCharsPerResult int
}

func NewExa(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("exa requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = exaDefaultBaseURL
	}

	return &ExaProvider{
		client: &httpClient{
			apiKey:  opts.APIKey,
			baseURL: opts.BaseURL,
			client:  http.DefaultClient,
		},
		maxCharsPerResult: maxCharPerResult(opts.MaxCharPerResult),
	}, nil
}

func (p *ExaProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	var response struct {
		Results []struct {
			Title      string   `json:"title"`
			URL        string   `json:"url"`
			Text       string   `json:"text"`
			Summary    string   `json:"summary"`
			Highlights []string `json:"highlights"`
		} `json:"results"`
	}

	if err := p.client.postJSON(ctx, "/search", map[string]any{
		"query":      query,
		"numResults": count,
		"contents": map[string]any{
			"highlights": map[string]any{
				"maxCharacters": p.resolvedMaxCharsPerResult(),
			},
		},
	}, p.exaHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(response.Results))
	for idx, result := range response.Results {
		results = append(results, SearchResult{
			Title:    strings.TrimSpace(result.Title),
			URL:      strings.TrimSpace(result.URL),
			Snippet:  truncateToMaxChars(firstNonEmpty(firstHighlight(result.Highlights), result.Summary, result.Text), p.resolvedMaxCharsPerResult()),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (*ExaProvider) Extract(context.Context, []string) ([]ExtractResult, error) {
	return nil, errProviderMethodNotImplemented
}

func (p *ExaProvider) exaHeaders() map[string]string {
	if p == nil || p.client == nil || strings.TrimSpace(p.client.apiKey) == "" {
		return nil
	}

	return map[string]string{
		"x-api-key": strings.TrimSpace(p.client.apiKey),
	}
}

func (p *ExaProvider) resolvedMaxCharsPerResult() int {
	if p == nil {
		return defaultMaxCharPerResult
	}

	return maxCharPerResult(p.maxCharsPerResult)
}

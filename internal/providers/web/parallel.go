package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const parallelDefaultBaseURL = "https://api.parallel.ai"

type ParallelProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
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
		maxCharsPerResult:        opts.MaxCharPerResult,
		maxExtractCharsPerResult: opts.MaxExtractCharPerResult,
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
			"max_chars_per_result": p.maxCharsPerResult,
		},
	}, p.parallelHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(response.Results))
	for idx, result := range response.Results {
		results = append(results, SearchResult{
			Title:    strings.TrimSpace(result.Title),
			URL:      strings.TrimSpace(result.URL),
			Snippet:  truncateToMaxChars(firstNonEmpty(strings.Join(result.Excerpts, " "), result.Snippet), p.maxCharsPerResult),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *ParallelProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	var response struct {
		Results []struct {
			URL         string   `json:"url"`
			Title       string   `json:"title"`
			FullContent string   `json:"full_content"`
			Excerpts    []string `json:"excerpts"`
		} `json:"results"`
		Errors []struct {
			URL       string `json:"url"`
			Content   string `json:"content"`
			ErrorType string `json:"error_type"`
		} `json:"errors"`
	}

	if err := p.client.postJSON(ctx, "/v1beta/extract", map[string]any{
		"urls":         urls,
		"full_content": true,
	}, p.parallelHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]ExtractResult, 0, len(response.Results)+len(response.Errors))
	for _, result := range response.Results {
		content, truncated := truncateContent(firstNonEmpty(result.FullContent, strings.Join(result.Excerpts, "\n\n")),
			p.maxExtractCharsPerResult)
		results = append(results, ExtractResult{
			URL:           strings.TrimSpace(result.URL),
			Title:         strings.TrimSpace(result.Title),
			Content:       content,
			ContentFormat: "markdown",
			Truncated:     truncated,
		})
	}
	for _, result := range response.Errors {
		results = append(results, ExtractResult{
			URL:           strings.TrimSpace(result.URL),
			ContentFormat: "markdown",
			Error:         firstNonEmpty(result.Content, result.ErrorType, "extraction failed"),
		})
	}

	return results, nil
}

func (p *ParallelProvider) parallelHeaders() map[string]string {
	if p == nil || p.client == nil || strings.TrimSpace(p.client.apiKey) == "" {
		return nil
	}

	return map[string]string{
		"x-api-key": strings.TrimSpace(p.client.apiKey),
	}
}

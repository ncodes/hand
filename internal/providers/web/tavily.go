package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const tavilyDefaultBaseURL = "https://api.tavily.com"

type TavilyProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
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
		maxCharsPerResult:        opts.MaxCharPerResult,
		maxExtractCharsPerResult: opts.MaxExtractCharPerResult,
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
			Snippet:  truncateToMaxChars(result.Content, p.maxCharsPerResult),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *TavilyProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	var response struct {
		Results []struct {
			URL        string `json:"url"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			RawContent string `json:"raw_content"`
		} `json:"results"`
		FailedResults []struct {
			URL   string `json:"url"`
			Error string `json:"error"`
		} `json:"failed_results"`
		FailedURLs []string `json:"failed_urls"`
	}

	if err := p.client.postJSON(ctx, "/extract", map[string]any{
		"urls":             urls,
		"extract_depth":    "basic",
		"format":           "markdown",
		"include_images":   false,
		"include_raw_html": false,
	}, p.client.authorizationHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]ExtractResult, 0, len(response.Results)+len(response.FailedResults)+len(response.FailedURLs))
	for _, result := range response.Results {
		content, truncated := truncateContent(firstNonEmpty(result.RawContent, result.Content), p.maxExtractCharsPerResult)
		results = append(results, ExtractResult{
			URL:           strings.TrimSpace(result.URL),
			Title:         strings.TrimSpace(result.Title),
			Content:       content,
			ContentFormat: "markdown",
			Truncated:     truncated,
		})
	}
	for _, result := range response.FailedResults {
		results = append(results, ExtractResult{
			URL:           strings.TrimSpace(result.URL),
			ContentFormat: "markdown",
			Error:         firstNonEmpty(result.Error, "extraction failed"),
		})
	}
	for _, url := range response.FailedURLs {
		results = append(results, ExtractResult{
			URL:           strings.TrimSpace(url),
			ContentFormat: "markdown",
			Error:         "extraction failed",
		})
	}

	return results, nil
}

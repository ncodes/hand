package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const firecrawlDefaultBaseURL = "https://api.firecrawl.dev"

type FirecrawlProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
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
		maxCharsPerResult:        opts.MaxCharPerResult,
		maxExtractCharsPerResult: opts.MaxExtractCharPerResult,
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
			Snippet:  truncateToMaxChars(firstNonEmpty(result.Description, result.Snippet), p.maxCharsPerResult),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *FirecrawlProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	type scrapeMetadata struct {
		SourceURL string `json:"sourceURL"`
		Title     string `json:"title"`
	}

	type scrapeData struct {
		Markdown string         `json:"markdown"`
		HTML     string         `json:"html"`
		Metadata scrapeMetadata `json:"metadata"`
	}

	type scrapeResponse struct {
		Success bool       `json:"success"`
		Data    scrapeData `json:"data"`
	}

	results := make([]ExtractResult, 0, len(urls))
	for _, rawURL := range urls {
		url := strings.TrimSpace(rawURL)

		var response scrapeResponse
		err := p.client.postJSON(ctx, "/v2/scrape", map[string]any{
			"url": url,
			"formats": []string{
				"markdown",
			},
			"onlyMainContent": true,
			"parsers": []string{
				"pdf",
			},
		}, p.client.authorizationHeaders(), &response)
		if err != nil {
			results = append(results, ExtractResult{
				URL:           url,
				ContentFormat: "markdown",
				Error:         err.Error(),
			})
			continue
		}

		content, truncated := truncateContent(
			firstNonEmpty(response.Data.Markdown, response.Data.HTML),
			p.maxExtractCharsPerResult)
		results = append(results, ExtractResult{
			URL:           firstNonEmpty(response.Data.Metadata.SourceURL, url),
			Title:         strings.TrimSpace(response.Data.Metadata.Title),
			Content:       content,
			ContentFormat: "markdown",
			Truncated:     truncated,
		})
	}

	return results, nil
}

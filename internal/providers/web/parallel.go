package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

const parallelDefaultBaseURL = "https://api.parallel.ai"

// ParallelProvider fans web requests out to multiple providers.
type ParallelProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
	maxExtractResponseBytes  int
}

// NewParallel returns a provider that queries multiple web providers concurrently.
func NewParallel(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, providerCredentialError("parallel requires web API key")
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
		maxExtractResponseBytes:  opts.MaxExtractResponseBytes,
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
		stringValue1 := str.String(result.Title)
		stringValue2 := str.String(result.URL)
		results = append(results, SearchResult{
			Title:    stringValue1.Trim(),
			URL:      stringValue2.Trim(),
			Snippet:  truncateToMaxChars(getFirstNonEmpty(strings.Join(result.Excerpts, " "), result.Snippet), p.maxCharsPerResult),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *ParallelProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	format := getExtractFormat(ctx, "markdown")
	maxChars := getExtractCharLimit(ctx, p.maxExtractCharsPerResult)

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

	if err := p.client.postJSONLimited(ctx, "/v1beta/extract", map[string]any{
		"urls":         urls,
		"full_content": true,
	}, p.parallelHeaders(), &response, p.maxExtractResponseBytes); err != nil {
		return nil, err
	}

	results := make([]ExtractResult, 0, len(response.Results)+len(response.Errors))
	for _, result := range response.Results {
		content, truncated, downloadTruncated := limitExtractContent(
			getFirstNonEmpty(result.FullContent, strings.Join(result.Excerpts, "\n\n")),
			p.maxExtractResponseBytes,
			maxChars)
		stringValue3 := str.String(result.URL)
		stringValue4 := str.String(result.Title)
		results = append(results, ExtractResult{
			URL:               stringValue3.Trim(),
			Title:             stringValue4.Trim(),
			Content:           content,
			ContentFormat:     format,
			Truncated:         truncated,
			DownloadTruncated: downloadTruncated,
		})
	}
	for _, result := range response.Errors {
		stringValue5 := str.String(result.URL)
		results = append(results, ExtractResult{
			URL:           stringValue5.Trim(),
			ContentFormat: format,
			Error:         getFirstNonEmpty(result.Content, result.ErrorType, "extraction failed"),
		})
	}

	return results, nil
}

func (p *ParallelProvider) parallelHeaders() map[string]string {
	if p == nil || p.client == nil {
		return nil
	}
	apiKey := str.String(p.client.apiKey)
	if apiKey.Trim() == "" {
		return nil
	}
	return map[string]string{
		"x-api-key": apiKey.Trim(),
	}
}

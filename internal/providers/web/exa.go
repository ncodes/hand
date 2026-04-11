package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const exaDefaultBaseURL = "https://api.exa.ai"

type ExaProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
	maxExtractResponseBytes  int
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
		maxCharsPerResult:        opts.MaxCharPerResult,
		maxExtractCharsPerResult: opts.MaxExtractCharPerResult,
		maxExtractResponseBytes:  opts.MaxExtractResponseBytes,
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
				"maxCharacters": p.maxCharsPerResult,
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
			Snippet:  truncateToMaxChars(firstNonEmpty(firstHighlight(result.Highlights), result.Summary, result.Text), p.maxCharsPerResult),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *ExaProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	format := extractFormat(ctx, "text")
	maxChars := extractCharLimit(ctx, p.maxExtractCharsPerResult)

	var response struct {
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
			Text  string `json:"text"`
			Error string `json:"error"`
		} `json:"results"`
		Statuses []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  struct {
				Tag            string `json:"tag"`
				HTTPStatusCode int    `json:"httpStatusCode"`
			} `json:"error"`
		} `json:"statuses"`
	}

	if err := p.client.postJSON(ctx, "/contents", map[string]any{
		"urls": urls,
		"text": map[string]any{
			"maxCharacters": maxChars,
		},
	}, p.exaHeaders(), &response); err != nil {
		return nil, err
	}

	results := make([]ExtractResult, 0, len(response.Results))
	statusByURL := make(map[string]string, len(response.Statuses))
	for _, status := range response.Statuses {
		if strings.TrimSpace(status.Status) != "error" {
			continue
		}
		statusByURL[strings.TrimSpace(status.ID)] = exaStatusError(status.Error.Tag, status.Error.HTTPStatusCode)
	}

	seen := make(map[string]struct{}, len(response.Results))
	for _, result := range response.Results {
		url := strings.TrimSpace(result.URL)
		content, truncated, downloadTruncated := limitExtractContent(
			result.Text,
			p.maxExtractResponseBytes,
			maxChars)
		seen[url] = struct{}{}

		results = append(results, ExtractResult{
			URL:               url,
			Title:             strings.TrimSpace(result.Title),
			Content:           content,
			ContentFormat:     format,
			Truncated:         truncated,
			DownloadTruncated: downloadTruncated,
			Error:             firstNonEmpty(result.Error, statusByURL[url]),
		})
	}
	for _, status := range response.Statuses {
		url := strings.TrimSpace(status.ID)
		if _, ok := seen[url]; ok || strings.TrimSpace(status.Status) != "error" {
			continue
		}

		results = append(results, ExtractResult{
			URL:           url,
			ContentFormat: format,
			Error:         exaStatusError(status.Error.Tag, status.Error.HTTPStatusCode),
		})
	}

	return results, nil
}

func (p *ExaProvider) exaHeaders() map[string]string {
	if p == nil || p.client == nil || strings.TrimSpace(p.client.apiKey) == "" {
		return nil
	}

	return map[string]string{
		"x-api-key": strings.TrimSpace(p.client.apiKey),
	}
}

func exaStatusError(tag string, httpStatusCode int) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "extraction failed"
	}
	if httpStatusCode <= 0 {
		return tag
	}

	return tag + " (" + strconv.Itoa(httpStatusCode) + ")"
}

package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/wandxy/morph/pkg/stringx"
)

const exaDefaultBaseURL = "https://api.exa.ai"

// ExaProvider sends web requests to Exa.
type ExaProvider struct {
	client                   *httpClient
	maxCharsPerResult        int
	maxExtractCharsPerResult int
	maxExtractResponseBytes  int
}

// NewExa returns a web provider backed by Exa.
func NewExa(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, providerCredentialError("exa requires web API key")
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
			Title: stringx.String(result.Title).Trim(),
			URL:   stringx.String(result.URL).Trim(),
			Snippet: truncateToMaxChars(
				getFirstNonEmpty(getFirstHighlight(result.Highlights), result.Summary, result.Text),
				p.maxCharsPerResult,
			),
			Position: idx + 1,
		})
	}

	return results, nil
}

func (p *ExaProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	format := getExtractFormat(ctx, "text")
	maxChars := getExtractCharLimit(ctx, p.maxExtractCharsPerResult)
	query := getExtractQuery(ctx)

	var response struct {
		Results []struct {
			URL        string   `json:"url"`
			Title      string   `json:"title"`
			Text       string   `json:"text"`
			Highlights []string `json:"highlights"`
			Error      string   `json:"error"`
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

	payload := map[string]any{
		"urls": urls,
		"text": map[string]any{
			"maxCharacters": maxChars,
		},
	}
	if query != "" {
		payload["highlights"] = map[string]any{
			"query":         query,
			"maxCharacters": maxChars,
		}
	}

	if err := p.client.postJSONLimited(
		ctx,
		"/contents",
		payload, p.exaHeaders(),
		&response,
		p.maxExtractResponseBytes); err != nil {
		return nil, err
	}

	results := make([]ExtractResult, 0, len(response.Results))
	statusByURL := make(map[string]string, len(response.Statuses))
	for _, status := range response.Statuses {
		if stringx.String(status.Status).Trim() != "error" {
			continue
		}
		statusByURL[stringx.String(status.ID).Trim()] = getExaStatusError(status.Error.Tag, status.Error.HTTPStatusCode)
	}

	seen := make(map[string]struct{}, len(response.Results))
	for _, result := range response.Results {
		url := stringx.String(result.URL).Trim()

		content, truncated, downloadTruncated := limitExtractContent(
			getFirstNonEmpty(getFirstHighlight(result.Highlights), result.Text),
			p.maxExtractResponseBytes,
			maxChars)

		seen[url] = struct{}{}

		results = append(results, ExtractResult{
			URL:               url,
			Title:             stringx.String(result.Title).Trim(),
			Content:           content,
			ContentFormat:     format,
			Truncated:         truncated,
			DownloadTruncated: downloadTruncated,
			Error:             getFirstNonEmpty(result.Error, statusByURL[url]),
		})
	}
	for _, status := range response.Statuses {
		url := stringx.String(status.ID).Trim()
		if _, ok := seen[url]; ok || stringx.String(status.Status).Trim() != "error" {
			continue
		}

		results = append(results, ExtractResult{
			URL:           url,
			ContentFormat: format,
			Error:         getExaStatusError(status.Error.Tag, status.Error.HTTPStatusCode),
		})
	}

	return results, nil
}

func (p *ExaProvider) exaHeaders() map[string]string {
	if p == nil || p.client == nil || stringx.String(p.client.apiKey).Trim() == "" {
		return nil
	}

	return map[string]string{
		"x-api-key": stringx.String(p.client.apiKey).Trim(),
	}
}

func getExaStatusError(tag string, httpStatusCode int) string {
	tag = stringx.String(tag).Trim()
	if tag == "" {
		return "extraction failed"
	}
	if httpStatusCode <= 0 {
		return tag
	}

	return tag + " (" + strconv.Itoa(httpStatusCode) + ")"
}

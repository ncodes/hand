package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/wandxy/morph/pkg/str"
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
		stringValue1 := str.String(result.Title)
		stringValue2 := str.String(result.URL)
		results = append(results, SearchResult{
			Title: stringValue1.Trim(),
			URL:   stringValue2.Trim(),
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
		stringValue3 := str.String(status.Status)
		if stringValue3.Trim() != "error" {
			continue
		}
		statusID := str.String(status.ID)
		statusByURL[statusID.Trim()] = getExaStatusError(status.Error.Tag, status.Error.HTTPStatusCode)
	}

	seen := make(map[string]struct{}, len(response.Results))
	for _, result := range response.Results {
		stringValue4 := str.String(result.URL)
		url := stringValue4.Trim()

		content, truncated, downloadTruncated := limitExtractContent(
			getFirstNonEmpty(getFirstHighlight(result.Highlights), result.Text),
			p.maxExtractResponseBytes,
			maxChars)

		seen[url] = struct{}{}
		stringValue5 := str.String(result.Title)
		results = append(results, ExtractResult{
			URL:               url,
			Title:             stringValue5.Trim(),
			Content:           content,
			ContentFormat:     format,
			Truncated:         truncated,
			DownloadTruncated: downloadTruncated,
			Error:             getFirstNonEmpty(result.Error, statusByURL[url]),
		})
	}
	for _, status := range response.Statuses {
		stringValue6 := str.String(status.ID)
		url := stringValue6.Trim()
		stringValue7 := str.String(status.Status)
		if _, ok := seen[url]; ok || stringValue7.Trim() != "error" {
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

func getExaStatusError(tag string, httpStatusCode int) string {
	stringValue10 := str.String(tag)
	tag = stringValue10.Trim()
	if tag == "" {
		return "extraction failed"
	}
	if httpStatusCode <= 0 {
		return tag
	}

	return tag + " (" + strconv.Itoa(httpStatusCode) + ")"
}

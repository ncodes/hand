package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var errProviderMethodNotImplemented = errors.New("web provider method not implemented")

type httpClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (p *httpClient) postJSON(
	ctx context.Context,
	path string,
	payload any,
	headers map[string]string,
	target any,
) error {
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}

	baseURL := strings.TrimRight(strings.TrimSpace(p.baseURL), "/")
	if baseURL == "" {
		return errors.New("web provider base URL is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("web provider request failed: %s", message)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (p *httpClient) authorizationHeaders() map[string]string {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil
	}

	return map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(p.apiKey),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func firstHighlight(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

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

type httpClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type responseTooLargeError struct {
	Limit int
}

func (e responseTooLargeError) Error() string {
	return fmt.Sprintf("web provider response exceeds %d bytes", e.Limit)
}

func (p *httpClient) postJSON(
	ctx context.Context,
	path string,
	payload any,
	headers map[string]string,
	target any,
) error {
	return p.postJSONLimited(ctx, path, payload, headers, target, 0)
}

func (p *httpClient) postJSONLimited(
	ctx context.Context,
	path string,
	payload any,
	headers map[string]string,
	target any,
	maxResponseBytes int,
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

	if maxResponseBytes <= 0 {
		return json.NewDecoder(resp.Body).Decode(target)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxResponseBytes)+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBytes {
		return responseTooLargeError{Limit: maxResponseBytes}
	}

	return json.Unmarshal(data, target)
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

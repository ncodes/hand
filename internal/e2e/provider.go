package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

type ProviderRequest struct {
	Method string
	Path   string
	Body   string
}

type ProviderResponse struct {
	StatusCode int
	Headers    http.Header
	Body       string
}

type Provider struct {
	server   *httptest.Server
	response ProviderResponse
	mu       sync.Mutex
	requests []ProviderRequest
}

func NewProvider(resp ProviderResponse) *Provider {
	provider := &Provider{response: resp}
	provider.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		provider.mu.Lock()
		provider.requests = append(provider.requests, ProviderRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   string(body),
		})
		provider.mu.Unlock()

		for key, values := range resp.Headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		statusCode := resp.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, resp.Body)
	}))

	return provider
}

func (d *Provider) URL() string {
	if d == nil || d.server == nil {
		return ""
	}
	return d.server.URL
}

func (d *Provider) Close() {
	if d == nil || d.server == nil {
		return
	}
	d.server.Close()
}

func (d *Provider) Requests() []ProviderRequest {
	if d == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	requests := make([]ProviderRequest, len(d.requests))
	copy(requests, d.requests)
	return requests
}

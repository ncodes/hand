package web

import (
	"context"
	"errors"
	"net/http"
)

var errProviderMethodNotImplemented = errors.New("web provider method not implemented")

type HTTPProvider struct {
	Provider string
	APIKey   string
	BaseURL  string
	Client   *http.Client
}

func (p *HTTPProvider) Search(context.Context, string, int) ([]SearchResult, error) {
	return nil, errProviderMethodNotImplemented
}

func (p *HTTPProvider) Extract(context.Context, []string) ([]ExtractResult, error) {
	return nil, errProviderMethodNotImplemented
}

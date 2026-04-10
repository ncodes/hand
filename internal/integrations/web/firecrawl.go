package web

import (
	"errors"
	"net/http"
)

const firecrawlDefaultBaseURL = "https://api.firecrawl.dev"

func NewFirecrawl(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" && opts.BaseURL == "" {
		return nil, errors.New("firecrawl requires web API key or base URL")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = firecrawlDefaultBaseURL
	}

	return &HTTPProvider{
		Provider: ProviderFirecrawl,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Client:   http.DefaultClient,
	}, nil
}

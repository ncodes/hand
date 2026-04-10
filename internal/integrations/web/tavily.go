package web

import (
	"errors"
	"net/http"
)

const tavilyDefaultBaseURL = "https://api.tavily.com"

func NewTavily(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("tavily requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = tavilyDefaultBaseURL
	}

	return &HTTPProvider{
		Provider: ProviderTavily,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Client:   http.DefaultClient,
	}, nil
}

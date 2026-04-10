package web

import (
	"errors"
	"net/http"
)

const exaDefaultBaseURL = "https://api.exa.ai"

func NewExa(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("exa requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = exaDefaultBaseURL
	}

	return &HTTPProvider{
		Provider: ProviderExa,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Client:   http.DefaultClient,
	}, nil
}

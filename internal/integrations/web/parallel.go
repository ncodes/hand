package web

import (
	"errors"
	"net/http"
)

const parallelDefaultBaseURL = "https://api.parallel.ai"

func NewParallel(opts Options) (Provider, error) {
	opts = opts.Normalize()
	if opts.APIKey == "" {
		return nil, errors.New("parallel requires web API key")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = parallelDefaultBaseURL
	}

	return &HTTPProvider{
		Provider: ProviderParallel,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Client:   http.DefaultClient,
	}, nil
}

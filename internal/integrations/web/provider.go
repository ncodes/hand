package web

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
)

const (
	ProviderFirecrawl = "firecrawl"
	ProviderParallel  = "parallel"
	ProviderTavily    = "tavily"
	ProviderExa       = "exa"
)

var (
	ErrProviderNotConfigured = errors.New("web provider is not configured")
	ErrUnsupportedProvider   = errors.New("unsupported web provider")
)

type SearchResult struct {
	Title    string
	URL      string
	Snippet  string
	Position int
}

type ExtractResult struct {
	URL           string
	Title         string
	Content       string
	ContentFormat string
	Truncated     bool
	Error         string
}

type Provider interface {
	Search(context.Context, string, int) ([]SearchResult, error)
	Extract(context.Context, []string) ([]ExtractResult, error)
}

type Options struct {
	Provider string
	APIKey   string
	BaseURL  string
}

func (o Options) Normalize() Options {
	o.Provider = strings.TrimSpace(strings.ToLower(o.Provider))
	o.APIKey = strings.TrimSpace(o.APIKey)
	o.BaseURL = strings.TrimSpace(o.BaseURL)
	return o
}

func SupportedProvider(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case ProviderFirecrawl, ProviderParallel, ProviderTavily, ProviderExa:
		return true
	default:
		return false
	}
}

func ResolveOptions(cfg *config.Config) (Options, error) {
	var opts Options
	if cfg != nil {
		opts = Options{
			Provider: cfg.WebProvider,
			APIKey:   cfg.WebAPIKey,
			BaseURL:  cfg.WebBaseURL,
		}.Normalize()
	}

	if opts.Provider != "" && !SupportedProvider(opts.Provider) {
		return Options{}, ErrUnsupportedProvider
	}

	if opts.Provider == "" {
		return Options{}, ErrProviderNotConfigured
	}

	opts = fillProviderDefaults(opts)
	return opts, nil
}

func fillProviderDefaults(opts Options) Options {
	opts = opts.Normalize()

	switch opts.Provider {
	case ProviderFirecrawl:
		if opts.BaseURL == "" {
			opts.BaseURL = firecrawlDefaultBaseURL
		}
	case ProviderParallel:
		if opts.BaseURL == "" {
			opts.BaseURL = parallelDefaultBaseURL
		}
	case ProviderTavily:
		if opts.BaseURL == "" {
			opts.BaseURL = tavilyDefaultBaseURL
		}
	case ProviderExa:
		if opts.BaseURL == "" {
			opts.BaseURL = exaDefaultBaseURL
		}
	}

	return opts.Normalize()
}

func NewProvider(cfg *config.Config) (Provider, error) {
	opts, err := ResolveOptions(cfg)
	if err != nil {
		return nil, err
	}

	switch opts.Provider {
	case ProviderFirecrawl:
		return NewFirecrawl(opts)
	case ProviderParallel:
		return NewParallel(opts)
	case ProviderTavily:
		return NewTavily(opts)
	case ProviderExa:
		return NewExa(opts)
	default:
		return nil, ErrUnsupportedProvider
	}
}

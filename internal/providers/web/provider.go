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
	Provider                string
	APIKey                  string
	BaseURL                 string
	MaxCharPerResult        int
	MaxExtractCharPerResult int
}

func (o Options) Normalize() Options {
	o.Provider = strings.TrimSpace(strings.ToLower(o.Provider))
	o.APIKey = strings.TrimSpace(o.APIKey)
	o.BaseURL = strings.TrimSpace(o.BaseURL)
	if o.MaxCharPerResult < 0 {
		o.MaxCharPerResult = 0
	}
	if o.MaxExtractCharPerResult < 0 {
		o.MaxExtractCharPerResult = 0
	}
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
		normalized := *cfg
		normalized.Normalize()

		opts = Options{
			Provider:                normalized.WebProvider,
			APIKey:                  normalized.WebAPIKey,
			BaseURL:                 normalized.WebBaseURL,
			MaxCharPerResult:        normalized.WebMaxCharPerResult,
			MaxExtractCharPerResult: normalized.WebMaxExtractCharPerResult,
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

func truncateToMaxChars(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if maxChars <= 0 {
		return value
	}

	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}

	return strings.TrimSpace(string(runes[:maxChars]))
}

func truncateContent(value string, maxChars int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if maxChars <= 0 {
		return value, false
	}

	runes := []rune(value)
	if len(runes) <= maxChars {
		return value, false
	}

	return strings.TrimSpace(string(runes[:maxChars])), true
}

func NewProvider(cfg *config.Config) (Provider, error) {
	opts, err := ResolveOptions(cfg)
	if err != nil {
		return nil, err
	}

	return newProvider(opts)
}

func newProvider(opts Options) (Provider, error) {
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

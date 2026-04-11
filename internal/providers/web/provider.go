package web

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

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
	URL               string `json:"url"`
	Title             string `json:"title,omitempty"`
	Content           string `json:"content,omitempty"`
	ContentFormat     string `json:"content_format"`
	Truncated         bool   `json:"truncated,omitempty"`
	DownloadTruncated bool   `json:"download_truncated,omitempty"`
	Error             string `json:"error,omitempty"`
}

type Provider interface {
	Search(context.Context, string, int) ([]SearchResult, error)
	Extract(context.Context, []string) ([]ExtractResult, error)
}

type extractOptionsContextKey struct{}

type ExtractOptions struct {
	Format   string
	MaxChars int
	Query    string
}

type Options struct {
	Provider                string
	APIKey                  string
	BaseURL                 string
	MaxCharPerResult        int
	MaxExtractCharPerResult int
	MaxExtractResponseBytes int
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
	if o.MaxExtractResponseBytes < 0 {
		o.MaxExtractResponseBytes = 0
	}
	return o
}

func WithExtractOptions(ctx context.Context, opts ExtractOptions) context.Context {
	return context.WithValue(ctx, extractOptionsContextKey{}, opts.Normalize())
}

func ExtractOptionsFromContext(ctx context.Context) ExtractOptions {
	if ctx == nil {
		return ExtractOptions{}
	}

	opts, _ := ctx.Value(extractOptionsContextKey{}).(ExtractOptions)
	return opts.Normalize()
}

func (o ExtractOptions) Normalize() ExtractOptions {
	o.Format = strings.TrimSpace(strings.ToLower(o.Format))
	o.Query = strings.TrimSpace(o.Query)
	if o.Format != "text" && o.Format != "markdown" {
		o.Format = ""
	}
	if o.MaxChars < 0 {
		o.MaxChars = 0
	}

	return o
}

func extractCharLimit(ctx context.Context, configuredMax int) int {
	opts := ExtractOptionsFromContext(ctx)
	if opts.MaxChars > 0 {
		return opts.MaxChars
	}

	return configuredMax
}

func extractFormat(ctx context.Context, defaultFormat string) string {
	if format := ExtractOptionsFromContext(ctx).Format; format != "" {
		return format
	}

	return defaultFormat
}

func extractQuery(ctx context.Context) string {
	return ExtractOptionsFromContext(ctx).Query
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
			MaxExtractResponseBytes: normalized.WebMaxExtractResponseBytes,
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

func limitExtractContent(value string, maxBytes, maxChars int) (string, bool, bool) {
	content, downloadTruncated := truncateToMaxBytes(value, maxBytes)
	content, charTruncated := truncateContent(content, maxChars)

	return content, downloadTruncated || charTruncated, downloadTruncated
}

func isResponseTooLarge(err error) bool {
	var tooLarge responseTooLargeError
	return errors.As(err, &tooLarge)
}

func truncateToMaxBytes(value string, maxBytes int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || maxBytes <= 0 {
		return value, false
	}
	if len([]byte(value)) <= maxBytes {
		return value, false
	}

	data := []byte(value)
	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}

	return strings.TrimSpace(string(data)), true
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

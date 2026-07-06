package web

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/pkg/str"
)

const (
	ProviderFirecrawl = "firecrawl"
	ProviderParallel  = "parallel"
	ProviderTavily    = "tavily"
	ProviderExa       = "exa"
	ProviderNative    = "native"
)

var (
	ErrProviderNotConfigured = errors.New("web provider is not configured")
	ErrUnsupportedProvider   = errors.New("unsupported web provider")
	ErrProviderCredential    = errors.New("web provider credential is required")
)

type providerCredentialError string

func (e providerCredentialError) Error() string {
	return string(e)
}

func (e providerCredentialError) Is(target error) bool {
	return target == ErrProviderCredential
}

// SearchResult contains matches returned by a search request.
type SearchResult struct {
	Title    string
	URL      string
	Snippet  string
	Position int
}

// ExtractResult contains readable content extracted from a web page.
type ExtractResult struct {
	URL                string `json:"url"`
	Title              string `json:"title,omitempty"`
	Content            string `json:"content,omitempty"`
	ContentFormat      string `json:"content_format"`
	Truncated          bool   `json:"truncated,omitempty"`
	DownloadTruncated  bool   `json:"download_truncated,omitempty"`
	Summarized         bool   `json:"summarized,omitempty"`
	SummaryRefused     bool   `json:"summary_refused,omitempty"`
	SourceContentChars int    `json:"source_content_chars,omitempty"`
	SummaryChars       int    `json:"summary_chars,omitempty"`
	Error              string `json:"error,omitempty"`
}

// Provider searches the web and extracts readable page content.
type Provider interface {
	Search(context.Context, string, int) ([]SearchResult, error)
	Extract(context.Context, []string) ([]ExtractResult, error)
}

type extractOptionsContextKey struct{}

// ExtractOptions controls format, size limits, query context, and website policy for extraction.
type ExtractOptions struct {
	Format        string
	MaxChars      int
	Query         string
	WebsitePolicy guardrails.WebsitePolicy
}

// Options configures the selected web provider and its search/extraction limits.
type Options struct {
	Provider                string
	APIKey                  string
	BaseURL                 string
	MaxCharPerResult        int
	MaxExtractCharPerResult int
	MaxExtractResponseBytes int
	NativeAllowedHosts      []string
	NativeBlockedHosts      []string
	NativeAllowedHostFiles  []string
	NativeBlockedHostFiles  []string
}

func (o Options) Normalize() Options {
	stringValue1 := str.String(o.Provider)
	o.Provider = stringValue1.Normalized()
	stringValue2 := str.String(o.APIKey)
	o.APIKey = stringValue2.Trim()
	stringValue3 := str.String(o.BaseURL)
	o.BaseURL = stringValue3.Trim()
	if o.MaxCharPerResult < 0 {
		o.MaxCharPerResult = 0
	}
	if o.MaxExtractCharPerResult < 0 {
		o.MaxExtractCharPerResult = 0
	}
	if o.MaxExtractResponseBytes < 0 {
		o.MaxExtractResponseBytes = 0
	}
	o.NativeAllowedHosts = dedupeTrimValues(o.NativeAllowedHosts)
	o.NativeBlockedHosts = dedupeTrimValues(o.NativeBlockedHosts)
	o.NativeAllowedHostFiles = dedupeTrimValues(o.NativeAllowedHostFiles)
	o.NativeBlockedHostFiles = dedupeTrimValues(o.NativeBlockedHostFiles)
	return o
}

// WithExtractOptions describes extract options on ctx.
func WithExtractOptions(ctx context.Context, opts ExtractOptions) context.Context {
	return context.WithValue(ctx, extractOptionsContextKey{}, opts.Normalize())
}

// ExtractOptionsFromContext returns extract options stored on ctx.
func ExtractOptionsFromContext(ctx context.Context) ExtractOptions {
	if ctx == nil {
		return ExtractOptions{}
	}

	opts, _ := ctx.Value(extractOptionsContextKey{}).(ExtractOptions)
	return opts.Normalize()
}

func (o ExtractOptions) Normalize() ExtractOptions {
	stringValue4 := str.String(o.Format)
	o.Format = stringValue4.Normalized()
	stringValue5 := str.String(o.Query)
	o.Query = stringValue5.Trim()
	if o.Format != "text" && o.Format != "markdown" {
		o.Format = ""
	}
	if o.MaxChars < 0 {
		o.MaxChars = 0
	}

	return o
}

func getExtractCharLimit(ctx context.Context, configuredMax int) int {
	opts := ExtractOptionsFromContext(ctx)
	if opts.MaxChars > 0 {
		return opts.MaxChars
	}

	return configuredMax
}

func getExtractFormat(ctx context.Context, defaultFormat string) string {
	if format := ExtractOptionsFromContext(ctx).Format; format != "" {
		return format
	}

	return defaultFormat
}

func getExtractQuery(ctx context.Context) string {
	return ExtractOptionsFromContext(ctx).Query
}

func getExtractWebsitePolicy(ctx context.Context) guardrails.WebsitePolicy {
	return ExtractOptionsFromContext(ctx).WebsitePolicy
}

// SupportedProvider reports whether supported provider is supported.
func SupportedProvider(name string) bool {
	stringValue6 := str.String(name)
	switch stringValue6.Normalized() {
	case ProviderFirecrawl, ProviderParallel, ProviderTavily, ProviderExa, ProviderNative:
		return true
	default:
		return false
	}
}

// ResolveOptions resolves options.
func ResolveOptions(cfg *config.Config) (Options, error) {
	var opts Options
	if cfg != nil {
		normalized := *cfg
		normalized.Normalize()

		apiKey, err := normalized.WebAPIKeyEffective()
		if err != nil {
			return Options{}, err
		}

		opts = Options{
			Provider:                normalized.Web.Provider,
			APIKey:                  apiKey,
			BaseURL:                 normalized.Web.BaseURL,
			MaxCharPerResult:        normalized.Web.MaxCharPerResult,
			MaxExtractCharPerResult: normalized.Web.MaxExtractCharPerResult,
			MaxExtractResponseBytes: normalized.Web.MaxExtractResponseBytes,
			NativeAllowedHosts:      normalized.Web.NativeAllowedHosts,
			NativeBlockedHosts:      normalized.Web.NativeBlockedHosts,
			NativeAllowedHostFiles:  normalized.Web.NativeAllowedHostFiles,
			NativeBlockedHostFiles:  normalized.Web.NativeBlockedHostFiles,
		}.Normalize()
	}

	if opts.Provider != "" && !SupportedProvider(opts.Provider) {
		return Options{}, ErrUnsupportedProvider
	}

	if opts.Provider == "" {
		opts.Provider = ProviderNative
	}

	opts = applyProviderDefaults(opts)
	return opts, nil
}

func applyProviderDefaults(opts Options) Options {
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
	case ProviderNative:
	}

	return opts.Normalize()
}

func dedupeTrimValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		stringValue7 := str.String(value)
		value = stringValue7.Trim()
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	return normalized
}

func truncateToMaxChars(value string, maxChars int) string {
	stringValue8 := str.String(value)
	value = stringValue8.Trim()
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
	stringValue9 := str.String(string(runes[:maxChars]))
	return stringValue9.Trim()
}

func truncateContent(value string, maxChars int) (string, bool) {
	stringValue10 := str.String(value)
	value = stringValue10.Trim()
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
	stringValue11 := str.String(string(runes[:maxChars]))
	return stringValue11.Trim(), true
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
	stringValue12 := str.String(value)
	value = stringValue12.Trim()
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
	stringValue13 := str.String(string(data))
	return stringValue13.Trim(), true
}

// NewProvider returns a provider selected from config.
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
	case ProviderNative:
		return NewNative(opts)
	default:
		return nil, ErrUnsupportedProvider
	}
}

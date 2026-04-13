package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html"
)

const (
	nativeDefaultTimeout = 15 * time.Second
	nativeUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
)

var errNativeSearchUnsupported = errors.New("native web provider does not support search")

var nativeReadabilityFromReader = readability.FromReader
var nativeDefaultHTTPClient = func(provider *NativeProvider) *http.Client {
	return provider.newHTTPClient()
}
var nativeDefaultDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, address)
}

var nativeBlockedAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

type NativeProvider struct {
	client                   *http.Client
	makeClient               func() *http.Client
	newRequest               func(context.Context, string, string, io.Reader) (*http.Request, error)
	dial                     func(context.Context, string, string) (net.Conn, error)
	resolveHost              func(context.Context, string) ([]netip.Addr, error)
	maxExtractCharsPerResult int
	maxExtractResponseBytes  int
}

func NewNative(opts Options) (Provider, error) {
	opts = opts.Normalize()
	provider := &NativeProvider{
		resolveHost: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		maxExtractCharsPerResult: opts.MaxExtractCharPerResult,
		maxExtractResponseBytes:  opts.MaxExtractResponseBytes,
	}
	provider.client = provider.newHTTPClient()

	return provider, nil
}

func (p *NativeProvider) Search(context.Context, string, int) ([]SearchResult, error) {
	return nil, errNativeSearchUnsupported
}

func (p *NativeProvider) Extract(ctx context.Context, urls []string) ([]ExtractResult, error) {
	format := extractFormat(ctx, "text")
	maxChars := extractCharLimit(ctx, p.maxExtractCharsPerResult)
	results := make([]ExtractResult, 0, len(urls))

	for _, rawURL := range urls {
		result := p.extract(ctx, strings.TrimSpace(rawURL), format, maxChars)
		results = append(results, result)
	}

	return results, nil
}

func (p *NativeProvider) extract(ctx context.Context, rawURL, format string, maxChars int) ExtractResult {
	result := ExtractResult{URL: rawURL, ContentFormat: format}

	validatedURL, err := p.validateURL(ctx, rawURL)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	newRequest := p.newRequest
	if newRequest == nil {
		newRequest = http.NewRequestWithContext
	}
	req, err := newRequest(ctx, http.MethodGet, validatedURL.String(), nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Accept", "text/html,text/plain;q=0.9")
	req.Header.Set("User-Agent", nativeUserAgent)

	client := p.client
	if client == nil {
		makeClient := p.makeClient
		if makeClient == nil {
			makeClient = func() *http.Client {
				return nativeDefaultHTTPClient(p)
			}
		}
		client = makeClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	extractionURL := validatedURL
	if resp.Request != nil && resp.Request.URL != nil {
		extractionURL = resp.Request.URL
		result.URL = extractionURL.String()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("web extraction request failed: %s", resp.Status)
		return result
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != "" && mediaType != "text/html" && mediaType != "application/xhtml+xml" && mediaType != "text/plain" {
		result.Error = "unsupported content type: " + mediaType
		return result
	}

	data, downloadTruncated, err := readNativeResponse(resp.Body, p.maxExtractResponseBytes)
	if err != nil {
		result.Error = err.Error()
		result.DownloadTruncated = downloadTruncated
		return result
	}

	var extracted nativeDocument
	if mediaType == "text/plain" {
		extracted.Content = strings.TrimSpace(string(data))
	} else {
		extracted, err = extractNativeHTML(data, extractionURL, format)
		if err != nil {
			result.Error = err.Error()
			return result
		}
	}

	content, truncated := truncateContent(extracted.Content, maxChars)
	result.Title = extracted.Title
	result.Content = content
	result.Truncated = truncated || downloadTruncated
	result.DownloadTruncated = downloadTruncated

	return result
}

func (p *NativeProvider) newHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext:           p.dialContext,
	}

	return &http.Client{
		Timeout:   nativeDefaultTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			_, err := p.validateURL(req.Context(), req.URL.String())
			return err
		},
	}
}

func (p *NativeProvider) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addrs, err := p.resolveAndValidateHost(ctx, host)
	if err != nil {
		return nil, err
	}

	dial := p.dial
	if dial == nil {
		dial = nativeDefaultDialContext
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dial(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (p *NativeProvider) validateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("url scheme must be http or https")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, errors.New("url host is required")
	}
	if parsed.User != nil {
		return nil, errors.New("url userinfo is not allowed")
	}
	if block, blocked := extractWebsitePolicy(ctx).Check(parsed.String()); blocked {
		return nil, errors.New(block.Message)
	}
	if _, err := p.resolveAndValidateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}

	return parsed, nil
}

func (p *NativeProvider) resolveAndValidateHost(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("url host is required")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if !safeNativeAddr(addr) {
			return nil, errors.New("url host resolves to a blocked address")
		}
		return []netip.Addr{addr}, nil
	}

	resolveHost := p.resolveHost
	if resolveHost == nil {
		resolveHost = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	addrs, err := resolveHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("url host resolved to no addresses")
	}
	for _, addr := range addrs {
		if !safeNativeAddr(addr) {
			return nil, errors.New("url host resolves to a blocked address")
		}
	}

	return addrs, nil
}

func safeNativeAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() ||
		!addr.IsGlobalUnicast() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}

	for _, prefix := range nativeBlockedAddressPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}

	return true
}

func readNativeResponse(body io.Reader, maxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		data, err := io.ReadAll(body)
		return data, false, err
	}

	data, err := io.ReadAll(io.LimitReader(body, int64(maxBytes)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
		for len(data) > 0 && !utf8.Valid(data) {
			data = data[:len(data)-1]
		}

		return data, true, nil
	}

	return data, false, nil
}

type nativeDocument struct {
	Title   string
	Content string
}

func extractNativeHTML(data []byte, pageURL *url.URL, format string) (nativeDocument, error) {
	article, err := nativeReadabilityFromReader(bytes.NewReader(data), pageURL)
	if err != nil {
		return nativeDocument{}, err
	}

	var content strings.Builder
	if format == "markdown" {
		content.WriteString(renderNativeMarkdown(article.Node))
	} else {
		err = article.RenderText(&content)
	}
	if err != nil {
		return nativeDocument{}, err
	}

	return nativeDocument{
		Title:   strings.TrimSpace(article.Title()),
		Content: strings.TrimSpace(content.String()),
	}, nil
}

func renderNativeMarkdown(root *html.Node) string {
	lines := make([]string, 0, 16)
	appendLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			return
		}
		lines = append(lines, line)
	}

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}

		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				text := collectNativeText(node)
				if text != "" {
					appendLine("")
					appendLine(strings.Repeat("#", nativeHeadingLevel(node.Data)) + " " + text)
					appendLine("")
				}
				return
			case "p", "blockquote":
				text := collectNativeText(node)
				if text != "" {
					appendLine(text)
					appendLine("")
				}
				return
			case "li":
				text := collectNativeText(node)
				if text != "" {
					appendLine("- " + text)
				}
				return
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(root)

	return strings.TrimSpace(strings.Join(compactNativeLines(lines), "\n"))
}

func collectNativeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return strings.Join(strings.Fields(node.Data), " ")
	}

	parts := make([]string, 0, 4)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if text := collectNativeText(child); text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " ")
}

func compactNativeLines(lines []string) []string {
	compacted := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(compacted) == 0 || blank {
				continue
			}
			blank = true
			compacted = append(compacted, "")
			continue
		}
		blank = false
		compacted = append(compacted, line)
	}
	for len(compacted) > 0 && compacted[len(compacted)-1] == "" {
		compacted = compacted[:len(compacted)-1]
	}

	return compacted
}

func nativeHeadingLevel(name string) int {
	switch strings.ToLower(name) {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	default:
		return 6
	}
}

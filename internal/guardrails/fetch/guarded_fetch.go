package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const defaultDialTimeout = 10 * time.Second

type ResolveHostFunc func(context.Context, string) ([]netip.Addr, error)

type DialFunc func(context.Context, string, string) (net.Conn, error)

type Policy interface {
	Check(context.Context, *url.URL) error
}

type GuardedFetcher struct {
	ResolveHost ResolveHostFunc
	Dial        DialFunc
	Policy      Policy
}

func (f *GuardedFetcher) NewHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   defaultDialTimeout,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext:           f.DialContext,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}

			_, err := f.ValidateURL(req.Context(), req.URL.String())
			return err
		},
	}
}

func (f *GuardedFetcher) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addrs, err := f.ResolveAndValidateHost(ctx, host)
	if err != nil {
		return nil, err
	}

	dial := f.Dial
	if dial == nil {
		dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: defaultDialTimeout}
			return dialer.DialContext(ctx, network, address)
		}
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

func (f *GuardedFetcher) ValidateURL(ctx context.Context, rawURL string) (*url.URL, error) {
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

	if f.Policy != nil {
		if err := f.Policy.Check(ctx, parsed); err != nil {
			return nil, err
		}
	}

	if _, err := f.ResolveAndValidateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}

	return parsed, nil
}

func (f *GuardedFetcher) ResolveAndValidateHost(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("url host is required")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if !SafeAddr(addr) {
			return nil, errors.New("url host resolves to a blocked address")
		}

		return []netip.Addr{addr}, nil
	}

	resolveHost := f.ResolveHost
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
		if !SafeAddr(addr) {
			return nil, errors.New("url host resolves to a blocked address")
		}
	}

	return addrs, nil
}

func SafeAddr(addr netip.Addr) bool {
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

	for _, prefix := range blockedAddressPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}

	return true
}

var blockedAddressPrefixes = []netip.Prefix{
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

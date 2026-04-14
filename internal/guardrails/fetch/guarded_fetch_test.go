package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubPolicy struct {
	check func(context.Context, *url.URL) error
}

func (p stubPolicy) Check(ctx context.Context, parsed *url.URL) error {
	if p.check == nil {
		return nil
	}

	return p.check(ctx, parsed)
}

func TestGuardedFetcher_ValidateURLRejectsInvalidInputs(t *testing.T) {
	fetcher := &GuardedFetcher{}

	_, err := fetcher.ValidateURL(context.Background(), "%")
	require.Error(t, err)

	_, err = fetcher.ValidateURL(context.Background(), "file:///etc/passwd")
	require.EqualError(t, err, "url scheme must be http or https")

	_, err = fetcher.ValidateURL(context.Background(), "https:///missing-host")
	require.EqualError(t, err, "url host is required")

	_, err = fetcher.ValidateURL(context.Background(), "https://user@example.com/page")
	require.EqualError(t, err, "url userinfo is not allowed")
}

func TestGuardedFetcher_ValidateURLAppliesHostAndURLPolicies(t *testing.T) {
	fetcher := &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		Policy: stubPolicy{
			check: func(_ context.Context, parsed *url.URL) error {
				require.Equal(t, "blocked.example", parsed.Hostname())
				return errors.New("blocked by host policy")
			},
		},
	}

	_, err := fetcher.ValidateURL(context.Background(), "https://blocked.example/page")
	require.EqualError(t, err, "blocked by host policy")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		Policy: stubPolicy{
			check: func(_ context.Context, parsed *url.URL) error {
				require.Equal(t, "https://example.com/page", parsed.String())
				return errors.New("blocked by url policy")
			},
		},
	}

	_, err = fetcher.ValidateURL(context.Background(), "https://example.com/page")
	require.EqualError(t, err, "blocked by url policy")
}

func TestGuardedFetcher_ValidateURLReturnsResolverErrorAndSuccess(t *testing.T) {
	fetcher := &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("resolver failed")
		},
	}

	_, err := fetcher.ValidateURL(context.Background(), "https://example.com/page")
	require.EqualError(t, err, "resolver failed")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
	}

	parsed, err := fetcher.ValidateURL(context.Background(), "https://example.com/page")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/page", parsed.String())
}

func TestGuardedFetcher_ResolveAndValidateHostHandlesLiteralResolverAndBlockedAddresses(t *testing.T) {
	fetcher := &GuardedFetcher{}

	_, err := fetcher.ResolveAndValidateHost(context.Background(), " ")
	require.EqualError(t, err, "url host is required")

	addrs, err := fetcher.ResolveAndValidateHost(context.Background(), "93.184.216.34")
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("93.184.216.34")}, addrs)

	_, err = fetcher.ResolveAndValidateHost(context.Background(), "127.0.0.1")
	require.EqualError(t, err, "url host resolves to a blocked address")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("resolver failed")
		},
	}

	_, err = fetcher.ResolveAndValidateHost(context.Background(), "example.com")
	require.EqualError(t, err, "resolver failed")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return nil, nil
		},
	}

	_, err = fetcher.ResolveAndValidateHost(context.Background(), "example.com")
	require.EqualError(t, err, "url host resolved to no addresses")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
		},
	}

	_, err = fetcher.ResolveAndValidateHost(context.Background(), "example.com")
	require.EqualError(t, err, "url host resolves to a blocked address")
}

func TestGuardedFetcher_DialContextHandlesErrorsAndSuccess(t *testing.T) {
	fetcher := &GuardedFetcher{}

	_, err := fetcher.DialContext(context.Background(), "tcp", "bad-address")
	require.Error(t, err)

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("resolver failed")
		},
	}

	_, err = fetcher.DialContext(context.Background(), "tcp", "example.com:443")
	require.EqualError(t, err, "resolver failed")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		},
	}

	_, err = fetcher.DialContext(context.Background(), "tcp", "example.com:443")
	require.EqualError(t, err, "dial failed")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("93.184.216.35"),
			}, nil
		},
		Dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address == "93.184.216.34:443" {
				return nil, errors.New("first failed")
			}

			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() {
				_ = serverConn.Close()
			})
			return clientConn, nil
		},
	}

	conn, err := fetcher.DialContext(context.Background(), "tcp", "example.com:443")
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())
}

func TestGuardedFetcher_DialContextUsesDefaultDialer(t *testing.T) {
	fetcher := &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
	}

	_, err := fetcher.DialContext(context.Background(), "tcp", "example.com:1")
	require.Error(t, err)
}

func TestGuardedFetcher_DialContextReturnsLastErrorAfterAllAddressesFail(t *testing.T) {
	fetcher := &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("93.184.216.35"),
			}, nil
		},
		Dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address == "93.184.216.34:443" {
				return nil, errors.New("first failed")
			}

			return nil, errors.New("second failed")
		},
	}

	_, err := fetcher.DialContext(context.Background(), "tcp", "example.com:443")
	require.EqualError(t, err, "second failed")
}

func TestGuardedFetcher_NewHTTPClientCheckRedirect(t *testing.T) {
	fetcher := &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
	}

	client := fetcher.NewHTTPClient(time.Second)
	req, err := http.NewRequest(http.MethodGet, "https://example.com/page", nil)
	require.NoError(t, err)

	via := []*http.Request{
		mustNewRequest(t, "https://example.com/1"),
		mustNewRequest(t, "https://example.com/2"),
		mustNewRequest(t, "https://example.com/3"),
		mustNewRequest(t, "https://example.com/4"),
		mustNewRequest(t, "https://example.com/5"),
	}

	err = client.CheckRedirect(req, via)
	require.EqualError(t, err, "too many redirects")

	fetcher = &GuardedFetcher{
		ResolveHost: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		Policy: stubPolicy{
			check: func(_ context.Context, parsed *url.URL) error {
				require.Equal(t, "https://example.com/page", parsed.String())
				return errors.New("blocked redirect")
			},
		},
	}

	client = fetcher.NewHTTPClient(time.Second)
	err = client.CheckRedirect(req, nil)
	require.EqualError(t, err, "blocked redirect")
}

func TestGuardedFetcher_ResolveAndValidateHostUsesDefaultResolver(t *testing.T) {
	fetcher := &GuardedFetcher{}

	addrs, err := fetcher.ResolveAndValidateHost(context.Background(), "localhost")
	require.Error(t, err)
	require.Nil(t, addrs)
}

func TestSafeAddr(t *testing.T) {
	require.True(t, SafeAddr(netip.MustParseAddr("93.184.216.34")))
	require.False(t, SafeAddr(netip.MustParseAddr("127.0.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("10.0.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("100.64.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("169.254.169.254")))
	require.False(t, SafeAddr(netip.MustParseAddr("192.0.2.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("198.18.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("203.0.113.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("240.0.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("::1")))
	require.False(t, SafeAddr(netip.MustParseAddr("::ffff:127.0.0.1")))
	require.False(t, SafeAddr(netip.MustParseAddr("64:ff9b::1")))
	require.False(t, SafeAddr(netip.MustParseAddr("64:ff9b:1::1")))
	require.False(t, SafeAddr(netip.MustParseAddr("100::1")))
	require.False(t, SafeAddr(netip.MustParseAddr("2001:db8::1")))
	require.False(t, SafeAddr(netip.MustParseAddr("2002::1")))
	require.False(t, SafeAddr(netip.Addr{}))
}

func mustNewRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)

	return req
}

package guardrails

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeAddr_UsesDefaultBlockedPrefixes(t *testing.T) {
	require.True(t, SafeAddr(netip.MustParseAddr("93.184.216.34"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("127.0.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("10.0.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("100.64.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("169.254.169.254"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("192.0.2.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("198.18.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("203.0.113.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("240.0.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("::1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("::ffff:127.0.0.1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("64:ff9b::1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("64:ff9b:1::1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("100::1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("2001:db8::1"), nil))
	require.False(t, SafeAddr(netip.MustParseAddr("2002::1"), nil))
	require.False(t, SafeAddr(netip.Addr{}, nil))
}

func TestSafeAddr_UsesCallerBlockedPrefixes(t *testing.T) {
	blocked := []netip.Prefix{
		netip.MustParsePrefix("93.184.216.0/24"),
	}

	require.False(t, SafeAddr(netip.MustParseAddr("93.184.216.34"), blocked))
	require.True(t, SafeAddr(netip.MustParseAddr("8.8.8.8"), blocked))
}

package guardrails

import (
	"net/netip"

	"github.com/wandxy/hand/pkg/netpolicy"
)

var DefaultBlockedAddressPrefixes = netpolicy.DefaultBlockedAddressPrefixes

// SafeAddr reports whether addr avoids blocked loopback and private network prefixes.
func SafeAddr(addr netip.Addr, blockedPrefixes []netip.Prefix) bool {
	return netpolicy.SafeAddr(addr, blockedPrefixes)
}

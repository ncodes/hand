package guardrails

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostPolicy_CheckBlocksDeniedHost(t *testing.T) {
	policy := NewHostPolicy(nil, []string{"example.com"}, nil, nil)

	block, blocked := policy.Check("docs.example.com")

	require.True(t, blocked)
	require.Equal(t, "docs.example.com", block.Host)
	require.Equal(t, "example.com", block.Rule)
	require.Equal(t, "config", block.Source)
	require.Contains(t, block.Message, "native host denylist policy")
}

func TestHostPolicy_CheckBlocksHostMissingFromAllowlist(t *testing.T) {
	policy := NewHostPolicy([]string{"allowed.example"}, nil, nil, nil)

	block, blocked := policy.Check("other.example")

	require.True(t, blocked)
	require.Equal(t, "other.example", block.Host)
	require.Empty(t, block.Rule)
	require.Contains(t, block.Message, "native host allowlist policy")
}

func TestHostPolicy_CheckAllowsHostMatchingAllowlist(t *testing.T) {
	policy := NewHostPolicy([]string{"*.allowed.example"}, nil, nil, nil)

	_, blocked := policy.Check("api.allowed.example")
	require.False(t, blocked)

	_, blocked = policy.Check("allowed.example")
	require.True(t, blocked)
}

func TestHostPolicy_CheckLoadsRulesFromFiles(t *testing.T) {
	allowPath := filepath.Join(t.TempDir(), "allow.txt")
	denyPath := filepath.Join(t.TempDir(), "deny.txt")
	require.NoError(t, os.WriteFile(allowPath, []byte("allowed.example\n"), 0o600))
	require.NoError(t, os.WriteFile(denyPath, []byte("blocked.example\n"), 0o600))

	policy := NewHostPolicy(nil, nil, []string{allowPath}, []string{denyPath})

	_, blocked := policy.Check("allowed.example")
	require.False(t, blocked)

	block, blocked := policy.Check("blocked.example")
	require.True(t, blocked)
	require.Equal(t, denyPath, block.Source)
}

func TestHostPolicy_CheckAllowsHostWhenNoRulesMatchAndNoAllowlistExists(t *testing.T) {
	policy := NewHostPolicy(nil, []string{"blocked.example"}, nil, nil)

	_, blocked := policy.Check("allowed.example")

	require.False(t, blocked)
}

func TestHostPolicy_CheckAllowsEmptyHostValue(t *testing.T) {
	policy := NewHostPolicy([]string{"allowed.example"}, []string{"blocked.example"}, nil, nil)

	_, blocked := policy.Check(" ")

	require.False(t, blocked)
}

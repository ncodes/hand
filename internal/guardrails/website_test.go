package guardrails

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebsitePolicy_CheckAllowsWhenDisabled(t *testing.T) {
	policy := NewWebsitePolicy(false, []string{"example.com"}, nil)
	_, blocked := policy.Check("https://example.com/article")
	require.False(t, blocked)
}

func TestWebsitePolicy_CheckBlocksExactAndSubdomain(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{"example.com"}, nil)

	block, blocked := policy.Check("https://docs.example.com/article")
	require.True(t, blocked)
	require.Equal(t, "docs.example.com", block.Host)
	require.Equal(t, "example.com", block.Rule)
	require.Equal(t, "config", block.Source)
	require.Contains(t, block.Message, "blocked by configured website blocklist policy")
	require.Contains(t, block.Message, `from "config"`)
}

func TestWebsitePolicy_CheckWildcardBlocksOnlySubdomains(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{"*.example.com"}, nil)

	_, rootBlocked := policy.Check("https://example.com")
	block, subdomainBlocked := policy.Check("https://api.example.com")
	require.False(t, rootBlocked)
	require.True(t, subdomainBlocked)
	require.Equal(t, "*.example.com", block.Rule)
}

func TestWebsitePolicy_CheckNormalizesURLRules(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{"https://www.example.com/path"}, nil)

	block, blocked := policy.Check("https://www.example.com/news")

	require.True(t, blocked)
	require.Equal(t, "www.example.com", block.Host)
	require.Equal(t, "www.example.com", block.Rule)
}

func TestWebsitePolicy_CheckSupportsSchemelessInput(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{"example.com"}, nil)

	block, blocked := policy.Check("example.com/path")

	require.True(t, blocked)
	require.Equal(t, "example.com", block.Host)
}

func TestWebsitePolicy_CheckAllowsInvalidOrHostlessURL(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{"example.com"}, nil)

	_, blocked := policy.Check("not a url")

	require.False(t, blocked)
}

func TestWebsitePolicy_LoadsRulesFromFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked.txt")
	require.NoError(t, os.WriteFile(path, []byte("\n# comment\n*.blocked.test\n"), 0o600))

	policy := NewWebsitePolicy(true, nil, []string{path})
	block, blocked := policy.Check("https://sub.blocked.test/page")

	require.True(t, blocked)
	require.Equal(t, path, block.Source)
	require.Equal(t, "*.blocked.test", block.Rule)
	require.Contains(t, block.Message, `from "`+path+`"`)
}

func TestWebsitePolicy_IgnoresMissingFiles(t *testing.T) {
	policy := NewWebsitePolicy(true, nil, []string{filepath.Join(t.TempDir(), "missing.txt")})

	_, blocked := policy.Check("https://example.com")

	require.False(t, blocked)
	require.Empty(t, policy.Rules)
}

func TestWebsitePolicy_IgnoresEmptyFilesAndInvalidRules(t *testing.T) {
	policy := NewWebsitePolicy(true, []string{" ", ".", "https:///missing-host"}, []string{" "})

	_, blocked := policy.Check("https://example.com")

	require.False(t, blocked)
	require.Empty(t, policy.Rules)
}

func TestWebsitePolicy_MatchesNormalizedRules(t *testing.T) {
	require.True(t, websiteRuleMatches("HTTPS://WWW.Example.COM/path", "docs.www.example.com"))
	require.False(t, websiteRuleMatches("HTTPS://WWW.Example.COM/path", "docs.example.com"))
	require.False(t, websiteRuleMatches("*.example.com", "example.com"))
	require.False(t, websiteRuleMatches("*.example.com", "not-example.com"))
	require.False(t, websiteRuleMatches(" ", "example.com"))
	require.False(t, websiteRuleMatches("example.com", " "))
}

func TestWebsitePolicy_HostFromURLHandlesEmptyAndMalformedValues(t *testing.T) {
	require.Empty(t, hostFromWebsiteURL(" "))
	require.Empty(t, hostFromWebsiteURL("%"))
}

func TestWebsiteBlockMessage_OmitsSourceWhenRuleSourceIsEmpty(t *testing.T) {
	message := websiteBlockMessage("example.com", WebsiteRule{
		Pattern: "example.com",
	})

	require.Equal(t, `blocked by configured website blocklist policy: "example.com" matched "example.com"`, message)
}

func TestFirstMatchingDomainRule_HandlesEmptyAndUnmatchedHosts(t *testing.T) {
	rules := []domainRule{{Pattern: "example.com", Source: "config"}}

	rule, matched := firstMatchingDomainRule(rules, " ")
	require.False(t, matched)
	require.Equal(t, domainRule{}, rule)

	rule, matched = firstMatchingDomainRule(rules, "other.example")
	require.False(t, matched)
	require.Equal(t, domainRule{}, rule)

	rule, matched = firstMatchingDomainRule(rules, "docs.example.com")
	require.True(t, matched)
	require.Equal(t, rules[0], rule)
}

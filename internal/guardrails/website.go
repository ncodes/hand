package guardrails

import (
	"net/url"
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

// WebsitePolicy defines website policy settings.
type WebsitePolicy struct {
	Enabled bool
	Rules   []WebsiteRule
}

// WebsiteRule classifies website safety rules.
type WebsiteRule = domainRule

// WebsiteBlock describes one blocked website rule.
type WebsiteBlock struct {
	URL     string
	Host    string
	Rule    string
	Source  string
	Message string
}

// NewWebsitePolicy returns a website safety policy compiled from blocked website rules.
func NewWebsitePolicy(enabled bool, domains, files []string) WebsitePolicy {
	rules := appendDomainRules(nil, domains, "config")
	rules = appendDomainRulesFromFiles(rules, files)
	return WebsitePolicy{Enabled: enabled, Rules: rules}
}

func (p WebsitePolicy) Check(rawURL string) (WebsiteBlock, bool) {
	if !p.Enabled || len(p.Rules) == 0 {
		return WebsiteBlock{}, false
	}

	host := getHostFromWebsiteURL(rawURL)
	if host == "" {
		return WebsiteBlock{}, false
	}

	if rule, ok := getFirstMatchingDomainRule(p.Rules, host); ok {
		message := getWebsiteBlockMessage(host, rule)
		stringValue1 := str.String(rawURL)
		return WebsiteBlock{
			URL:     stringValue1.Trim(),
			Host:    host,
			Rule:    rule.Pattern,
			Source:  rule.Source,
			Message: message,
		}, true
	}

	return WebsiteBlock{}, false
}

func getWebsiteBlockMessage(host string, rule WebsiteRule) string {
	message := `blocked by configured website blocklist policy: "` + host + `" matched "` + rule.Pattern + `"`
	stringValue2 := str.String(rule.Source)
	source := stringValue2.Trim()
	if source == "" {
		return message
	}

	return message + ` from "` + source + `"`
}

func normalizeWebsiteRule(value string) string {
	stringValue3 := str.String(value)
	value = stringValue3.Normalized()
	if value == "" {
		return ""
	}

	wildcard := strings.HasPrefix(value, "*.")
	if wildcard {
		value = strings.TrimPrefix(value, "*.")
	}

	host := getHostFromWebsiteURL(value)
	if host == "" {
		if strings.Contains(value, "://") {
			return ""
		}
		host = normalizeWebsiteHost(value)
	}
	if host == "" {
		return ""
	}
	if wildcard {
		return "*." + host
	}

	return host
}

func getHostFromWebsiteURL(rawURL string) string {
	stringValue4 := str.String(rawURL)
	rawURL = stringValue4.Trim()
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Hostname() != "" {
		return normalizeWebsiteHost(parsed.Hostname())
	}
	if err == nil && parsed.Scheme != "" {
		return ""
	}

	parsed, err = url.Parse("//" + rawURL)
	if err != nil {
		return ""
	}

	return normalizeWebsiteHost(parsed.Hostname())
}

func normalizeWebsiteHost(host string) string {
	stringValue5 := str.String(host)
	host = stringValue5.Normalized()
	host = strings.TrimSuffix(host, ".")

	return host
}

func checkWebsiteRuleMatches(rule, host string) bool {
	rule = normalizeWebsiteRule(rule)
	host = normalizeWebsiteHost(host)
	if rule == "" || host == "" {
		return false
	}

	if after, ok := strings.CutPrefix(rule, "*."); ok {
		suffix := after
		return host != suffix && strings.HasSuffix(host, "."+suffix)
	}

	return host == rule || strings.HasSuffix(host, "."+rule)
}

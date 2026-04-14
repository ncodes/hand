package guardrails

import (
	"net/url"
	"strings"
)

type WebsitePolicy struct {
	Enabled bool
	Rules   []WebsiteRule
}

type WebsiteRule = domainRule

type WebsiteBlock struct {
	URL     string
	Host    string
	Rule    string
	Source  string
	Message string
}

func NewWebsitePolicy(enabled bool, domains, files []string) WebsitePolicy {
	rules := appendDomainRules(nil, domains, "config")
	rules = appendDomainRulesFromFiles(rules, files)
	return WebsitePolicy{Enabled: enabled, Rules: rules}
}

func (p WebsitePolicy) Check(rawURL string) (WebsiteBlock, bool) {
	if !p.Enabled || len(p.Rules) == 0 {
		return WebsiteBlock{}, false
	}

	host := hostFromWebsiteURL(rawURL)
	if host == "" {
		return WebsiteBlock{}, false
	}

	if rule, ok := firstMatchingDomainRule(p.Rules, host); ok {
		message := websiteBlockMessage(host, rule)
		return WebsiteBlock{
			URL:     strings.TrimSpace(rawURL),
			Host:    host,
			Rule:    rule.Pattern,
			Source:  rule.Source,
			Message: message,
		}, true
	}

	return WebsiteBlock{}, false
}

func websiteBlockMessage(host string, rule WebsiteRule) string {
	message := `blocked by configured website blocklist policy: "` + host + `" matched "` + rule.Pattern + `"`
	source := strings.TrimSpace(rule.Source)
	if source == "" {
		return message
	}

	return message + ` from "` + source + `"`
}

func normalizeWebsiteRule(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	wildcard := strings.HasPrefix(value, "*.")
	if wildcard {
		value = strings.TrimPrefix(value, "*.")
	}

	host := hostFromWebsiteURL(value)
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

func hostFromWebsiteURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
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
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")

	return host
}

func websiteRuleMatches(rule, host string) bool {
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

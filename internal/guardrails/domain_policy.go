package guardrails

import (
	"bufio"
	"os"
	"strings"
)

type domainRule struct {
	Pattern string
	Source  string
}

func appendDomainRules(existing []domainRule, values []string, source string) []domainRule {
	for _, value := range values {
		rule := normalizeWebsiteRule(value)
		if rule == "" {
			continue
		}

		existing = append(existing, domainRule{
			Pattern: rule,
			Source:  strings.TrimSpace(source),
		})
	}

	return existing
}

func appendDomainRulesFromFiles(existing []domainRule, files []string) []domainRule {
	for _, file := range files {
		existing = appendDomainRules(existing, readPolicyFile(file), strings.TrimSpace(file))
	}

	return existing
}

func firstMatchingDomainRule(rules []domainRule, host string) (domainRule, bool) {
	host = normalizeWebsiteHost(host)
	if host == "" {
		return domainRule{}, false
	}

	for _, rule := range rules {
		if websiteRuleMatches(rule.Pattern, host) {
			return rule, true
		}
	}

	return domainRule{}, false
}

func readPolicyFile(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var values []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		values = append(values, line)
	}

	return values
}

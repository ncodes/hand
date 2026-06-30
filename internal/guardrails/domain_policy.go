package guardrails

import (
	"bufio"
	"os"
	"strings"

	"github.com/wandxy/morph/pkg/stringx"
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
			Source:  stringx.String(source).Trim(),
		})
	}

	return existing
}

func appendDomainRulesFromFiles(existing []domainRule, files []string) []domainRule {
	for _, file := range files {
		existing = appendDomainRules(existing, loadPolicyFile(file), stringx.String(file).Trim())
	}

	return existing
}

func getFirstMatchingDomainRule(rules []domainRule, host string) (domainRule, bool) {
	host = normalizeWebsiteHost(host)
	if host == "" {
		return domainRule{}, false
	}

	for _, rule := range rules {
		if checkWebsiteRuleMatches(rule.Pattern, host) {
			return rule, true
		}
	}

	return domainRule{}, false
}

func loadPolicyFile(path string) []string {
	path = stringx.String(path).Trim()
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
		line := stringx.String(scanner.Text()).Trim()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		values = append(values, line)
	}

	return values
}

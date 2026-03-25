package guardrails

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

type SafetyScanResult struct {
	Content  string
	Blocked  bool
	Findings []string
}

type threatPattern struct {
	re *regexp.Regexp
	id string
}

var (
	safetyThreatPatterns = []threatPattern{
		{re: regexp.MustCompile(`(?i)ignore\s+(previous|all|above|prior)\s+instructions`), id: "prompt_injection"},
		{re: regexp.MustCompile(`(?i)do\s+not\s+tell\s+the\s+user`), id: "deception_hide"},
		{re: regexp.MustCompile(`(?i)system\s+prompt\s+override`), id: "sys_prompt_override"},
		{re: regexp.MustCompile(`(?i)disregard\s+(your|all|any)\s+(instructions|rules|guidelines)`), id: "disregard_rules"},
		{re: regexp.MustCompile(`(?i)act\s+as\s+(if|though)\s+you\s+(have\s+no|don't\s+have)\s+(restrictions|limits|rules)`), id: "bypass_restrictions"},
		{re: regexp.MustCompile(`(?i)<!--[^>]*(?:ignore|override|system|secret|hidden)[^>]*-->`), id: "html_comment_injection"},
		{re: regexp.MustCompile(`(?i)<\s*div\s+style\s*=\s*["'].*display\s*:\s*none`), id: "hidden_div"},
		{re: regexp.MustCompile(`(?i)translate\s+.*\s+into\s+.*\s+and\s+(execute|run|eval)`), id: "translate_execute"},
		{re: regexp.MustCompile(`(?i)curl\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|API)`), id: "exfil_curl"},
		{re: regexp.MustCompile(`(?i)cat\s+[^\n]*(\.env|credentials|\.netrc|\.pgpass)`), id: "read_secrets"},
	}
	safetyInvisibleChars = []rune{
		'\u200b', '\u200c', '\u200d', '\u2060', '\ufeff',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
	}
)

func SafetyScan(content, source string) SafetyScanResult {
	findings := make([]string, 0, len(safetyThreatPatterns))

	for _, char := range safetyInvisibleChars {
		if strings.ContainsRune(content, char) {
			findings = append(findings, fmt.Sprintf("invisible unicode U+%04X", char))
		}
	}

	for _, pattern := range safetyThreatPatterns {
		if pattern.re.MatchString(content) {
			findings = append(findings, pattern.id)
		}
	}

	if len(findings) == 0 {
		return SafetyScanResult{Content: content}
	}

	blocked := fmt.Sprintf("[BLOCKED: %s contained potential prompt injection (%s). Content not loaded.]", source, strings.Join(findings, ", "))
	log.Warn().Str("source", source).Strs("findings", findings).Msg("Content blocked by safety scan")
	return SafetyScanResult{
		Content:  blocked,
		Blocked:  true,
		Findings: findings,
	}
}

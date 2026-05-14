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
	Findings []SafetyFinding
}

type threatPattern struct {
	re       *regexp.Regexp
	id       SafetyFindingID
	category SafetyCategory
}

var (
	safetyThreatPatterns = []threatPattern{
		{re: regexp.MustCompile(`(?i)ignore\s+(previous|all|above|prior)\s+instructions`), id: SafetyFindingPromptInjection, category: SafetyCategoryPromptInjection},
		{re: regexp.MustCompile(`(?i)show\s+(me\s+)?(your\s+)?(system|developer)\s+(prompt|message|instructions)`), id: SafetyFindingPromptExfiltration, category: SafetyCategoryPromptExfiltration},
		{re: regexp.MustCompile(`(?i)do\s+not\s+tell\s+the\s+user`), id: SafetyFindingDeceptionHide, category: SafetyCategoryInstructionManipulation},
		{re: regexp.MustCompile(`(?i)system\s+prompt\s+override`), id: SafetyFindingSystemPromptOverride, category: SafetyCategoryInstructionManipulation},
		{re: regexp.MustCompile(`(?i)disregard\s+(your|all|any)\s+(instructions|rules|guidelines)`), id: SafetyFindingDisregardRules, category: SafetyCategoryInstructionManipulation},
		{re: regexp.MustCompile(`(?i)act\s+as\s+(if|though)\s+you\s+(have\s+no|don't\s+have)\s+(restrictions|limits|rules)`), id: SafetyFindingBypassRestrictions, category: SafetyCategoryInstructionManipulation},
		{re: regexp.MustCompile(`(?i)<!--[^>]*(?:ignore|override|system|secret|hidden)[^>]*-->`), id: SafetyFindingHTMLCommentInjection, category: SafetyCategoryHiddenInstruction},
		{re: regexp.MustCompile(`(?i)<\s*div\s+style\s*=\s*["'].*display\s*:\s*none`), id: SafetyFindingHiddenDiv, category: SafetyCategoryHiddenInstruction},
		{re: regexp.MustCompile(`(?i)translate\s+.*\s+into\s+.*\s+and\s+(execute|run|eval)`), id: SafetyFindingTranslateExecute, category: SafetyCategorySuspiciousToolCoercion},
		{re: regexp.MustCompile(`(?i)curl\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|API)`), id: SafetyFindingCurlSecretExfil, category: SafetyCategorySecretExfiltration},
		{re: regexp.MustCompile(`(?i)cat\s+[^\n]*(\.env|credentials|\.netrc|\.pgpass)`), id: SafetyFindingReadSecrets, category: SafetyCategorySecretExfiltration},
	}
	safetyInvisibleChars = []rune{
		'\u200b', '\u200c', '\u200d', '\u2060', '\ufeff',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
	}
)

func SafetyScan(content, source string) SafetyScanResult {
	findings := findSafetyFindings(content, source)
	if len(findings) == 0 {
		return SafetyScanResult{Content: content}
	}

	findingIDs := getSafetyFindingIDs(findings)
	blocked := fmt.Sprintf("[BLOCKED: %s contained potential prompt injection (%s). Content not loaded.]",
		source, strings.Join(findingIDs, ", "))
	log.Warn().Str("source", source).Strs("findings", findingIDs).Msg("Content blocked by safety scan")

	return SafetyScanResult{
		Content:  blocked,
		Blocked:  true,
		Findings: findings,
	}
}

func findSafetyFindings(content, source string) []SafetyFinding {
	findings := make([]SafetyFinding, 0, len(safetyThreatPatterns))

	for _, char := range safetyInvisibleChars {
		if strings.ContainsRune(content, char) {
			findings = append(findings, SafetyFinding{
				ID:       SafetyFindingInvisibleUnicode,
				Category: SafetyCategoryHiddenInstruction,
				Message:  fmt.Sprintf("invisible unicode U+%04X", char),
				Source:   source,
			})
		}
	}

	for _, pattern := range safetyThreatPatterns {
		if pattern.re.MatchString(content) {
			findings = append(findings, SafetyFinding{
				ID:       pattern.id,
				Category: pattern.category,
				Source:   source,
			})
		}
	}

	return findings
}

package guardrails

import "strings"

type SafetyCategory string

const (
	SafetyCategoryPromptInjection         SafetyCategory = "prompt_injection"
	SafetyCategoryPromptExfiltration      SafetyCategory = "prompt_exfiltration"
	SafetyCategoryInstructionManipulation SafetyCategory = "instruction_manipulation"
	SafetyCategorySecretExfiltration      SafetyCategory = "secret_exfiltration"
	SafetyCategoryHiddenInstruction       SafetyCategory = "hidden_or_obfuscated_instruction"
	SafetyCategorySuspiciousToolCoercion  SafetyCategory = "suspicious_tool_use_coercion"
)

type SafetyFindingID string

const (
	SafetyFindingPromptInjection      SafetyFindingID = "prompt_injection"
	SafetyFindingPromptExfiltration   SafetyFindingID = "prompt_exfiltration"
	SafetyFindingDeceptionHide        SafetyFindingID = "deception_hide"
	SafetyFindingSystemPromptOverride SafetyFindingID = "sys_prompt_override"
	SafetyFindingDisregardRules       SafetyFindingID = "disregard_rules"
	SafetyFindingBypassRestrictions   SafetyFindingID = "bypass_restrictions"
	SafetyFindingHTMLCommentInjection SafetyFindingID = "html_comment_injection"
	SafetyFindingHiddenDiv            SafetyFindingID = "hidden_div"
	SafetyFindingTranslateExecute     SafetyFindingID = "translate_execute"
	SafetyFindingCurlSecretExfil      SafetyFindingID = "exfil_curl"
	SafetyFindingReadSecrets          SafetyFindingID = "read_secrets"
	SafetyFindingInvisibleUnicode     SafetyFindingID = "invisible_unicode"
)

const defaultSafetyRefusal = "I can't help reveal, override, or manipulate hidden instructions, secrets, or safety controls. I can still explain the public behavior at a high level."

type SafetyFinding struct {
	ID       SafetyFindingID
	Category SafetyCategory
	Message  string
	Source   string
}

type InputSafetyResult struct {
	Allowed        bool
	Blocked        bool
	RefusalMessage string
	Findings       []SafetyFinding
}

type OutputSafetyResult struct {
	Content        string
	Blocked        bool
	Redacted       bool
	RefusalMessage string
	Findings       []SafetyFinding
}

func CheckInputSafety(content, source string) InputSafetyResult {
	findings := findSafetyFindings(content, source)
	if len(findings) == 0 {
		return InputSafetyResult{Allowed: true}
	}

	return InputSafetyResult{
		Blocked:        true,
		RefusalMessage: defaultSafetyRefusal,
		Findings:       findings,
	}
}

func CheckOutputSafety(content, source string, redactor Redactor) OutputSafetyResult {
	if redactor == nil {
		redactor = NewRedactor()
	}

	redactedContent, _ := redactor.Sanitize(content).(string)
	if redactedContent == "" && content != "" {
		redactedContent = content
	}

	findings := findSafetyFindings(redactedContent, source)
	if len(findings) == 0 {
		return OutputSafetyResult{
			Content:  redactedContent,
			Redacted: redactedContent != content,
		}
	}

	return OutputSafetyResult{
		Content:        defaultSafetyRefusal,
		Blocked:        true,
		Redacted:       redactedContent != content,
		RefusalMessage: defaultSafetyRefusal,
		Findings:       findings,
	}
}

func (finding SafetyFinding) LogFields() map[string]string {
	fields := map[string]string{
		"id":       string(finding.ID),
		"category": string(finding.Category),
	}
	if source := strings.TrimSpace(finding.Source); source != "" {
		fields["source"] = source
	}

	return fields
}

func getSafetyFindingIDs(findings []SafetyFinding) []string {
	ids := make([]string, 0, len(findings))
	for _, finding := range findings {
		ids = append(ids, string(finding.ID))
	}

	return ids
}

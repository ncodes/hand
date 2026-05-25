package guardrails

import "strings"

// SafetyTracePayloadOptions controls safety trace payload.
type SafetyTracePayloadOptions struct {
	SessionID     string
	Source        string
	Action        string
	ContentLength int
	Blocked       bool
	Redacted      bool
	Refusal       string
	Findings      []SafetyFinding
}

// SafetyTracePayload converts a safety finding into trace payload fields.
func SafetyTracePayload(opts SafetyTracePayloadOptions) map[string]any {
	payload := map[string]any{
		"action":         strings.TrimSpace(opts.Action),
		"blocked":        opts.Blocked,
		"redacted":       opts.Redacted,
		"content_length": opts.ContentLength,
		"findings":       SafetyFindingLogFields(opts.Findings),
	}
	if sessionID := strings.TrimSpace(opts.SessionID); sessionID != "" {
		payload["session_id"] = sessionID
	}
	if source := strings.TrimSpace(opts.Source); source != "" {
		payload["source"] = source
	}
	if refusal := strings.TrimSpace(opts.Refusal); refusal != "" {
		payload["refusal"] = refusal
	}

	return payload
}

// SafetyFindingLogFields converts a safety finding into structured log fields.
func SafetyFindingLogFields(findings []SafetyFinding) []map[string]string {
	if len(findings) == 0 {
		return nil
	}

	fields := make([]map[string]string, 0, len(findings))
	for _, finding := range findings {
		fields = append(fields, finding.LogFields())
	}

	return fields
}

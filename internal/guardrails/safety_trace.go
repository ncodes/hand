package guardrails

import "github.com/wandxy/morph/pkg/str"

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
	stringValue1 := str.String(opts.Action)
	payload := map[string]any{
		"action":         stringValue1.Trim(),
		"blocked":        opts.Blocked,
		"redacted":       opts.Redacted,
		"content_length": opts.ContentLength,
		"findings":       SafetyFindingLogFields(opts.Findings),
	}
	stringValue2 := str.String(opts.SessionID)
	if sessionID := stringValue2.Trim(); sessionID != "" {
		payload["session_id"] = sessionID
	}
	stringValue3 := str.String(opts.Source)
	if source := stringValue3.Trim(); source != "" {
		payload["source"] = source
	}
	stringValue4 := str.String(opts.Refusal)
	if refusal := stringValue4.Trim(); refusal != "" {
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

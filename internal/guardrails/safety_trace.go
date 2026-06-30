package guardrails

import "github.com/wandxy/morph/pkg/stringx"

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
		"action":         stringx.String(opts.Action).Trim(),
		"blocked":        opts.Blocked,
		"redacted":       opts.Redacted,
		"content_length": opts.ContentLength,
		"findings":       SafetyFindingLogFields(opts.Findings),
	}
	if sessionID := stringx.String(opts.SessionID).Trim(); sessionID != "" {
		payload["session_id"] = sessionID
	}
	if source := stringx.String(opts.Source).Trim(); source != "" {
		payload["source"] = source
	}
	if refusal := stringx.String(opts.Refusal).Trim(); refusal != "" {
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

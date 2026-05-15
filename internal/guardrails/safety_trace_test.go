package guardrails

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafetyTracePayload_ExcludesRawContent(t *testing.T) {
	payload := SafetyTracePayload(SafetyTracePayloadOptions{
		SessionID:     "default",
		Source:        "user",
		Action:        "blocked",
		ContentLength: 32,
		Blocked:       true,
		Findings: []SafetyFinding{{
			ID:       SafetyFindingPromptExfiltration,
			Category: SafetyCategoryPromptExfiltration,
			Source:   "user",
		}},
		Refusal: defaultSafetyRefusal,
	})

	require.Equal(t, "default", payload["session_id"])
	require.Equal(t, "user", payload["source"])
	require.Equal(t, "blocked", payload["action"])
	require.Equal(t, 32, payload["content_length"])
	require.Equal(t, true, payload["blocked"])
	require.NotContains(t, payload, "content")
	require.NotContains(t, payload, "input")
	require.NotContains(t, payload, "message")
	findings, ok := payload["findings"].([]map[string]string)
	require.True(t, ok)
	require.Contains(t, findings, map[string]string{
		"id":       string(SafetyFindingPromptExfiltration),
		"category": string(SafetyCategoryPromptExfiltration),
		"source":   "user",
	})
}

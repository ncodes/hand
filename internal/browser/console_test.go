package browser

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestGetSafeConsoleMessages_BoundsRedactsAndNormalizesBackendOutput(t *testing.T) {
	localTime := time.Date(2026, 7, 19, 12, 0, 0, 0, time.FixedZone("WAT", 60*60))
	messages := make([]ConsoleMessage, maxConsoleMessages+2)
	for index := range messages {
		messages[index] = ConsoleMessage{Level: ConsoleLevel("invalid"), Text: "ordinary", Timestamp: localTime}
	}
	messages[len(messages)-1].Text = `token=private Bearer credential ` + "\x1b[31mred\x1b[0m" + strings.Repeat("x", maxConsoleText)

	result := getSafeConsoleMessages(messages, 2)
	require.Len(t, result, 2)
	require.Equal(t, ConsoleInfo, result[0].Level)
	require.Equal(t, time.UTC, result[0].Timestamp.Location())
	require.NotContains(t, result[1].Text, "private")
	require.NotContains(t, result[1].Text, "credential")
	require.NotContains(t, result[1].Text, "\x1b")
	require.Len(t, result[1].Text, maxConsoleText)

	defaulted := getSafeConsoleMessages(messages, -1)
	require.Len(t, defaulted, defaultConsoleLimit)
	require.Empty(t, getSafeConsoleMessages(nil, maxConsoleMessages+1))
	require.Equal(
		t,
		"fetch https://example.com/private, socket wss://example.com/events)",
		sanitizeConsoleText(
			"fetch https://user:password@example.com/private?token=secret#fragment, "+
				"socket wss://example.com/events?credential=secret)",
		),
	)
	unicodeText := sanitizeConsoleText(strings.Repeat("界", maxConsoleText))
	require.LessOrEqual(t, len(unicodeText), maxConsoleText)
	require.True(t, utf8.ValidString(unicodeText))
}

package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultTheme_ExposesSharedTUIColorTokens(t *testing.T) {
	require.Equal(t, "232", DefaultTheme.InputFrameBackground)
	require.Equal(t, "235", DefaultTheme.UserTranscriptBackground)
	require.Equal(t, "83", DefaultTheme.ToolCompletedDot)
	require.Equal(t, "235", DefaultTheme.NoticeBackground)
	require.Equal(t, "39", DefaultTheme.MarkdownLinkForeground)
}

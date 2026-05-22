package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultTheme_ExposesSharedTUIColorTokens(t *testing.T) {
	require.Equal(t, "#050505", DefaultTheme.InputFrameBackground)
	require.Equal(t, "#151515", DefaultTheme.UserTranscriptBackground)
	require.Equal(t, "83", DefaultTheme.ToolCompletedDot)
	require.Equal(t, "#151515", DefaultTheme.NoticeBackground)
	require.Equal(t, "39", DefaultTheme.MarkdownLinkForeground)
}

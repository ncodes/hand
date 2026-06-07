package telegram

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkText(t *testing.T) {
	require.Nil(t, ChunkText(" ", 3))
	require.Equal(t, []string{"abc", "def", "g"}, ChunkText("abcdefg", 3))
	require.Equal(t, []string{strings.Repeat("x", MessageTextLimit)}, ChunkText(strings.Repeat("x", MessageTextLimit), 0))
}

func TestSupportsNativeDraft(t *testing.T) {
	require.True(t, SupportsNativeDraft(Target{ChatType: "private"}))
	require.False(t, SupportsNativeDraft(Target{ChatType: "group"}))
	require.False(t, SupportsNativeDraft(Target{ChatType: "private", ThreadID: "42"}))
}

func TestWithCursor(t *testing.T) {
	require.Equal(t, DraftCursor, WithCursor(" "))
	require.Equal(t, "hello\n"+DraftCursor, WithCursor(" hello "))
}

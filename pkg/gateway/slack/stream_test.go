package slack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkMarkdown_TrimsAndSplitsByRuneLimit(t *testing.T) {
	chunks := ChunkMarkdown(" hello世界 ", 3)

	require.Equal(t, []string{"hel", "lo世", "界"}, chunks)
}

func TestChunkMarkdown_DefaultsInvalidLimitAndSkipsEmptyText(t *testing.T) {
	require.Nil(t, ChunkMarkdown("   ", 10))
	require.Equal(t, []string{"hello"}, ChunkMarkdown("hello", 0))
}

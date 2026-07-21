package search

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestChunkVectorText_PreservesBoundariesAndLimits(t *testing.T) {
	text := "first paragraph\n\nsecond line with words\nthird"

	chunks, truncated := ChunkVectorText(text, VectorChunkOptions{
		MaxInputBytes:    18,
		MaxDocumentBytes: 100,
	})

	require.False(t, truncated)
	require.Equal(t, []string{"first paragraph", "second line with", "words\nthird"}, chunks)
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk), 18)
		require.True(t, utf8.ValidString(chunk))
	}
}

func TestChunkVectorText_UsesUTF8SafeHardSplitAndDocumentLimit(t *testing.T) {
	text := strings.Repeat("🙂", 20)

	chunks, truncated := ChunkVectorText(text, VectorChunkOptions{
		MaxInputBytes:    9,
		MaxDocumentBytes: 25,
	})

	require.True(t, truncated)
	require.Equal(t, []string{"🙂🙂", "🙂🙂", "🙂🙂"}, chunks)
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk), 9)
		require.True(t, utf8.ValidString(chunk))
	}
}

func TestChunkVectorText_DiscardsEmptyContent(t *testing.T) {
	chunks, truncated := ChunkVectorText(" \n\t ", VectorChunkOptions{})

	require.Nil(t, chunks)
	require.False(t, truncated)
}

func TestChunkVectorText_DoesNotStallWhenLimitIsSmallerThanOneRune(t *testing.T) {
	chunks, truncated := ChunkVectorText("🙂", VectorChunkOptions{
		MaxInputBytes:    1,
		MaxDocumentBytes: 1,
	})

	require.True(t, truncated)
	require.Equal(t, []string{"🙂"}, chunks)
	require.EqualError(t, CheckVectorInputSizes([]VectorInput{{ID: "chunk", Text: chunks[0]}}, 1),
		`vector input "chunk" exceeds the configured byte limit`)
}

func TestTruncateUTF8_PreservesCompleteRunesAtEveryBoundary(t *testing.T) {
	require.Equal(t, "plain", truncateUTF8("plain", 10))
	require.Equal(t, "🙂", truncateUTF8("🙂text", 4))
}

func TestUTF8Boundary_HandlesLimitsOutsideAndInsideText(t *testing.T) {
	require.Equal(t, len("text"), utf8Boundary("text", len("text")))
	require.Zero(t, utf8Boundary("text", 0))
	require.Equal(t, 1, utf8Boundary("a🙂", 2))
}

package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownStreamCollector_CommitsOnlyNewlineStableChunks(t *testing.T) {
	var collector markdownStreamCollector

	require.Nil(t, collector.Add(""))
	require.Nil(t, collector.Add("hello"))
	require.Equal(t, "hello", collector.Render())

	chunks := collector.Add(" world\nnext")

	require.Equal(t, []string{"hello world\n"}, chunks)
	require.Equal(t, "hello world\nnext", collector.Render())
}

func TestMarkdownStreamCollector_HandlesSplitMarkdownFences(t *testing.T) {
	var collector markdownStreamCollector

	require.Nil(t, collector.Add("```"))
	require.Equal(t, "```", collector.Render())

	chunks := collector.Add("go\nfmt.Println(1)\n```")

	require.Equal(t, []string{"```go\nfmt.Println(1)\n"}, chunks)
	require.Equal(t, "```go\nfmt.Println(1)\n```", collector.Render())
	require.Equal(t, "```go\nfmt.Println(1)\n```", collector.Finalize())
}

func TestMarkdownStreamCollector_HandlesSplitListMarkers(t *testing.T) {
	var collector markdownStreamCollector

	require.Nil(t, collector.Add("-"))
	require.Nil(t, collector.Add(" item"))

	chunks := collector.Add("\n- second\n")

	require.Equal(t, []string{"- item\n- second\n"}, chunks)
	require.Equal(t, "- item\n- second\n", collector.Finalize())
}

func TestMarkdownStreamCollector_HandlesSplitCodeBlocks(t *testing.T) {
	var collector markdownStreamCollector

	collector.Add("```go\nfunc main")
	chunks := collector.Add("() {}\n```\n")

	require.Equal(t, []string{"func main() {}\n```\n"}, chunks)
	require.Equal(t, "```go\nfunc main() {}\n```\n", collector.Render())
}

func TestMarkdownStreamCollector_PreservesWideCharacters(t *testing.T) {
	var collector markdownStreamCollector

	collector.Add("測")
	collector.Add("試\n")

	require.Equal(t, "測試\n", collector.Finalize())
}

func TestMarkdownStreamCollector_DoesNotDuplicateCommittedLines(t *testing.T) {
	var collector markdownStreamCollector

	collector.Add("alpha\n")
	collector.Add("beta")
	text := collector.Finalize()

	require.Equal(t, "alpha\nbeta", text)
	require.Equal(t, 1, strings.Count(text, "alpha"))
}

func TestMarkdownStreamCollector_StreamedRenderMatchesFinalRender(t *testing.T) {
	var collector markdownStreamCollector
	deltas := []string{"# Title\n", "\n- one", "\n- two\n", "tail\n\n"}
	rendered := ""
	for _, delta := range deltas {
		collector.Add(delta)
		rendered = collector.Render()
	}

	require.Equal(t, rendered, collector.Finalize())
}

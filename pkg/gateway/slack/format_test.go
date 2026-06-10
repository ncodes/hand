package slack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatMrkdwn_FormatsCommonMarkdown(t *testing.T) {
	input := stringsJoin(
		"# Heading",
		"**bold** and __also bold__",
		"*italic* and _also italic_",
		"[Hand](https://example.com/docs)",
		"`code.value`",
		"~~gone~~",
		">quote & more",
	)

	require.Equal(t, stringsJoin(
		"*Heading*",
		"*bold* and *also bold*",
		"_italic_ and _also italic_",
		"<https://example.com/docs|Hand>",
		"`code.value`",
		"~gone~",
		"> quote &amp; more",
	), FormatMrkdwn(input))
}

func TestFormatMrkdwn_ReturnsEmptyText(t *testing.T) {
	require.Empty(t, FormatMrkdwn(""))
}

func TestFormatMrkdwn_EscapesPlainText(t *testing.T) {
	require.Equal(t, "hello &lt;world&gt; &amp; friends", FormatMrkdwn("hello <world> & friends"))
}

func TestFormatMrkdwn_PreservesFencedCode(t *testing.T) {
	input := "```go\nif a < b && b > c {}\n```"

	require.Equal(t, "```\nif a &lt; b &amp;&amp; b &gt; c {}\n```", FormatMrkdwn(input))
}

func TestFormatMrkdwn_EscapesLinkLabels(t *testing.T) {
	require.Equal(t,
		"<https://example.com/?q=a%3Eb|A &amp; B - C>",
		FormatMrkdwn("[A & B | C](https://example.com/?q=a>b)"),
	)
}

func TestFormatMrkdwn_PreservesSlackTokens(t *testing.T) {
	input := stringsJoin(
		"<@U12345678> user",
		"<#C12345678|general> channel",
		"<!here> <!channel> <!everyone>",
		"<!subteam^S12345678|team>",
		"<!date^1717776000^{date_short_pretty} at {time}|June 7, 2024 at 12:00 PM>",
		"<not-a-token>",
	)

	require.Equal(t, stringsJoin(
		"<@U12345678> user",
		"<#C12345678|general> channel",
		"<!here> <!channel> <!everyone>",
		"<!subteam^S12345678|team>",
		"<!date^1717776000^{date_short_pretty} at {time}|June 7, 2024 at 12:00 PM>",
		"&lt;not-a-token&gt;",
	), FormatMrkdwn(input))
}

func TestFormatStreamMarkdown_NormalizesStreamingMarkdown(t *testing.T) {
	input := stringsJoin(
		"Mention-style examples:",
		"<@U12345678> user mention",
		"```go",
		"a < b",
		"```",
		"Done",
	)

	require.Equal(t, "Mention-style examples:\n<@U12345678> user mention\n```\na < b\n```\nDone", FormatStreamMarkdown(input))
}

func TestFormatStreamMarkdown_NormalizesStrikethroughOutsideFencedCode(t *testing.T) {
	input := stringsJoin(
		"~gone~ and ~~already gone~~ and `~literal~`",
		"```",
		"~~literal~~",
		"```",
	)

	require.Equal(t, "~~gone~~ and ~~already gone~~ and `~literal~`\n```\n~~literal~~\n```", FormatStreamMarkdown(input))
}

func TestFormatStreamChunks_StreamsFencedCodeAsMarkdownText(t *testing.T) {
	input := "Before\n\n```go\npackage main\nimport \"fmt\"\n```\n\nAfter"

	require.Equal(t, []Chunk{
		MarkdownTextChunk("Before\n\n"),
		FencedCodeChunk("package main\nimport \"fmt\"\n"),
		MarkdownTextChunk("\n\nAfter"),
	}, FormatStreamChunks(input))
}

func TestFencedCodeChunk_WrapsCodeInUnlabeledFence(t *testing.T) {
	require.Equal(t,
		MarkdownTextChunk("```\npackage main\nimport \"fmt\"\n```"),
		FencedCodeChunk("package main\nimport \"fmt\""),
	)
}

func TestFormatStreamChunks_StripsGenericFenceInfoString(t *testing.T) {
	input := "```mermaid\ngraph TD\nA-->B\n```"

	require.Equal(t, []Chunk{
		FencedCodeChunk("graph TD\nA-->B\n"),
	}, FormatStreamChunks(input))
}

func TestFormatStreamChunks_PreservesUnlabeledCodeFirstLine(t *testing.T) {
	input := "```\npackage main\nfunc main() {}\n```"

	require.Equal(t, []Chunk{
		FencedCodeChunk("package main\nfunc main() {}\n"),
	}, FormatStreamChunks(input))
}

func TestFormatStreamChunks_KeepsNewlineAfterClosingFence(t *testing.T) {
	require.Equal(t, []Chunk{
		FencedCodeChunk("graph TD\n"),
		MarkdownTextChunk("\n"),
	}, FormatStreamChunks("```mermaid\ngraph TD\n```\n"))
}

func TestFormatStreamChunks_KeepsCodeLinesAfterFenceInfoString(t *testing.T) {
	input := "```go\npackage main\nimport \"fmt\"\n\nfunc main() {}\n```"

	require.Equal(t, []Chunk{
		FencedCodeChunk("package main\nimport \"fmt\"\n\nfunc main() {}\n"),
	}, FormatStreamChunks(input))
}

func stringsJoin(lines ...string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}

	return out
}

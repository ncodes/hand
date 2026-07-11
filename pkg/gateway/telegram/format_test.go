package telegram

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatMarkdownV2_FormatsCommonMarkdown(t *testing.T) {
	input := stringsJoin(
		"# Heading",
		"**standard bold** and *telegram bold*",
		"_italic_ and __underlined__",
		"[Morph](https://example.com/docs)",
		"[Nedy](tg://user?id=123456789)",
		"`code.value`",
		"~~gone~~ and ~deleted~ and ||secret||",
		">quote!",
	)

	require.Equal(t, stringsJoin(
		"*Heading*",
		"*standard bold* and *telegram bold*",
		"_italic_ and __underlined__",
		"[Morph](https://example.com/docs)",
		"[Nedy](tg://user?id=123456789)",
		"`code.value`",
		"~gone~ and ~deleted~ and ||secret||",
		"> quote\\!",
	), FormatMarkdownV2(input))
}

func TestFormatMarkdownV2_ReturnsEmptyText(t *testing.T) {
	require.Empty(t, FormatMarkdownV2(""))
}

func TestFormatMarkdownV2_EscapesPlainText(t *testing.T) {
	require.Equal(t,
		`hello \(world\) \+ price \= 5\.00\!`,
		FormatMarkdownV2("hello (world) + price = 5.00!"),
	)
}

func TestFormatMarkdownV2_PreservesExistingEscapes(t *testing.T) {
	input := `hello \(world\) \+ price \= 5\.00\!`

	require.Equal(t, input, FormatMarkdownV2(input))
}

func TestFormatMarkdownV2_PreservesFencedCode(t *testing.T) {
	input := "```go\nfmt.Println(`hi`)\n```"

	require.Equal(t, "```go\nfmt.Println(\\`hi\\`)\n```", FormatMarkdownV2(input))
}

func TestFormatMarkdownV2_PreservesExpandableBlockQuote(t *testing.T) {
	input := stringsJoin(
		"**>Expandable quote visible line",
		">Hidden expandable quote line one",
		">Hidden expandable quote line two||",
	)

	require.Equal(t, input, FormatMarkdownV2(input))
}

func TestPlainTextFromMarkdownV2_RemovesCommonMarkdown(t *testing.T) {
	input := "# Heading\n**bold** and *telegram bold*\n_italic_ and __underlined__\n~deleted~\n[Morph](https://example.com)\n`code`"

	require.Equal(t,
		"Heading\nbold and telegram bold\nitalic and underlined\ndeleted\nMorph (https://example.com)\ncode",
		PlainTextFromMarkdownV2(input),
	)
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

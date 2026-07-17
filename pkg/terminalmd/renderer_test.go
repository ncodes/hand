package terminalmd

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestRenderer_RendersHeadingsParagraphsAndBlockquotes(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 48}).Render(strings.Join([]string{
		"# Conflict / Geopolitics",
		"",
		"A short paragraph with **strong** text.",
		"",
		"> quoted context",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "● Conflict / Geopolitics")
	require.Contains(t, plain, "A short paragraph with strong text.")
	require.Contains(t, plain, "│ quoted context")
	require.NotContains(t, plain, "# Conflict")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderer_RendersHeadingHierarchyWithoutMarkdownMarkers(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(strings.Join([]string{
		"# Getting Started with Markdown",
		"",
		"## Basic Syntax",
		"",
		"### Headings",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "● Getting Started with Markdown")
	require.Contains(t, plain, "Basic Syntax")
	require.Contains(t, plain, "Headings")
	require.NotContains(t, plain, "# Getting Started")
	require.NotContains(t, plain, "## Basic")
	require.NotContains(t, plain, "### Headings")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderer_RendersBulletListsWithHangingIndent(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 54}).Render(strings.Join([]string{
		"- Court nullifies INEC's membership deadline - A Federal High Court ruling that INEC cannot shorten the statutory period.",
		"- Otedola to invest $100m in Dangote Refinery - Billionaire backs a private placement.",
	}, "\n"))
	require.NoError(t, err)

	lines := strings.Split(xansi.Strip(rendered), "\n")
	firstBullet := indexLineContaining(lines, "Court nullifies")
	firstContinuation := indexLineContaining(lines, "ruling that INEC")
	secondBullet := indexLineContaining(lines, "Otedola to invest")
	require.NotEqual(t, -1, firstBullet)
	require.NotEqual(t, -1, firstContinuation)
	require.NotEqual(t, -1, secondBullet)
	require.Greater(t, countLeadingSpaces(lines[firstContinuation]), countLeadingSpaces(lines[firstBullet]))
	require.Equal(t, countLeadingSpaces(lines[firstBullet]), countLeadingSpaces(lines[secondBullet]))
}

func TestRenderer_RendersUnicodeBulletArtifactsAsMarkdownLists(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 58}).Render(strings.Join([]string{
		"• **Court nullifies INEC's membership deadline** – A Federal High Court ruling that wraps onto another line.",
		"• **Otedola invests** – Billionaire backs a placement.",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "• Court nullifies INEC's membership deadline")
	require.Contains(t, plain, "• Otedola invests")
	require.NotContains(t, plain, "**Court")
	require.NotContains(t, plain, "**Otedola")

	lines := strings.Split(plain, "\n")
	firstBullet := indexLineContaining(lines, "Court nullifies")
	firstContinuation := indexLineContaining(lines, "another line.")
	secondBullet := indexLineContaining(lines, "Otedola invests")
	require.NotEqual(t, -1, firstBullet)
	require.NotEqual(t, -1, firstContinuation)
	require.NotEqual(t, -1, secondBullet)
	require.Greater(t, countLeadingSpaces(lines[firstContinuation]), countLeadingSpaces(lines[firstBullet]))
	require.Equal(t, countLeadingSpaces(lines[firstBullet]), countLeadingSpaces(lines[secondBullet]))
}

func TestRenderer_RendersNestedUnorderedListsWithDepthMarkers(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(strings.Join([]string{
		"- Item one",
		"- Item two",
		"- Item three",
		"  - Nested item A",
		"  - Nested item B",
		"    - Deeply nested",
	}, "\n"))
	require.NoError(t, err)

	lines := strings.Split(xansi.Strip(rendered), "\n")
	require.Contains(t, lines, "• Item one")
	require.Contains(t, lines, "• Item two")
	require.Contains(t, lines, "• Item three")
	require.Contains(t, lines, "  ◦ Nested item A")
	require.Contains(t, lines, "  ◦ Nested item B")
	require.Contains(t, lines, "    ▪ Deeply nested")
}

func TestRenderer_RendersNumberedAndTaskLists(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 40}).Render(strings.Join([]string{
		"1. First ordered task with enough text to wrap cleanly",
		"2. Second ordered task",
		"",
		"- [x] Done item",
		"- [ ] Pending item",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "1. First ordered task")
	require.Contains(t, plain, "2. Second ordered task")
	require.Contains(t, plain, "[x] Done item")
	require.Contains(t, plain, "[ ] Pending item")
}

func TestRenderer_RendersInlineCodeBoldItalicAndLinks(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render("Use `go test` with **bold**, *italic*, and [docs](https://example.com).")
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Use go test with bold, italic, and docs.")
	require.Contains(t, rendered, "\x1b[")
	require.NotContains(t, rendered, "\x1b]8;;")
}

func TestRenderer_RendersClickableLinksWhenEnabled(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80, EnableHyperlinks: true}).Render("Read [docs](https://example.com/path?q=1) or https://example.org.")
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Read docs or https://example.org.")
	require.Contains(t, rendered, "\x1b]8;;https://example.com/path?q=1\a")
	require.Contains(t, rendered, "\x1b]8;;https://example.org\a")
	require.Equal(t, 2, strings.Count(rendered, "\x1b]8;;\a"))
}

func TestRenderer_DoesNotRenderUnsafeOrRelativeLinksAsTerminalHyperlinks(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80, EnableHyperlinks: true}).Render("Use [local](/guide) and [bad](https://example.com/\x1b\a).")
	require.NoError(t, err)

	require.Contains(t, xansi.Strip(rendered), "Use local and bad.")
	require.NotContains(t, rendered, "\x1b]8;;/guide")
	require.Contains(t, rendered, "\x1b]8;;https://example.com/\a")
}

func TestRenderer_RendersGFMInlineArtifacts(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render("Use ~~old~~, ![diagram](diagram.png), and <strong>HTML text</strong><br>next.")
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Use old, diagram, and HTML text")
	require.Contains(t, plain, "next.")
	require.NotContains(t, plain, "~~old~~")
	require.NotContains(t, plain, "![diagram]")
	require.NotContains(t, plain, "<strong>")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderer_RendersEscapedMarkdownPunctuationAsText(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(`Use \*literal\* brackets \[x\].`)
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Use *literal* brackets [x].")
	require.NotContains(t, plain, `\*literal\*`)
	require.NotContains(t, plain, `\[x\]`)
}

func TestRenderer_RendersFencedCodeWithChromaHighlighting(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render("```go\nfmt.Println(\"hi\")\n```")
	require.NoError(t, err)

	require.Contains(t, xansi.Strip(rendered), `fmt.Println("hi")`)
	require.Contains(t, rendered, "\x1b[")
	require.NotContains(t, rendered, "```")
}

func TestRenderer_RendersMermaidFencesAsDiagramBlocks(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(strings.Join([]string{
		"```mermaid",
		"flowchart LR",
		"  A[Start] --> B{Ready?}",
		"  B -->|yes| C[Ship]",
		"```",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Mermaid source (visual render unavailable)")
	require.Contains(t, plain, "flowchart LR")
	require.Contains(t, plain, "A[Start] --> B{Ready?}")
	require.Contains(t, plain, "B -->|yes| C[Ship]")
	require.NotContains(t, plain, "```mermaid")
}

func TestRenderer_RendersMermaidFencesThroughGoldmarkMermaid(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(strings.Join([]string{
		"Before",
		"",
		"```mermaid",
		"sequenceDiagram",
		"  Alice->>Bob: Hello",
		"```",
		"",
		"After",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Before")
	require.Contains(t, plain, "Mermaid source (visual render unavailable)")
	require.Contains(t, plain, "sequenceDiagram")
	require.Contains(t, plain, "Alice->>Bob: Hello")
	require.Contains(t, plain, "After")
	require.NotContains(t, plain, "```mermaid")
	require.NotContains(t, plain, "<script")
}

func TestRenderer_RendersUnfencedMermaidBlocks(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render(strings.Join([]string{
		"Here's the Mermaid diagram in a copyable format:",
		"",
		"flowchart LR",
		"  A[User Login] --> B{Validate Credentials}",
		"  B -->|Valid| C[Grant Access]",
		"  B -->|Invalid| D[Show Error]",
		"",
		"Done.",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "Here's the Mermaid diagram in a copyable format:")
	require.Contains(t, plain, "Mermaid source (visual render unavailable)")
	require.Contains(t, plain, "flowchart LR")
	require.Contains(t, plain, "A[User Login] --> B{Validate Credentials}")
	require.Contains(t, plain, "B -->|Valid| C[Grant Access]")
	require.Contains(t, plain, "B -->|Invalid| D[Show Error]")
	require.Contains(t, plain, "Done.")
	require.NotContains(t, plain, "```mermaid")
}

func TestRenderer_DoesNotParseTablesInsideFencedCode(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 120}).Render(strings.Join([]string{
		"```",
		"| A | B |",
		"| --- | --- |",
		"| C | D |",
		"```",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "| A | B |")
	require.Contains(t, plain, "| --- | --- |")
	require.NotContains(t, plain, "┌")
	require.NotContains(t, plain, "│ A")
}

func TestRenderer_RendersCompactTables(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 120}).Render(strings.Join([]string{
		"| **Issue** | Details |",
		"| --- | --- |",
		"| [One](https://example.com) | `Short` |",
		"| Two | Also **short** |",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "┌───────┬────────────┐")
	require.Contains(t, plain, "│ Issue │ Details    │")
	require.Contains(t, plain, "│ Two   │ Also short │")
	require.Contains(t, plain, "└───────┴────────────┘")
}

func TestRenderer_RendersTableCellsWithLiteralPipes(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 120}).Render(strings.Join([]string{
		"| Pattern | Meaning |",
		"| --- | --- |",
		"| `foo|bar` | inline code pipe |",
		`| alpha \| beta | escaped pipe |`,
		`| \*literal\* | escaped markdown |`,
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.Contains(t, plain, "│ foo|bar      │ inline code pipe │")
	require.Contains(t, plain, "│ alpha | beta │ escaped pipe     │")
	require.Contains(t, plain, "│ *literal*    │ escaped markdown │")
	require.NotContains(t, plain, "│ foo         │ bar")
}

func TestRenderer_RendersWideTablesAsLabeledRows(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 60}).Render(strings.Join([]string{
		"| Source | Story |",
		"| --- | --- |",
		"| CNN | **Iran rebuilding military faster than expected** - US intelligence finds Iran is restarting drone production during the ceasefire. |",
		"| BBC | US sends 5,000 troops to Poland as tensions remain high. |",
	}, "\n"))
	require.NoError(t, err)

	plain := xansi.Strip(rendered)
	require.NotContains(t, plain, "┌")
	require.NotContains(t, plain, "│")
	require.Contains(t, plain, "Source: CNN")
	require.Contains(t, plain, "Story: Iran rebuilding military faster than expected")
	require.Contains(t, plain, "Source: BBC")
}

func TestRenderer_RendersEmoji(t *testing.T) {
	rendered, err := NewRenderer(Options{Width: 80}).Render("Ship it :rocket:")
	require.NoError(t, err)

	require.Contains(t, xansi.Strip(rendered), "Ship it 🚀")
}

func indexLineContaining(lines []string, value string) int {
	for index, line := range lines {
		if strings.Contains(line, value) {
			return index
		}
	}

	return -1
}

func countLeadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRenderTranscriptCell_RendersAssistantMarkdown(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(
		assistantTranscriptCell{text: "# Title\n\n## Key Complications\n\n### What Could Happen Next\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```"},
		60,
	)
	plain := stripANSI(rendered)

	require.NotContains(t, plain, "Morph:")
	require.Contains(t, plain, "● Title")
	require.Contains(t, plain, "Key Complications")
	require.Contains(t, plain, "What Could Happen Next")
	require.Contains(t, plain, "first")
	require.Contains(t, plain, "second")
	require.Contains(t, plain, `fmt.Println("hi")`)
	require.NotContains(t, plain, "# Title")
	require.NotContains(t, plain, "## Key Complications")
	require.NotContains(t, plain, "### What Could Happen Next")
	require.NotContains(t, plain, "```")
	require.Contains(t, rendered, "\x1b[")
	require.NotContains(t, rendered, "\x1b[38;5;39m")
	require.NotContains(t, rendered, "\x1b[48;5;63m")
}

func TestRenderTranscriptCells_AlignsAssistantMarkdownWithThoughtCell(t *testing.T) {
	rendered := renderTranscriptCellsWithWidth([]transcriptCell{
		thoughtTranscriptCell{duration: time.Second},
		assistantTranscriptCell{text: "**54 sensors are working.**\n\nRechecked: 9 containers."},
	}, 80)
	lines := assistantTranscriptBodyLines(stripANSI(rendered))

	thoughtLine := indexLineContaining(lines, "Thought for 1s")
	answerLine := indexLineContaining(lines, "54 sensors are working.")
	require.NotEqual(t, -1, thoughtLine)
	require.NotEqual(t, -1, answerLine)
	require.Equal(t, countLeadingSpaces(lines[thoughtLine]), countLeadingSpaces(lines[answerLine]))
}

func TestRenderTranscriptCell_IndentsWrappedMarkdownListContinuations(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"- Court nullifies INEC's membership deadline - A Federal High Court ruling that INEC cannot shorten the statutory 120-day period.",
		"- Otedola to invest $100m in Dangote Refinery - Billionaire backs a billion private placement.",
	}, "\n")}, 54)
	lines := strings.Split(stripANSI(rendered), "\n")

	firstBullet := indexLineContaining(lines, "Court nullifies")
	firstContinuation := indexLineContaining(lines, "ruling that INEC")
	secondBullet := indexLineContaining(lines, "Otedola to invest")
	require.NotEqual(t, -1, firstBullet)
	require.NotEqual(t, -1, firstContinuation)
	require.NotEqual(t, -1, secondBullet)
	require.Greater(t, countLeadingSpaces(lines[firstContinuation]), countLeadingSpaces(lines[firstBullet]))
	require.LessOrEqual(t, countLeadingSpaces(lines[secondBullet]), countLeadingSpaces(lines[firstContinuation]))
}

func TestRenderTranscriptCell_RendersUnicodeBulletMarkdownArtifacts(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"• **Court nullifies INEC's membership deadline** – A Federal High Court ruling that wraps onto another line.",
		"• **Otedola invests** – Billionaire backs a placement.",
	}, "\n")}, 58)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "• Court nullifies INEC's membership deadline")
	require.Contains(t, plain, "• Otedola invests")
	require.NotContains(t, plain, "**Court")
	require.NotContains(t, plain, "**Otedola")
}

func TestRenderTranscriptCell_RendersCompactMarkdownTables(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"| **Issue** | Details |",
		"| --- | --- |",
		"| [One](https://example.com) | `Short` |",
		"| Two | Also **short** |",
	}, "\n")}, 120)
	plain := stripANSI(rendered)
	lines := assistantTranscriptBodyLines(plain)

	require.Contains(t, plain, "┌───────┬────────────┐")
	require.Contains(t, plain, "│ Issue │ Details    │")
	require.Contains(t, plain, "├───────┼────────────┤")
	require.Contains(t, plain, "│ Two   │ Also short │")
	require.Contains(t, plain, "└───────┴────────────┘")
	require.Equal(t, 1, strings.Count(plain, "├───────┼────────────┤"))
	require.NotContains(t, plain, strings.Repeat(" ", 20))
	require.Contains(t, rendered, "\x1b[")
	for _, line := range lines {
		if strings.Contains(line, "│ Issue") {
			require.False(t, strings.HasPrefix(line, " "))
			require.Less(t, len(line), 40)
		}
	}
}

func TestRenderTranscriptCell_DetectsUnfencedMermaidFlowchart(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"flowchart LR",
		"  A[Start] --> B{Decision}",
		"  B -->|Yes| C[Action 1]",
		"  B -->|No| D[Action 2]",
	}, "\n")}, 80)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "Mermaid source (visual render unavailable)")
	require.Contains(t, plain, "flowchart LR")
	require.Contains(t, plain, "A[Start] --> B{Decision}")
	require.Contains(t, plain, "B -->|Yes| C[Action 1]")
}

func TestRenderTranscriptCell_RendersWideMarkdownTablesAsLabeledRows(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"| Source | Story |",
		"| --- | --- |",
		"| CNN | **Iran rebuilding military faster than expected** - US intelligence finds Iran is restarting drone production during the ceasefire. |",
		"| BBC | US sends 5,000 troops to Poland as tensions remain high. |",
	}, "\n")}, 60)
	plain := stripANSI(rendered)

	require.NotContains(t, plain, "┌")
	require.NotContains(t, plain, "│")
	require.Contains(t, plain, "Source: CNN")
	require.Contains(t, plain, "Story: Iran rebuilding military faster than expected")
	require.Contains(t, plain, "Source: BBC")
	require.Contains(t, rendered, "\x1b[")
	for _, line := range assistantTranscriptBodyLines(plain) {
		if strings.Contains(line, "Source:") || strings.Contains(line, "Story:") {
			require.False(t, strings.HasPrefix(line, " "))
		}
	}
}

func assistantTranscriptBodyLines(text string) []string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		switch {
		case strings.HasPrefix(line, "● "):
			lines[index] = strings.TrimPrefix(line, "● ")
		case strings.HasPrefix(line, "  "):
			lines[index] = strings.TrimPrefix(line, "  ")
		}
	}

	return lines
}

func TestRenderTranscriptCell_KeepsTableCloseToPrecedingHeading(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"## Key Complications",
		"",
		"| Issue | Details |",
		"| --- | --- |",
		"| One | Short |",
	}, "\n")}, 120)
	lines := strings.Split(stripANSI(rendered), "\n")
	headingIndex := indexLineContaining(lines, "Key Complications")
	tableIndex := indexLineContaining(lines, "┌───────┬─────────┐")

	require.NotEqual(t, -1, headingIndex)
	require.NotEqual(t, -1, tableIndex)
	require.LessOrEqual(t, tableIndex-headingIndex, 2)
}

func TestRenderTranscriptCell_DoesNotRenderUserMarkdown(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(userTranscriptCell{text: "# literal\n\n- keep"}, 60)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "❯ # literal")
	require.Contains(t, plain, "  - keep")
	require.Equal(t, 1, strings.Count(plain, "❯"))
}

func TestRenderMarkdownForTranscript_LeavesPlainTextAlone(t *testing.T) {
	require.Equal(t, "hello there", renderMarkdownForTranscript("hello there", 60))
}

func TestRenderMarkdownForTranscript_RendersClickableLinks(t *testing.T) {
	rendered := renderMarkdownForTranscript("[docs](https://example.com)", 60)

	require.Contains(t, stripANSI(rendered), "docs")
	require.Contains(t, rendered, "\x1b]8;;https://example.com\a")
	require.Contains(t, rendered, "\x1b]8;;\a")
}

func TestRenderMarkdownForTranscript_RendersBareURLsAsClickableLinks(t *testing.T) {
	rendered := renderMarkdownForTranscript("Read https://example.com/docs for details.", 80)

	require.Contains(t, stripANSI(rendered), "Read https://example.com/docs for details.")
	require.Contains(t, rendered, "\x1b]8;;https://example.com/docs\a")
	require.Contains(t, rendered, "\x1b]8;;\a")
}

func TestRenderTranscriptCells_PreservesClickableLinksThroughViewportContent(t *testing.T) {
	runModel := newModel()
	runModel.width = 100
	runModel.height = 30
	runModel.transcript.SetWidth(100)
	runModel.transcript.SetHeight(20)
	runModel.messages = []transcriptCell{
		assistantTranscriptCell{text: "Read https://example.com/docs for details."},
	}

	runModel.setTranscriptContent()

	require.Contains(t, runModel.transcript.GetContent(), "\x1b]8;;https://example.com/docs\a")
	require.Contains(t, runModel.transcript.View(), "\x1b]8;;https://example.com/docs\a")
}

func TestHasTranscriptMarkdown_DetectsCommonSyntax(t *testing.T) {
	require.True(t, hasTranscriptMarkdown("1. first"))
	require.True(t, hasTranscriptMarkdown("• first"))
	require.True(t, hasTranscriptMarkdown("**strong**"))
	require.True(t, hasTranscriptMarkdown("~~old~~"))
	require.True(t, hasTranscriptMarkdown("[link](https://example.com)"))
	require.True(t, hasTranscriptMarkdown("https://example.com"))
	require.True(t, hasTranscriptMarkdown("Read https://example.com"))
	require.True(t, hasTranscriptMarkdown("<strong>important</strong>"))
	require.False(t, hasTranscriptMarkdown("plain sentence"))
	require.False(t, isOrderedMarkdownListItem("1a. bad"))
}

func indexLineContaining(lines []string, value string) int {
	for index, line := range lines {
		if strings.Contains(line, value) {
			return index
		}
	}

	return -1
}

func countLeadingSpaces(value string) int {
	count := 0
	for _, char := range value {
		if char != ' ' {
			return count
		}
		count++
	}
	return count
}

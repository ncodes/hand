package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

type stubTranscriptMarkdownRenderer struct {
	output string
	err    error
}

func (r stubTranscriptMarkdownRenderer) Render(string) (string, error) {
	return r.output, r.err
}

func withStubTranscriptMarkdownRenderer(
	t *testing.T,
	renderer transcriptMarkdownRenderer,
	err error,
) {
	t.Helper()
	previous := newTranscriptMarkdownRenderer
	newTranscriptMarkdownRenderer = func(int) (transcriptMarkdownRenderer, error) {
		return renderer, err
	}
	t.Cleanup(func() {
		newTranscriptMarkdownRenderer = previous
	})
}

func TestRenderTranscriptCell_RendersAssistantMarkdown(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(
		assistantTranscriptCell{text: "# Title\n\n## Key Complications\n\n### What Could Happen Next\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```"},
		60,
	)
	plain := stripANSI(rendered)

	require.NotContains(t, plain, "Hand:")
	require.Contains(t, plain, "Title")
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
	lines := strings.Split(stripANSI(rendered), "\n")

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
	require.Equal(t, countLeadingSpaces(lines[firstBullet]), countLeadingSpaces(lines[secondBullet]))
}

func TestRenderTranscriptCell_RendersCompactMarkdownTables(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"| **Issue** | Details |",
		"| --- | --- |",
		"| [One](https://example.com) | `Short` |",
		"| Two | Also **short** |",
	}, "\n")}, 120)
	plain := stripANSI(rendered)
	lines := strings.Split(plain, "\n")

	require.Contains(t, plain, "┌───────┬────────────┐")
	require.Contains(t, plain, "│ Issue │ Details    │")
	require.Contains(t, plain, "├───────┼────────────┤")
	require.Contains(t, plain, "│ Two   │ Also short │")
	require.Contains(t, plain, "└───────┴────────────┘")
	require.Equal(t, 2, strings.Count(plain, "├───────┼────────────┤"))
	require.NotContains(t, plain, strings.Repeat(" ", 20))
	require.Contains(t, rendered, "\x1b[")
	for _, line := range lines {
		if strings.Contains(line, "│ Issue") {
			require.False(t, strings.HasPrefix(line, " "))
			require.Less(t, len(line), 40)
		}
	}
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
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "Source:") || strings.Contains(line, "Story:") {
			require.False(t, strings.HasPrefix(line, " "))
		}
	}
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

func TestRenderMarkdownForTranscript_FallsBackWhenRendererFails(t *testing.T) {
	withStubTranscriptMarkdownRenderer(t, nil, errors.New("boom"))

	require.Equal(t, "**hello**", renderMarkdownForTranscript("**hello**", 60))
	_, err := glamourRenderMarkdown("**hello**", 60)
	require.ErrorContains(t, err, "boom")
}

func TestRenderMarkdownForTranscript_FallsBackWhenRendererReturnsBlank(t *testing.T) {
	withStubTranscriptMarkdownRenderer(t, stubTranscriptMarkdownRenderer{output: "\n\n"}, nil)

	require.Equal(t, "**hello**", renderMarkdownForTranscript("**hello**", 60))
}

func TestRenderMarkdownWithCompactTables_FallsBackWhenMarkdownChunkRenderFails(t *testing.T) {
	withStubTranscriptMarkdownRenderer(t, nil, errors.New("boom"))

	rendered := renderMarkdownWithCompactTables(strings.Join([]string{
		"## Heading",
		"",
		"| A |",
		"| --- |",
		"| B |",
	}, "\n"), 60)

	require.Contains(t, rendered, "## Heading")
	require.Contains(t, rendered, "┌───┐")
}

func TestMarkdownTrimHelpers_HandleNoopInputs(t *testing.T) {
	require.Equal(t, "no margin", removeCommonRenderedMarkdownLeftMargin("no margin"))
	require.Empty(t, trimLeadingSpaces("", 2))
	require.Equal(t, "already", trimLeadingSpaces("already", 0))
}

func TestHasTranscriptMarkdown_DetectsCommonSyntax(t *testing.T) {
	require.True(t, hasTranscriptMarkdown("1. first"))
	require.True(t, hasTranscriptMarkdown("**strong**"))
	require.True(t, hasTranscriptMarkdown("[link](https://example.com)"))
	require.False(t, hasTranscriptMarkdown("plain sentence"))
}

func TestMarkdownTableHelpers_HandleEdgeCases(t *testing.T) {
	require.Equal(t, "| only | one |", renderCompactMarkdownTable([]string{"| only | one |"}, 80))
	require.Zero(t, compactMarkdownTableWidth(nil))
	require.Equal(t, "", renderMarkdownTableAsLabeledRows([][]string{{"Source", "Story"}}))
	require.False(t, isMarkdownTableSeparator("| --- | nope |"))
	require.False(t, isOrderedMarkdownListItem("1a. bad"))
}

func TestRenderMarkdownTableAsLabeledRows_SkipsBlankHeadersAndValues(t *testing.T) {
	rendered := renderMarkdownTableAsLabeledRows([][]string{
		{"Source", "", "Story", "Empty"},
		{"CNN", "ignored", "Headline", ""},
		{"", "ignored", "", ""},
	})

	require.Equal(t, "Source: CNN\nStory: Headline", rendered)
}

func TestCompactMarkdownInlineRenderingLeavesMalformedSyntaxAlone(t *testing.T) {
	require.Equal(t, "prefix [label only", renderCompactMarkdownLinks("prefix [label only"))
	require.Equal(t, "[label](missing", renderCompactMarkdownLinks("[label](missing"))
	require.Equal(t, "**bold", renderCompactInlineDelimited("**bold", "**", lipgloss.NewStyle().Bold(true)))
	require.Equal(t, "plain", renderCompactInlineDelimited("plain", "**", lipgloss.NewStyle().Bold(true)))
}

func indexLineContaining(lines []string, value string) int {
	for index, line := range lines {
		if strings.Contains(line, value) {
			return index
		}
	}

	return -1
}

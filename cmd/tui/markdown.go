package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

const transcriptMarkdownMargin = 2

func renderMarkdownForTranscript(markdown string, width int) string {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" || !hasTranscriptMarkdown(markdown) {
		return markdown
	}
	if hasMarkdownTable(markdown) {
		return renderMarkdownWithCompactTables(markdown, width)
	}

	rendered, err := glamourRenderMarkdown(markdown, width)
	if err != nil {
		return markdown
	}
	if rendered = strings.TrimSpace(rendered); rendered != "" {
		return rendered
	}

	return markdown
}

func glamourRenderMarkdown(markdown string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(transcriptMarkdownStyle()),
		glamour.WithWordWrap(max(width, 1)),
	)
	if err != nil {
		return "", err
	}

	return renderer.Render(markdown)
}

func transcriptMarkdownStyle() glamouransi.StyleConfig {
	style := styles.DarkStyleConfig
	style.Heading.Color = nil
	clearHeadingPrefix(&style.H1)
	clearHeadingPrefix(&style.H2)
	clearHeadingPrefix(&style.H3)
	clearHeadingPrefix(&style.H4)
	clearHeadingPrefix(&style.H5)
	clearHeadingPrefix(&style.H6)

	return style
}

func clearHeadingPrefix(block *glamouransi.StyleBlock) {
	block.Prefix = ""
	block.Color = nil
	block.BackgroundColor = nil
}

func renderMarkdownWithCompactTables(markdown string, width int) string {
	lines := strings.Split(markdown, "\n")
	rendered := make([]string, 0, len(lines))
	markdownChunk := make([]string, 0, len(lines))
	flushMarkdown := func() {
		chunk := strings.TrimSpace(strings.Join(markdownChunk, "\n"))
		markdownChunk = markdownChunk[:0]
		if chunk == "" {
			return
		}
		if output, err := glamourRenderMarkdown(chunk, width); err == nil && strings.TrimSpace(xansi.Strip(output)) != "" {
			rendered = append(rendered, trimRenderedMarkdown(output))
			return
		}
		rendered = append(rendered, chunk)
	}

	for index := 0; index < len(lines); {
		if !isMarkdownTableStart(lines, index) {
			markdownChunk = append(markdownChunk, lines[index])
			index++
			continue
		}

		flushMarkdown()
		tableEnd := index + 2
		for tableEnd < len(lines) && isMarkdownTableRow(lines[tableEnd]) {
			tableEnd++
		}
		rendered = append(rendered, indentMarkdownBlock(renderCompactMarkdownTable(lines[index:tableEnd])))
		index = tableEnd
	}
	flushMarkdown()

	return strings.Join(rendered, "\n\n")
}

func trimRenderedMarkdown(output string) string {
	lines := strings.Split(output, "\n")
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func indentMarkdownBlock(block string) string {
	padding := strings.Repeat(" ", transcriptMarkdownMargin)
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[index] = padding + line
		}
	}

	return strings.Join(lines, "\n")
}

func hasMarkdownTable(markdown string) bool {
	lines := strings.Split(markdown, "\n")
	for index := 0; index < len(lines)-1; index++ {
		if isMarkdownTableStart(lines, index) {
			return true
		}
	}

	return false
}

func isMarkdownTableStart(lines []string, index int) bool {
	return index+1 < len(lines) &&
		isMarkdownTableRow(lines[index]) &&
		isMarkdownTableSeparator(lines[index+1])
}

func isMarkdownTableRow(line string) bool {
	line = strings.TrimSpace(line)

	return strings.Contains(line, "|") && strings.Trim(line, "| ") != ""
}

func isMarkdownTableSeparator(line string) bool {
	cells := splitMarkdownTableRowRaw(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, " :-")
		if cell != "" {
			return false
		}
	}

	return true
}

func renderCompactMarkdownTable(lines []string) string {
	if len(lines) < 2 {
		return strings.Join(lines, "\n")
	}

	rows := make([][]string, 0, len(lines)-1)
	rows = append(rows, splitMarkdownTableRow(lines[0]))
	for _, line := range lines[2:] {
		rows = append(rows, splitMarkdownTableRow(line))
	}

	columnCount := 0
	for _, row := range rows {
		columnCount = max(columnCount, len(row))
	}
	if columnCount == 0 {
		return strings.Join(lines, "\n")
	}

	widths := make([]int, columnCount)
	for _, row := range rows {
		for index := range columnCount {
			if index < len(row) {
				widths[index] = max(widths[index], xansi.StringWidth(row[index]))
			}
		}
	}

	rendered := make([]string, 0, len(rows)*2+1)
	rendered = append(rendered, renderCompactMarkdownTableBorder(widths, "┌", "┬", "┐"))
	for rowIndex, row := range rows {
		rendered = append(rendered, renderCompactMarkdownTableRow(row, widths))
		if rowIndex < len(rows)-1 {
			rendered = append(rendered, renderCompactMarkdownTableBorder(widths, "├", "┼", "┤"))
		}
	}
	rendered = append(rendered, renderCompactMarkdownTableBorder(widths, "└", "┴", "┘"))

	return strings.Join(rendered, "\n")
}

func splitMarkdownTableRow(line string) []string {
	cells := splitMarkdownTableRowRaw(line)
	for index, cell := range cells {
		cells[index] = renderCompactMarkdownTableCell(cell)
	}

	return cells
}

func splitMarkdownTableRowRaw(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	cells := strings.Split(line, "|")
	for index, cell := range cells {
		cells[index] = strings.TrimSpace(cell)
	}

	return cells
}

func renderCompactMarkdownTableCell(cell string) string {
	cell = strings.TrimSpace(cell)
	cell = renderCompactMarkdownLinks(cell)
	cell = renderCompactInlineDelimited(cell, "**", lipgloss.NewStyle().Bold(true))
	cell = renderCompactInlineDelimited(cell, "__", lipgloss.NewStyle().Bold(true))
	cell = renderCompactInlineDelimited(cell, "`", lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("#1A1A1A")))

	return cell
}

func renderCompactMarkdownLinks(value string) string {
	var output strings.Builder
	for {
		labelStart := strings.Index(value, "[")
		if labelStart < 0 {
			output.WriteString(value)
			break
		}
		labelEnd := strings.Index(value[labelStart+1:], "](")
		if labelEnd < 0 {
			output.WriteString(value)
			break
		}
		hrefStart := labelStart + 1 + labelEnd + len("](")
		hrefEnd := strings.Index(value[hrefStart:], ")")
		if hrefEnd < 0 {
			output.WriteString(value)
			break
		}

		labelEnd = labelStart + 1 + labelEnd
		hrefEnd += hrefStart
		output.WriteString(value[:labelStart])
		output.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Underline(true).
			Render(value[labelStart+1 : labelEnd]))
		value = value[hrefEnd+1:]
	}

	return output.String()
}

func renderCompactInlineDelimited(value string, delimiter string, style lipgloss.Style) string {
	var output strings.Builder
	for {
		start := strings.Index(value, delimiter)
		if start < 0 {
			output.WriteString(value)
			break
		}
		end := strings.Index(value[start+len(delimiter):], delimiter)
		if end < 0 {
			output.WriteString(value)
			break
		}

		output.WriteString(value[:start])
		contentStart := start + len(delimiter)
		contentEnd := contentStart + end
		output.WriteString(style.Render(value[contentStart:contentEnd]))
		value = value[contentEnd+len(delimiter):]
	}

	return output.String()
}

func renderCompactMarkdownTableRow(row []string, widths []int) string {
	cells := make([]string, len(widths))
	for index, width := range widths {
		cell := ""
		if index < len(row) {
			cell = row[index]
		}
		cells[index] = cell + strings.Repeat(" ", max(width-xansi.StringWidth(cell), 0))
	}

	return "│ " + strings.Join(cells, " │ ") + " │"
}

func renderCompactMarkdownTableBorder(widths []int, left string, separator string, right string) string {
	cells := make([]string, len(widths))
	for index, width := range widths {
		cells[index] = strings.Repeat("─", width+2)
	}

	return left + strings.Join(cells, separator) + right
}

func hasTranscriptMarkdown(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#"),
			strings.HasPrefix(line, "- "),
			strings.HasPrefix(line, "* "),
			strings.HasPrefix(line, "+ "),
			strings.HasPrefix(line, "> "),
			strings.HasPrefix(line, "```"),
			strings.HasPrefix(line, "|"),
			isOrderedMarkdownListItem(line):
			return true
		}
	}

	return strings.Contains(value, "**") ||
		strings.Contains(value, "__") ||
		strings.Contains(value, "`") ||
		strings.Contains(value, "](")
}

func isOrderedMarkdownListItem(line string) bool {
	dot := strings.Index(line, ". ")
	if dot <= 0 {
		return false
	}
	for _, char := range line[:dot] {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

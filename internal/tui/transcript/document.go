package transcript

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RenderedDocument describes rendered document.
type RenderedDocument struct {
	Content   string
	PlainText string
	Lines     []RenderedLine
}

// RenderedLine describes rendered line.
type RenderedLine struct {
	Text        string
	PlainText   string
	StartOffset int
	EndOffset   int
}

// NewRenderedDocument returns a rendered transcript document with line offsets indexed.
func NewRenderedDocument(content string) RenderedDocument {
	plainText := ansi.Strip(content)
	plainLines := strings.Split(plainText, "\n")
	renderedLines := strings.Split(content, "\n")
	lines := make([]RenderedLine, 0, len(plainLines))

	offset := 0
	for index, plainLine := range plainLines {
		renderedLine := ""
		if index < len(renderedLines) {
			renderedLine = renderedLines[index]
		}
		end := offset + len(plainLine)
		lines = append(lines, RenderedLine{
			Text:        renderedLine,
			PlainText:   plainLine,
			StartOffset: offset,
			EndOffset:   end,
		})

		offset = end
		if index < len(plainLines)-1 {
			offset++
		}
	}

	return RenderedDocument{
		Content:   content,
		PlainText: plainText,
		Lines:     lines,
	}
}

func (doc RenderedDocument) PlainLines() []string {
	lines := make([]string, len(doc.Lines))
	for index, line := range doc.Lines {
		lines[index] = line.PlainText
	}

	return lines
}

func (doc RenderedDocument) PlainRange(start int, end int) string {
	if start > end {
		start, end = end, start
	}
	if start == end || start >= len(doc.PlainText) {
		return ""
	}
	if start < 0 {
		start = 0
	}
	if end > len(doc.PlainText) {
		end = len(doc.PlainText)
	}

	return doc.PlainText[start:end]
}

func (doc RenderedDocument) Line(index int) (RenderedLine, bool) {
	if index < 0 || index >= len(doc.Lines) {
		return RenderedLine{}, false
	}

	return doc.Lines[index], true
}

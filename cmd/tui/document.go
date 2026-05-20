package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type renderedTranscriptDocument struct {
	Content   string
	PlainText string
	Lines     []renderedTranscriptLine
}

type renderedTranscriptLine struct {
	Text        string
	PlainText   string
	StartOffset int
	EndOffset   int
}

func newRenderedTranscriptDocument(content string) renderedTranscriptDocument {
	plainText := ansi.Strip(content)
	plainLines := strings.Split(plainText, "\n")
	renderedLines := strings.Split(content, "\n")
	lines := make([]renderedTranscriptLine, 0, len(plainLines))

	offset := 0
	for index, plainLine := range plainLines {
		renderedLine := ""
		if index < len(renderedLines) {
			renderedLine = renderedLines[index]
		}
		end := offset + len(plainLine)
		lines = append(lines, renderedTranscriptLine{
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

	return renderedTranscriptDocument{
		Content:   content,
		PlainText: plainText,
		Lines:     lines,
	}
}

func (doc renderedTranscriptDocument) PlainLines() []string {
	lines := make([]string, len(doc.Lines))
	for index, line := range doc.Lines {
		lines[index] = line.PlainText
	}

	return lines
}

func (doc renderedTranscriptDocument) PlainRange(start int, end int) string {
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

func (doc renderedTranscriptDocument) Line(index int) (renderedTranscriptLine, bool) {
	if index < 0 || index >= len(doc.Lines) {
		return renderedTranscriptLine{}, false
	}

	return doc.Lines[index], true
}

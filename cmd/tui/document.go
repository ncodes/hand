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

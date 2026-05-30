package tui

import tuitranscript "github.com/wandxy/hand/internal/tui/transcript"

type renderedTranscriptDocument = tuitranscript.RenderedDocument

func newRenderedTranscriptDocument(content string) renderedTranscriptDocument {
	return tuitranscript.NewRenderedDocument(content)
}

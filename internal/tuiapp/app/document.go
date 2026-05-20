package tui

import tuitranscript "github.com/wandxy/hand/internal/tuiapp/transcript"

type renderedTranscriptDocument = tuitranscript.RenderedDocument
type renderedTranscriptLine = tuitranscript.RenderedLine

func newRenderedTranscriptDocument(content string) renderedTranscriptDocument {
	return tuitranscript.NewRenderedDocument(content)
}

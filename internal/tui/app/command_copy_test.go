package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestModel_UpdateCopiesTranscriptToClipboard(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}, assistantTranscriptCell{text: "hi"}}
	runModel.setTranscriptContent()
	runModel.input.SetValue("/copy")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "You: hello\n\nHand: hi", copied)
	require.Equal(t, "transcript copied", runModel.status.Text())
	require.Empty(t, runModel.input.Value())
}

func TestModel_UpdateCopiesTranscriptWithShortcut(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "shortcut"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "Hand: shortcut", copied)
	require.Equal(t, "transcript copied", runModel.status.Text())
}

func TestModel_CopyTranscriptReportsEmptyTranscript(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent(" \n\t ")

	cmd := runModel.copyTranscript()

	require.NotNil(t, cmd)
	require.Equal(t, "transcript is empty", runModel.status.Text())
}

func TestModel_TranscriptTextIncludesLiveAssistantCell(t *testing.T) {
	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.live = assistantTranscriptCell{text: "streaming"}

	require.Equal(t, "You: hello\n\nHand: streaming", runModel.transcriptText())
}

func TestModel_TranscriptTextFallsBackToViewportContent(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent("  saved viewport  ")

	require.Equal(t, "saved viewport", runModel.transcriptText())
}

func TestModel_UpdateReportsClipboardFailure(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	writeClipboard = func(string) error {
		return errors.New("clipboard unavailable")
	}
	runModel := newModel()
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "hi"}}
	runModel.setTranscriptContent()
	runModel.input.SetValue("/copy")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	require.Equal(t, "copy failed", updated.(model).status.Text())
}

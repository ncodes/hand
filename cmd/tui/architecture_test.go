package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTUIAction_SetViewportSizeBoundsValues(t *testing.T) {
	state := newTUIState(nil, false)

	setViewportSizeAction{Width: 0, Height: -10}.apply(&state)

	require.Equal(t, 1, state.width)
	require.Equal(t, 1, state.height)
}

func TestTUIAction_TranscriptCellActions(t *testing.T) {
	state := newTUIState(nil, false)

	appendTranscriptCellAction{Cell: userTranscriptCell{text: "hello"}}.apply(&state)
	setLiveTranscriptCellAction{Cell: assistantTranscriptCell{text: "world"}}.apply(&state)

	require.Len(t, state.messages, 1)
	require.Equal(t, transcriptCellUser, state.messages[0].Kind())
	require.Equal(t, transcriptCellAssistant, state.live.Kind())

	clearTranscriptAction{}.apply(&state)

	require.Empty(t, state.messages)
	require.Nil(t, state.live)
	require.False(t, state.showIntro)
	require.Equal(t, -1, state.reasoningMessageIndex)
}

func TestTUIAction_SetSessionTitleFallsBackToDefault(t *testing.T) {
	state := newTUIState(nil, false)

	setSessionTitleAction{Title: "  Project Notes  "}.apply(&state)
	require.Equal(t, "Project Notes", state.sessionTitle)

	setSessionTitleAction{}.apply(&state)
	require.Equal(t, defaultSessionTitle, state.sessionTitle)
}

func TestTUILayout_ComputesStableRegions(t *testing.T) {
	layout := getTUILayout(100, 30, 2)

	require.Equal(t, 0, layout.Transcript.Y)
	require.Equal(t, 100, layout.Composer.Width)
	require.Equal(t, layout.Transcript.Height, layout.JumpToBottom.Y)
	require.Equal(t, layout.JumpToBottom.Y+transcriptComposerGapHeight, layout.Composer.Y)
	require.Equal(t, getPanelContentWidth(100), layout.PanelContentWidth)
}

func TestRenderedTranscriptDocument_StripsAnsiAndTracksOffsets(t *testing.T) {
	document := newRenderedTranscriptDocument("\x1b[31mHello\x1b[0m\nWorld")

	require.Equal(t, "Hello\nWorld", document.PlainText)
	require.Len(t, document.Lines, 2)
	require.Equal(t, "Hello", document.Lines[0].PlainText)
	require.Equal(t, 0, document.Lines[0].StartOffset)
	require.Equal(t, 5, document.Lines[0].EndOffset)
	require.Equal(t, "World", document.Lines[1].PlainText)
	require.Equal(t, 6, document.Lines[1].StartOffset)
	require.Equal(t, []string{"Hello", "World"}, document.PlainLines())
}

func TestTranscriptRenderer_GroupsAdjacentToolCells(t *testing.T) {
	rendered := defaultTranscriptRenderer.RenderCells(
		[]transcriptCell{
			toolTranscriptTestCell("call_1", "read_file", "read_file a.txt"),
			toolTranscriptTestCell("call_2", "read_file", "read_file b.txt"),
		},
		transcriptRenderContext{Width: 80},
	)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "Read")
	require.Contains(t, plain, "├ read_file a.txt")
	require.Contains(t, plain, "└ read_file b.txt")
	require.Equal(t, 1, strings.Count(plain, "Read"))
}

func TestTranscriptRenderer_UsesRenderContextTimeForRunningTools(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	rendered := defaultTranscriptRenderer.RenderCells(
		[]transcriptCell{
			toolTranscriptTestCellWithTiming(
				"call_1",
				"read_file",
				"read_file a.txt",
				startedAt,
				time.Time{},
				false,
			),
		},
		transcriptRenderContext{Width: 80, Now: startedAt.Add(3 * time.Second)},
	)

	require.Contains(t, stripANSI(rendered), "read_file a.txt (3s)")
}

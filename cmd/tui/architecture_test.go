package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
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

func TestTUIAction_ReplaceTranscriptCell(t *testing.T) {
	state := newTUIState(nil, false)
	appendTranscriptCellAction{Cell: userTranscriptCell{text: "before"}}.apply(&state)

	replaceTranscriptCellAction{
		Index: 0,
		Cell:  assistantTranscriptCell{text: "after"},
	}.apply(&state)

	require.Len(t, state.messages, 1)
	require.Equal(t, transcriptCellAssistant, state.messages[0].Kind())
	require.Equal(t, "Hand: after", state.messages[0].PlainText())
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
	require.Equal(t, "lo\nWo", document.PlainRange(3, 8))

	line, ok := document.Line(1)
	require.True(t, ok)
	require.Equal(t, "World", line.PlainText)

	_, ok = document.Line(2)
	require.False(t, ok)
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

func TestTranscriptCell_RenderDelegatesToRenderer(t *testing.T) {
	original := defaultTranscriptRenderer
	t.Cleanup(func() {
		defaultTranscriptRenderer = original
	})
	renderer := recordingTranscriptRenderer{output: "rendered"}
	defaultTranscriptRenderer = &renderer

	output := assistantTranscriptCell{text: "hello"}.Render(transcriptRenderContext{Width: 20})

	require.Equal(t, "rendered", output)
	require.Equal(t, transcriptCellAssistant, renderer.kind)
	require.Equal(t, 20, renderer.ctx.Width)
}

func TestTranscriptCellFactory_BuildsToolCellsSharedByLiveAndHydratedPaths(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(5 * time.Second)
	toolCall := handmsg.ToolCall{
		ID:    "call_1",
		Name:  "list_files",
		Input: `{"path":".","recursive":false,"include_hidden":false,"max_entries":50}`,
	}
	started, ok := toolInvocationStartedMsgFromMessageToolCall(toolCall, startedAt)
	require.True(t, ok)
	completed, ok := toolInvocationCompletedMsgFromMessage(handmsg.Message{
		Role:       handmsg.RoleTool,
		Name:       "list_files",
		ToolCallID: "call_1",
		CreatedAt:  completedAt,
	}, completedAt)
	require.True(t, ok)

	liveCells := []transcriptCell{
		defaultTranscriptCellFactory.FromTUIMessage(started),
		defaultTranscriptCellFactory.FromTUIMessage(completed),
	}
	hydratedCells := []transcriptCell{
		defaultTranscriptCellFactory.FromTimelineMessage(
			handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "list_files",
				ToolCallID: "call_1",
				Content:    `{"output":"done"}`,
				CreatedAt:  completedAt,
			},
			map[string]timelineToolCallDetail{
				"call_1": {
					detail:    started.Detail,
					startedAt: started.StartedAt,
				},
			},
		),
	}

	livePlain := stripANSI(renderTranscriptCells(liveCells))
	hydratedPlain := stripANSI(renderTranscriptCells(hydratedCells))
	require.Equal(t, hydratedPlain, livePlain)
	require.Contains(t, livePlain, "list_files(include_hidden=false max_entries=50 path=. recursive=false) (5s)")
}

func TestModel_HandleAppEventRoutesProductBehavior(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.handleAppEvent(viewportResizedEvent{Width: 120, Height: 40})
	require.Nil(t, cmd)
	require.Equal(t, 120, updated.width)
	require.Equal(t, 40, updated.height)

	updated, cmd = updated.handleAppEvent(applyTUIMessageEvent{
		Message: assistantResponseCompletedMsg{Text: "hello"},
	})
	require.Nil(t, cmd)
	require.Equal(t, []string{"Hand: hello"}, transcriptCellPlainTexts(updated.messages))
}

type recordingTranscriptRenderer struct {
	output string
	kind   transcriptCellKind
	ctx    transcriptRenderContext
}

func (r *recordingTranscriptRenderer) RenderCell(cell transcriptCell, ctx transcriptRenderContext) string {
	r.kind = cell.Kind()
	r.ctx = ctx

	return r.output
}

func (r *recordingTranscriptRenderer) RenderCells(cells []transcriptCell, ctx transcriptRenderContext) string {
	if len(cells) == 0 {
		return ""
	}

	return r.RenderCell(cells[0], ctx)
}

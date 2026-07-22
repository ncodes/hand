package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestModel_RenderTranscriptWindowMatchesFullRenderForSmallTranscript(t *testing.T) {
	runModel := newTranscriptWindowTestModel(20, 10)
	runModel.messages = transcriptWindowTestCells(4)
	expected := runModel.renderTranscriptContent()

	runModel.setTranscriptContent()

	require.Equal(t, expected, runModel.transcript.GetContent())
	if runModel.transcriptWindow.startBlock != 0 {
		t.Fatalf("start=%d offset=%d lines=%d end=%d total=%d", runModel.transcriptWindow.startBlock, runModel.transcript.YOffset(), runModel.transcript.TotalLineCount(), runModel.transcriptWindow.endBlock, runModel.transcriptWindow.totalBlocks)
	}
	require.Equal(t, runModel.transcriptWindow.totalBlocks, runModel.transcriptWindow.endBlock)
}

func TestModel_RenderTranscriptWindowMaterializesBoundedTail(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(2_000)
	full := runModel.renderTranscriptContent()
	runModel.transcriptCache.clear()

	runModel.setTranscriptContent()

	window := runModel.transcript.GetContent()
	require.Less(t, len(strings.Split(window, "\n")), 60)
	require.Less(t, runModel.transcriptCache.len(), 60)
	require.True(t, strings.HasSuffix(full, window))
	require.Greater(t, runModel.transcriptWindow.startBlock, 0)
	require.True(t, runModel.isTranscriptAtAbsoluteBottom())
}

func TestModel_RenderTranscriptWindowSlicesLargeCellAtLineBoundary(t *testing.T) {
	runModel := newTranscriptWindowTestModel(80, 8)
	runModel.messages = []transcriptCell{systemTranscriptCell{
		text: strings.Join(transcriptWindowTestLines(200), "\n"),
	}}
	full := runModel.renderTranscriptContent()

	runModel.setTranscriptContent()

	window := runModel.transcript.GetContent()
	require.Len(t, strings.Split(window, "\n"), 24)
	require.True(t, strings.HasSuffix(full, window))
	require.Greater(t, runModel.transcriptWindow.startLine, 0)
}

func TestModel_RenderTranscriptWindowMatchesFullRenderAtEveryWindowBoundary(t *testing.T) {
	t.Run("head", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(40, 8)
		runModel.messages = transcriptWindowTestCells(100)

		runModel.renderTranscriptWindowIntoViewport(transcriptWindowHead)

		assertTranscriptWindowMatchesFullRender(t, &runModel)
	})

	t.Run("middle", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(40, 8)
		runModel.messages = transcriptWindowTestCells(100)
		runModel.transcriptWindow.startBlock = 37

		runModel.renderTranscriptWindowIntoViewport(transcriptWindowCurrent)

		assertTranscriptWindowMatchesFullRender(t, &runModel)
		require.Greater(t, runModel.transcriptWindow.startBlock, 0)
		require.Less(t, runModel.transcriptWindow.endBlock, runModel.transcriptWindow.totalBlocks)
	})

	t.Run("partial last block", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(80, 8)
		runModel.messages = []transcriptCell{
			systemTranscriptCell{text: "first"},
			systemTranscriptCell{text: strings.Join(transcriptWindowTestLines(100), "\n")},
		}

		runModel.renderTranscriptWindowIntoViewport(transcriptWindowHead)

		assertTranscriptWindowMatchesFullRender(t, &runModel)
		require.Less(t, runModel.transcriptWindow.endBlock, runModel.transcriptWindow.totalBlocks)
	})
}

func TestModel_RenderTranscriptWindowBackfillsCurrentRangeFromTail(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(100)
	runModel.setTranscriptContent()
	oldStart := runModel.transcriptWindow.endBlock - 1
	runModel.transcriptWindow.startBlock = oldStart
	runModel.transcriptWindow.startLine = 0

	runModel.renderTranscriptWindowIntoViewport(transcriptWindowCurrent)

	require.Less(t, runModel.transcriptWindow.startBlock, oldStart)
	require.Len(t, strings.Split(runModel.transcript.GetContent(), "\n"), 24)
	require.True(t, runModel.isTranscriptAtAbsoluteBottom())
}

func TestModel_RenderTranscriptWindowHandlesSpecialContent(t *testing.T) {
	t.Run("name prompt", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(80, 8)
		runModel.namePromptEnabled = true
		runModel.userName = ""

		runModel.setTranscriptContent()

		require.Empty(t, runModel.transcript.GetContent())
	})

	t.Run("empty user prompt", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(80, 8)
		runModel.namePromptEnabled = false
		runModel.userName = "Nedy"

		runModel.setTranscriptContent()

		require.Contains(t, stripANSI(runModel.transcript.GetContent()), emptyUserPromptQuestion)
	})

	t.Run("intro", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(80, 8)
		runModel.namePromptEnabled = false
		runModel.userName = ""
		runModel.setupModelStep = "testing"
		runModel.showIntro = true

		runModel.setTranscriptContent()

		require.Contains(t, stripANSI(runModel.transcript.GetContent()), "Welcome to Morph TUI")
		require.False(t, runModel.showIntro)
	})

	t.Run("live tool", func(t *testing.T) {
		runModel := newTranscriptWindowTestModel(80, 8)
		runModel.messages = transcriptWindowTestCells(2)
		runModel.live = toolTranscriptTestCell("call_1", "browser", "snapshot:Page")

		runModel.setTranscriptContent()

		require.Contains(t, stripANSI(runModel.transcript.GetContent()), "Snapshot")
	})
}

func TestModel_TranscriptWindowUsesDefaultHeightBeforeLayout(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetHeight(0)
	runModel.transcript.SetWidth(0)
	runModel.width = 0

	require.Equal(t, defaultHeight*transcriptWindowHeightMultiplier, runModel.getTranscriptWindowLineTarget())
	require.NotEmpty(t, runModel.getTranscriptHeaderLines())
	require.Nil(t, splitTranscriptLines(" \n "))
}

func TestModel_MoveTranscriptWindowPositionForwardSkipsEmptyBlock(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	blocks := []transcriptWindowBlock{
		{},
		{staticLines: []string{"visible"}},
	}

	block, line, moved := runModel.moveTranscriptWindowPositionForward(blocks, 0, 0, 1)

	require.Equal(t, 2, block)
	require.Zero(t, line)
	require.Equal(t, 1, moved)
	require.Nil(t, runModel.renderTranscriptWindowBlock(transcriptWindowBlock{}))
}

func TestModel_MoveTranscriptWindowPositionBackwardClampsShrunkenBlock(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	blocks := []transcriptWindowBlock{{staticLines: []string{"first", "second"}}}

	block, line, moved := runModel.moveTranscriptWindowPositionBackward(blocks, 0, 100, 1)

	require.Zero(t, block)
	require.Equal(t, 1, line)
	require.Equal(t, 1, moved)
}

func TestModel_RenderTranscriptWindowStoresClampedStartLine(t *testing.T) {
	runModel := newTranscriptWindowTestModel(80, 8)
	runModel.messages = []transcriptCell{
		systemTranscriptCell{text: "short"},
		systemTranscriptCell{text: strings.Join(transcriptWindowTestLines(100), "\n")},
	}
	runModel.transcriptWindow.startLine = 100

	runModel.renderTranscriptWindowIntoViewport(transcriptWindowCurrent)

	blocks := runModel.getTranscriptWindowBlocks()
	require.Equal(t, len(runModel.renderTranscriptWindowBlock(blocks[0])), runModel.transcriptWindow.startLine)
}

func TestModel_RenderTranscriptWindowPreservesBlockBoundaryBlankLines(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	blocks := []transcriptWindowBlock{
		{staticLines: []string{"first", ""}},
		{staticLines: []string{"", "second"}, separator: true},
	}

	lines, _ := runModel.renderTranscriptWindowForward(blocks, 0, 0, 20)

	require.Equal(t, "first\n\n\n\nsecond", strings.Join(lines, "\n"))
}

func TestModel_ScrollTranscriptWindowReachesAbsoluteEdges(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()

	for attempts := 0; (runModel.transcriptWindow.startBlock > 0 || runModel.transcript.YOffset() > 0) && attempts < 1_000; attempts++ {
		runModel.scrollTranscriptWithKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	require.Zero(t, runModel.transcriptWindow.startBlock)
	require.Zero(t, runModel.transcript.YOffset())
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "message-0000")

	for attempts := 0; !runModel.isTranscriptAtAbsoluteBottom() && attempts < 1_000; attempts++ {
		runModel.scrollTranscriptWithKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	require.True(t, runModel.isTranscriptAtAbsoluteBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "message-0299")
}

func TestModel_MouseWheelShiftsTranscriptWindowInBothDirections(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()

	for attempts := 0; (runModel.transcriptWindow.startBlock > 0 || runModel.transcriptWindow.startLine > 0 ||
		runModel.transcript.YOffset() > 0) && attempts < 1_000; attempts++ {
		updated, _ := runModel.updateTranscriptWithScrollTracking(tea.MouseWheelMsg(tea.Mouse{
			Button: tea.MouseWheelUp,
		}))
		runModel = updated.(model)
	}
	require.Zero(t, runModel.transcriptWindow.startBlock)
	require.Zero(t, runModel.transcriptWindow.startLine)
	require.Zero(t, runModel.transcript.YOffset())

	for attempts := 0; !runModel.isTranscriptAtAbsoluteBottom() && attempts < 1_000; attempts++ {
		updated, _ := runModel.updateTranscriptWithScrollTracking(tea.MouseWheelMsg(tea.Mouse{
			Button: tea.MouseWheelDown,
		}))
		runModel = updated.(model)
	}
	require.True(t, runModel.isTranscriptAtAbsoluteBottom())
}

func TestModel_ShiftTranscriptWindowDownNeedsScrollableOffset(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.renderTranscriptWindowIntoViewport(transcriptWindowHead)
	runModel.transcript.GotoTop()

	require.False(t, runModel.shiftTranscriptWindowDown())
}

func TestModel_ActiveTurnFlushPreservesWindowAndOffset(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()
	for range 8 {
		runModel.scrollTranscriptWithKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	start := runModel.transcriptWindow.startBlock
	offset := runModel.transcript.YOffset()
	visible := stripANSI(runModel.transcript.View())
	runModel.messages = append(runModel.messages, assistantTranscriptCell{text: "new tail"})

	runModel.setTranscriptContentForActiveTurn()

	require.Equal(t, start, runModel.transcriptWindow.startBlock)
	require.Equal(t, offset, runModel.transcript.YOffset())
	require.Equal(t, visible, stripANSI(runModel.transcript.View()))
}

func TestModel_ViewportResizePreservesWindowPosition(t *testing.T) {
	runModel := newTranscriptWindowTestModel(80, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()
	for range 5 {
		runModel.scrollTranscriptWithKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	position := runModel.getTranscriptWindowPosition()
	visibleBefore := firstNonemptyTranscriptLine(runModel.transcript.View())

	updated, cmd := runModel.handleAppEvent(viewportResizedEvent{Width: 100, Height: 30})

	require.Nil(t, cmd)
	require.Contains(t, stripANSI(updated.transcript.View()), visibleBefore)
	require.GreaterOrEqual(t, updated.transcript.YOffset(), position.offset)
	require.False(t, updated.isTranscriptAtAbsoluteBottom())
}

func TestModel_ViewportResizeKeepsFollowAtAbsoluteTail(t *testing.T) {
	runModel := newTranscriptWindowTestModel(80, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()

	updated, cmd := runModel.handleAppEvent(viewportResizedEvent{Width: 100, Height: 30})

	require.Nil(t, cmd)
	require.True(t, updated.isTranscriptAtAbsoluteBottom())
	require.Contains(t, stripANSI(updated.transcript.View()), "message-0299")
}

func TestModel_TranscriptSelectionExpandsAcrossWindowEdges(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()
	document := newRenderedTranscriptDocument(runModel.transcript.GetContent())
	runModel.selection = transcriptSelection{
		active:   true,
		dragging: true,
		content:  runModel.transcript.GetContent(),
		start:    transcriptSelectionPoint{offset: len(document.PlainText) - 12},
		end:      transcriptSelectionPoint{offset: len(document.PlainText)},
	}
	selected := runModel.selectedTranscriptText()
	start := runModel.transcriptWindow.startBlock
	runModel.transcript.GotoTop()

	require.True(t, runModel.shiftTranscriptWindowUp())
	require.Less(t, runModel.transcriptWindow.startBlock, start)
	require.Equal(t, selected, runModel.selectedTranscriptText())
	require.True(t, runModel.selection.active)
	require.Contains(t, runModel.transcript.GetContent(), "\x1b[")

	runModel.scrollTranscriptWithKey(tea.KeyPressMsg{Code: tea.KeyHome})
	require.Zero(t, runModel.transcriptWindow.startBlock)
	require.Zero(t, runModel.transcriptWindow.startLine)
	require.Equal(t, selected, runModel.selectedTranscriptText())
	require.Equal(t, runModel.renderTranscriptContent(), runModel.selection.content)
}

func TestModel_TranscriptSelectionExpandsForwardToAbsoluteTail(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.renderTranscriptWindowIntoViewport(transcriptWindowHead)
	document := newRenderedTranscriptDocument(runModel.transcript.GetContent())
	runModel.selection = transcriptSelection{
		active:  true,
		content: runModel.transcript.GetContent(),
		start:   transcriptSelectionPoint{offset: 2},
		end:     transcriptSelectionPoint{offset: min(12, len(document.PlainText))},
	}
	selected := runModel.selectedTranscriptText()

	runModel.expandTranscriptSelectionToEdge(1)

	require.Equal(t, runModel.transcriptWindow.totalBlocks, runModel.transcriptWindow.endBlock)
	require.Equal(t, selected, runModel.selectedTranscriptText())
	require.Contains(t, stripANSI(runModel.selection.content), "message-0299")
	require.Equal(t, runModel.renderTranscriptContent(), runModel.selection.content)

	runModel.jumpTranscriptToBottom()
	require.True(t, runModel.selection.active)
	require.Equal(t, selected, runModel.selectedTranscriptText())
}

func TestModel_TranscriptSelectionAutoScrollRecognizesUnmaterializedContent(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.renderTranscriptWindowIntoViewport(transcriptWindowHead)
	runModel.transcript.GotoBottom()
	runModel.selection = transcriptSelection{active: true, dragging: true, content: runModel.transcript.GetContent()}
	mouse := tea.Mouse{Y: runModel.getTranscriptTop() + runModel.transcript.Height() - 1}

	require.Equal(t, 1, runModel.transcriptSelectionScrollDirection(mouse))
	end := runModel.transcriptWindow.endBlock
	runModel.scrollTranscriptSelection(1)
	require.Greater(t, runModel.transcriptWindow.endBlock, end)

	runModel = newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	runModel.selection = transcriptSelection{active: true, dragging: true, content: runModel.transcript.GetContent()}
	mouse = tea.Mouse{Y: runModel.getTranscriptTop()}

	require.Equal(t, -1, runModel.transcriptSelectionScrollDirection(mouse))
	start := runModel.transcriptWindow.startBlock
	runModel.scrollTranscriptSelection(-1)
	require.Less(t, runModel.transcriptWindow.startBlock, start)
}

func TestModel_TranscriptTextIncludesCellsOutsideWindow(t *testing.T) {
	runModel := newTranscriptWindowTestModel(30, 8)
	runModel.messages = transcriptWindowTestCells(300)
	runModel.setTranscriptContent()

	text := runModel.transcriptText()

	require.Contains(t, text, "message-0000")
	require.Contains(t, text, "message-0299")
	require.NotContains(t, stripANSI(runModel.transcript.GetContent()), "message-0000")
}

func newTranscriptWindowTestModel(width, height int) model {
	runModel := newModel()
	runModel.width = width
	runModel.height = height + 6
	runModel.transcript.SetWidth(width)
	runModel.transcript.SetHeight(height)
	runModel.showIntro = false

	return runModel
}

func transcriptWindowTestCells(count int) []transcriptCell {
	cells := make([]transcriptCell, count)
	for index := range cells {
		cells[index] = assistantTranscriptCell{text: fmt.Sprintf("message-%04d", index)}
	}

	return cells
}

func transcriptWindowTestLines(count int) []string {
	lines := make([]string, count)
	for index := range lines {
		lines[index] = fmt.Sprintf("line-%04d", index)
	}

	return lines
}

func firstNonemptyTranscriptLine(content string) string {
	for _, line := range strings.Split(stripANSI(content), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}

	return ""
}

func assertTranscriptWindowMatchesFullRender(t *testing.T, runModel *model) {
	t.Helper()

	fullLines := strings.Split(runModel.renderTranscriptContent(), "\n")
	blocks := runModel.getTranscriptWindowBlocks()
	start := 0
	for index := 0; index < runModel.transcriptWindow.startBlock; index++ {
		start += len(runModel.renderTranscriptWindowBlock(blocks[index]))
	}
	start += runModel.transcriptWindow.startLine
	windowLines := strings.Split(runModel.transcript.GetContent(), "\n")

	require.LessOrEqual(t, start+len(windowLines), len(fullLines))
	require.Equal(t, fullLines[start:start+len(windowLines)], windowLines)
}

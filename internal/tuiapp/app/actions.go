package tui

import (
	"strings"
	"time"

	tuistate "github.com/wandxy/hand/internal/tuiapp/state"
	tuitranscript "github.com/wandxy/hand/internal/tuiapp/transcript"
)

type tuiAction interface {
	apply(*tuiState)
}

type setViewportSizeAction struct {
	Width  int
	Height int
}

type appendTranscriptCellAction struct {
	Cell transcriptCell
}

type setTranscriptCellsAction struct {
	Cells []transcriptCell
}

type setLiveTranscriptCellAction struct {
	Cell transcriptCell
}

type clearTranscriptAction struct{}

type replaceTranscriptCellAction struct {
	Index int
	Cell  transcriptCell
}

type setSessionTitleAction struct {
	Title string
}

type setRespondingAction struct {
	Responding bool
	ResponseID int
}

func (action setViewportSizeAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	viewport := tuistate.NormalizeViewport(action.Width, action.Height)
	state.width = viewport.Width
	state.height = viewport.Height
}

func (action appendTranscriptCellAction) apply(state *tuiState) {
	if state == nil || action.Cell == nil || action.Cell.IsEmpty() {
		return
	}

	state.messages = append(state.messages, action.Cell)
	state.showIntro = false
}

func (action setTranscriptCellsAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.messages = cloneTranscriptCells(action.Cells)
	state.showIntro = len(state.messages) == 0
}

func (action setLiveTranscriptCellAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.live = action.Cell
}

func (clearTranscriptAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.messages = nil
	state.live = nil
	state.showIntro = false
	state.stream.Reset()
	state.reasoningStartedAt = time.Time{}
	state.reasoningMessageIndex = -1
}

func (action replaceTranscriptCellAction) apply(state *tuiState) {
	if state == nil || action.Index < 0 || action.Index >= len(state.messages) {
		return
	}
	if action.Cell == nil || action.Cell.IsEmpty() {
		return
	}

	state.messages[action.Index] = action.Cell
}

func (action setSessionTitleAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.sessionTitle = strings.TrimSpace(action.Title)
	if state.sessionTitle == "" {
		state.sessionTitle = defaultSessionTitle
	}
}

func (action setRespondingAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.responding = action.Responding
	state.responseID = action.ResponseID
}

func (m *model) applyAction(action tuiAction) {
	if action == nil {
		return
	}

	action.apply(&m.tuiState)
}

func cloneTranscriptCells(cells []transcriptCell) []transcriptCell {
	return tuitranscript.CloneCells(cells)
}

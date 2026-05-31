package tui

import (
	"strings"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	tuistate "github.com/wandxy/hand/internal/tui/state"
	tuitranscript "github.com/wandxy/hand/internal/tui/transcript"
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

type setSessionAction struct {
	ID    string
	Title string
}

type setSessionContextAction struct {
	Context string
}

type showCommandViewAction struct {
	TitleIcon       string
	TitleLeft       string
	TitleSubtext    string
	TitleRight      string
	AccentColor     string
	TitleRightColor string
	Content         string
	Height          int
	Kind            string
	Chats           []storage.Session
}

type hideCommandViewAction struct{}

type setRespondingAction struct {
	Responding bool
	ResponseID int
}

type resetResponseStateAction struct{}

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
	state.reasoningMessageIndices = nil
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

func (action setSessionAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.sessionID = strings.TrimSpace(action.ID)
	if state.sessionID == "" {
		state.sessionID = defaultSessionID
	}
	setSessionTitleAction{Title: action.Title}.apply(state)
}

func (action setSessionContextAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.context = strings.TrimSpace(action.Context)
}

func (action showCommandViewAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.commandView = commandViewState{
		Visible:         true,
		Kind:            strings.TrimSpace(action.Kind),
		TitleIcon:       strings.TrimSpace(action.TitleIcon),
		TitleLeft:       strings.TrimSpace(action.TitleLeft),
		TitleSubtext:    strings.TrimSpace(action.TitleSubtext),
		TitleRight:      strings.TrimSpace(action.TitleRight),
		AccentColor:     strings.TrimSpace(action.AccentColor),
		TitleRightColor: strings.TrimSpace(action.TitleRightColor),
		Content:         strings.TrimSpace(action.Content),
		Height:          max(action.Height, 0),
		Chats:           append([]storage.Session(nil), action.Chats...),
	}
	state.commandViewOffset = 0
	state.commandViewItemSelected = 0
	state.commandViewSelection = commandViewSelection{}
	state.chatsArchiveConfirm = false
	state.chatsRenaming = false
	state.chatsRenameSessionID = ""
}

func (hideCommandViewAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.commandView = commandViewState{}
	state.commandViewOffset = 0
	state.commandViewItemSelected = 0
	state.commandViewSelection = commandViewSelection{}
	state.chatsArchiveConfirm = false
	state.chatsRenaming = false
	state.chatsRenameSessionID = ""
}

func (action setRespondingAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.responding = action.Responding
	state.responseID = action.ResponseID
}

func (resetResponseStateAction) apply(state *tuiState) {
	if state == nil {
		return
	}

	state.responding = false
	state.responseTranscriptFollow = false
	state.responseTranscriptScrolled = false
	state.responseRunningToolCount = 0
	state.thinkingComposerActive = false
	state.toolAnimationActive = false
	state.responseCancel = nil
}

func (m *model) resetResponseState() {
	m.applyAction(resetResponseStateAction{})
	m.events = nil
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

package tui

import (
	"context"
	"time"

	"github.com/wandxy/hand/internal/constants"
)

type tuiState struct {
	width                      int
	height                     int
	status                     statusModel
	sessionID                  string
	sessionTitle               string
	modelName                  string
	runtimeInfo                runtimeInfo
	context                    string
	messages                   []transcriptCell
	live                       transcriptCell
	showIntro                  bool
	stream                     markdownStreamCollector
	reasoningStartedAt         time.Time
	reasoningMessageIndex      int
	reasoningMessageIndices    []int
	history                    []string
	historyAt                  int
	draft                      string
	responding                 bool
	responseID                 int
	responseCancel             context.CancelFunc
	responseTranscriptFollow   bool
	responseTranscriptScrolled bool
	responseStartMessageIndex  int
	responseRunningToolCount   int
	toolAnimationFrame         int
	toolAnimationActive        bool
	thinkingComposerFrame      int
	thinkingComposerActive     bool
	thinkingComposerEnabled    bool
	manualCompactionActive     bool
	manualCompactionIndex      int
	commandMenuOffset          int
	commandMenuSelected        int
	commandMenuPrefix          string
	exitAt                     time.Time
	allowShell                 bool
	selection                  transcriptSelection
}

func newTUIState(history []string, thinkingComposerEnabled bool) tuiState {
	return tuiState{
		width:                    defaultWidth,
		height:                   defaultHeight,
		status:                   newStatusModel(),
		sessionID:                defaultSessionID,
		sessionTitle:             defaultSessionTitle,
		modelName:                getModelDisplayName(constants.DefaultProfileModel),
		runtimeInfo:              defaultRuntimeInfo(),
		showIntro:                true,
		reasoningMessageIndex:    -1,
		manualCompactionIndex:    -1,
		history:                  history,
		historyAt:                len(history),
		thinkingComposerEnabled:  thinkingComposerEnabled,
		responseTranscriptFollow: false,
	}
}

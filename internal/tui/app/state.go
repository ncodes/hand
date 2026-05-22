package tui

import (
	"context"
	"time"
)

type tuiState struct {
	width                      int
	height                     int
	status                     statusModel
	sessionID                  string
	sessionTitle               string
	modelName                  string
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
		modelName:                "GPT 5.5",
		context:                  "60,000 used · 65%",
		showIntro:                true,
		reasoningMessageIndex:    -1,
		manualCompactionIndex:    -1,
		history:                  history,
		historyAt:                len(history),
		thinkingComposerEnabled:  thinkingComposerEnabled,
		responseTranscriptFollow: false,
	}
}

package tui

import (
	"time"
)

type tuiState struct {
	width                      int
	height                     int
	status                     statusModel
	sessionTitle               string
	modelName                  string
	context                    string
	messages                   []transcriptCell
	live                       transcriptCell
	showIntro                  bool
	stream                     markdownStreamCollector
	reasoningStartedAt         time.Time
	reasoningMessageIndex      int
	history                    []string
	historyAt                  int
	draft                      string
	responding                 bool
	responseID                 int
	responseTranscriptFollow   bool
	responseTranscriptScrolled bool
	toolAnimationFrame         int
	toolAnimationActive        bool
	thinkingComposerFrame      int
	thinkingComposerActive     bool
	thinkingComposerEnabled    bool
	exitAt                     time.Time
	allowShell                 bool
	selection                  transcriptSelection
}

func newTUIState(history []string, thinkingComposerEnabled bool) tuiState {
	return tuiState{
		width:                    defaultWidth,
		height:                   defaultHeight,
		status:                   newStatusModel(),
		sessionTitle:             defaultSessionTitle,
		modelName:                "GPT 5.5",
		context:                  "60,000 used · 65%",
		showIntro:                true,
		reasoningMessageIndex:    -1,
		history:                  history,
		historyAt:                len(history),
		thinkingComposerEnabled:  thinkingComposerEnabled,
		responseTranscriptFollow: false,
	}
}

package tui

import (
	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type tuiEvent interface {
	tuiEvent()
}

type viewportResizedEvent struct {
	Width  int
	Height int
}

type submitComposerEvent struct{}
type copyTranscriptEvent struct{}
type jumpTranscriptToBottomEvent struct{}
type showPreviousPromptEvent struct{}
type showNextPromptEvent struct{}
type insertInputNewlineEvent struct{}
type deleteInputLineEvent struct{}

type applyTUIMessageEvent struct {
	Message any
}

type hydrateTimelineEvent struct {
	Timeline rpcclient.SessionTimeline
}

func (viewportResizedEvent) tuiEvent()        {}
func (submitComposerEvent) tuiEvent()         {}
func (copyTranscriptEvent) tuiEvent()         {}
func (jumpTranscriptToBottomEvent) tuiEvent() {}
func (showPreviousPromptEvent) tuiEvent()     {}
func (showNextPromptEvent) tuiEvent()         {}
func (insertInputNewlineEvent) tuiEvent()     {}
func (deleteInputLineEvent) tuiEvent()        {}
func (applyTUIMessageEvent) tuiEvent()        {}
func (hydrateTimelineEvent) tuiEvent()        {}

func (m model) handleAppEvent(event tuiEvent) (model, tea.Cmd) {
	switch value := event.(type) {
	case viewportResizedEvent:
		m.applyAction(setViewportSizeAction{Width: value.Width, Height: value.Height})
		m.resize()
		return m, nil
	case submitComposerEvent:
		return m, m.submitPrompt()
	case copyTranscriptEvent:
		return m, m.copyTranscript()
	case jumpTranscriptToBottomEvent:
		m.jumpTranscriptToBottom()
		return m, nil
	case showPreviousPromptEvent:
		m.showPreviousPrompt()
		return m, nil
	case showNextPromptEvent:
		m.showNextPrompt()
		return m, nil
	case insertInputNewlineEvent:
		updated, cmd := m.insertInputNewline()
		next, _ := updated.(model)
		return next, cmd
	case deleteInputLineEvent:
		updated, cmd := m.deleteInputLine()
		next, _ := updated.(model)
		return next, cmd
	case applyTUIMessageEvent:
		return m, m.applyTUIMessage(value.Message)
	case hydrateTimelineEvent:
		return m, m.hydrateSessionTimeline(value.Timeline)
	default:
		return m, nil
	}
}

package tui

import (
	tea "charm.land/bubbletea/v2"

	tuievents "github.com/wandxy/hand/internal/tui/events"
)

type tuiEvent = tuievents.Event

type viewportResizedEvent = tuievents.ViewportResizedEvent

type submitComposerEvent = tuievents.SubmitComposerEvent
type copyTranscriptEvent = tuievents.CopyTranscriptEvent
type jumpTranscriptToBottomEvent = tuievents.JumpTranscriptToBottomEvent
type showPreviousPromptEvent = tuievents.ShowPreviousPromptEvent
type showNextPromptEvent = tuievents.ShowNextPromptEvent
type insertInputNewlineEvent = tuievents.InsertInputNewlineEvent
type deleteInputLineEvent = tuievents.DeleteInputLineEvent

type applyTUIMessageEvent = tuievents.ApplyTUIMessageEvent
type hydrateTimelineEvent = tuievents.HydrateTimelineEvent

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

package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	tuirpc "github.com/wandxy/hand/internal/tui/rpc"
)

type responseEventMsg = tuirpc.ResponseEvent
type responseEventsClosedMsg = tuirpc.ResponseEventsClosed
type responseCompletedMsg = tuirpc.ResponseCompleted

func respondToPromptCmd(
	client rpcclient.ChatAPI,
	responseID int,
	ctx context.Context,
	prompt string,
	events chan<- tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		defer close(events)

		if ctx == nil {
			ctx = context.Background()
		}

		reply, err := client.Respond(ctx, prompt, rpcclient.RespondOptions{
			OnEvent: func(event rpcclient.Event) {
				msg, ok := agentEventToTUIMessage(event)
				if !ok {
					return
				}
				if _, ok := msg.(assistantResponseCompletedMsg); ok {
					return
				}

				events <- msg
			},
		})

		return responseCompletedMsg{ResponseID: responseID, Text: reply, Err: err}
	}
}

func waitForResponseEvent(responseID int, events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return responseEventsClosedMsg{ResponseID: responseID}
		}

		return responseEventMsg{ResponseID: responseID, Message: msg}
	}
}

func (m *model) startResponse(prompt string) tea.Cmd {
	if m.chatClient == nil {
		return nil
	}

	if m.responseCancel != nil {
		m.responseCancel()
	}
	responseCtx := m.chatCtx
	if responseCtx == nil {
		responseCtx = context.Background()
	}
	responseCtx, cancel := context.WithCancel(responseCtx)

	events := make(chan tea.Msg, 32)
	m.responseID++
	m.events = events
	m.responseCancel = cancel
	m.applyAction(setRespondingAction{Responding: true, ResponseID: m.responseID})
	m.responseTranscriptFollow = m.transcript.AtBottom()
	m.responseTranscriptScrolled = false
	m.responseRunningToolCount = 0
	m.toolAnimationActive = false
	m.stream.Reset()
	m.applyAction(setLiveTranscriptCellAction{})
	m.clearReasoningTranscriptState()

	return tea.Batch(
		m.startThinkingComposer(),
		respondToPromptCmd(m.chatClient, m.responseID, responseCtx, prompt, events),
		waitForResponseEvent(m.responseID, events),
	)
}

func (m *model) completeResponse(msg responseCompletedMsg) tea.Cmd {
	if !m.isActiveResponse(msg.ResponseID) {
		return nil
	}

	shouldFollowTranscript := m.responseTranscriptFollow && !m.responseTranscriptScrolled
	if msg.Err != nil {
		errorMsg := sessionErrorMsg{Message: msg.Err.Error()}
		m.addTranscriptMessage(errorMsg)
		m.applyAction(setRespondingAction{Responding: false, ResponseID: m.responseID})
		m.responseTranscriptFollow = false
		m.responseTranscriptScrolled = false
		m.responseRunningToolCount = 0
		m.thinkingComposerActive = false
		m.events = nil
		m.responseCancel = nil
		return m.setStatus("response failed")
	}

	m.completeAssistantResponse(msg.Text)
	m.applyAction(setRespondingAction{Responding: false, ResponseID: m.responseID})
	m.responseTranscriptFollow = false
	m.responseTranscriptScrolled = false
	m.responseRunningToolCount = 0
	m.thinkingComposerActive = false
	m.events = nil
	m.responseCancel = nil
	if shouldFollowTranscript {
		m.resize()
		m.transcript.GotoBottom()
	}
	return loadSessionTitleCmd(m.chatCtx, m.title)
}

func (m *model) cancelActiveResponse() tea.Cmd {
	if !m.responding {
		return nil
	}

	if m.responseCancel != nil {
		m.responseCancel()
	}
	m.applyAction(setRespondingAction{Responding: false, ResponseID: m.responseID})
	m.responseTranscriptFollow = false
	m.responseTranscriptScrolled = false
	m.responseRunningToolCount = 0
	m.thinkingComposerActive = false
	m.toolAnimationActive = false
	m.responseCancel = nil
	m.events = nil

	return m.setStatus("response cancelled")
}

func (m model) isActiveResponse(responseID int) bool {
	return m.responding && responseID == m.responseID
}

package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type responseEventMsg struct {
	ResponseID int
	Message    tea.Msg
}

type responseEventsClosedMsg struct {
	ResponseID int
}

type responseCompletedMsg struct {
	ResponseID int
	Text       string
	Err        error
}

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
			Stream: new(true),
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

	events := make(chan tea.Msg, 32)
	m.responseID++
	m.events = events
	m.responding = true
	m.responseTranscriptFollow = m.transcript.AtBottom()
	m.responseTranscriptScrolled = false

	return tea.Batch(
		respondToPromptCmd(m.chatClient, m.responseID, m.chatCtx, prompt, events),
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
		m.responding = false
		m.responseTranscriptFollow = false
		m.responseTranscriptScrolled = false
		m.events = nil
		return m.setStatus("response failed")
	}

	m.completeAssistantResponse(msg.Text)
	m.responding = false
	m.responseTranscriptFollow = false
	m.responseTranscriptScrolled = false
	m.events = nil
	if shouldFollowTranscript {
		m.resize()
		m.transcript.GotoBottom()
	}
	return nil
}

func (m model) isActiveResponse(responseID int) bool {
	return m.responding && responseID == m.responseID
}

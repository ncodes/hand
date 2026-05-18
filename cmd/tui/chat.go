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

package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/morph/internal/permissions"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/rpc/rpcmeta"
	tuirpc "github.com/wandxy/morph/internal/tui/rpc"
)

type responseEventMsg = tuirpc.ResponseEvent
type responseEventsClosedMsg = tuirpc.ResponseEventsClosed
type responseCompletedMsg = tuirpc.ResponseCompleted

const responseEventBatchLimit = 64

var streamingTranscriptRenderInterval = 33 * time.Millisecond

type responseEventBatchMsg struct {
	ResponseID int
	Messages   []tea.Msg
	Closed     bool
}

type streamingTranscriptFlushMsg struct {
	ResponseID int
}

func respondToPromptCmd(
	client rpcclient.ChatAPI,
	responseID int,
	ctx context.Context,
	sessionID string,
	prompt string,
	preset permissions.Preset,
	events chan<- tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		defer close(events)

		if ctx == nil {
			ctx = context.Background()
		}
		ctx = rpcmeta.WithOutgoingPermissionSurface(ctx, permissions.SurfaceTUI)
		ctx = rpcmeta.WithOutgoingPermissionPreset(ctx, preset)

		reply, err := client.Respond(ctx, prompt, rpcclient.RespondOptions{
			SessionID: sessionID,
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

		batch := responseEventBatchMsg{
			ResponseID: responseID,
			Messages:   []tea.Msg{msg},
		}
		for len(batch.Messages) < responseEventBatchLimit {
			select {
			case next, open := <-events:
				if !open {
					batch.Closed = true
					return batch
				}
				batch.Messages = append(batch.Messages, next)
			default:
				return batch
			}
		}

		return batch
	}
}

func (m *model) startResponse(prompt string, followTranscript bool) tea.Cmd {
	if m.chatClient == nil {
		return nil
	}

	m.cancelResponseAndDrainEvents()
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
	m.responseStartMessageIndex = len(m.messages)
	m.responseStartedAt = currentTime()
	m.responseTranscriptFollow = followTranscript
	m.responseTranscriptScrolled = false
	m.responseRunningToolCount = 0
	m.responseEventStreamActive = true
	m.pendingResponseCompletion = nil
	m.toolAnimationActive = false
	m.stream.Reset()
	m.applyAction(setLiveTranscriptCellAction{})
	m.clearReasoningTranscriptState()

	return tea.Batch(
		m.startThinkingComposer(),
		respondToPromptCmd(
			m.chatClient,
			m.responseID,
			responseCtx,
			m.getCurrentSessionID(),
			prompt,
			m.permissionPreset,
			events,
		),
		waitForResponseEvent(m.responseID, events),
	)
}

func (m *model) handleResponseCompleted(msg responseCompletedMsg) tea.Cmd {
	if !m.isActiveResponse(msg.ResponseID) {
		return nil
	}
	if m.responseEventStreamActive {
		m.pendingResponseCompletion = &msg
		return nil
	}

	return m.completeResponse(msg)
}

func (m *model) handleResponseEventsClosed(msg responseEventsClosedMsg) tea.Cmd {
	if !m.isActiveResponse(msg.ResponseID) {
		return nil
	}

	m.responseEventStreamActive = false
	m.events = nil
	if m.pendingResponseCompletion == nil {
		return nil
	}

	completion := *m.pendingResponseCompletion
	m.pendingResponseCompletion = nil
	return m.completeResponse(completion)
}

func (m *model) completeResponse(msg responseCompletedMsg) tea.Cmd {
	if !m.isActiveResponse(msg.ResponseID) {
		return nil
	}

	shouldFollowTranscript := m.responseTranscriptFollow && !m.responseTranscriptScrolled
	if msg.Err != nil {
		failure := formatToolFailureDisplayMessage(getUserFacingErrorMessage(msg.Err.Error()), false)
		m.failRunningToolTranscriptCells(currentTime(), failure)
		errorMsg := sessionErrorMsg{Message: msg.Err.Error()}
		m.addTranscriptMessage(errorMsg)
		m.resetResponseState()
		return m.setStatus("response failed")
	}

	m.interruptRunningToolTranscriptCells(currentTime())
	m.completeAssistantResponse(msg.Text, m.getCompletedResponseDuration())
	m.resetResponseState()
	if shouldFollowTranscript {
		m.transcript.GotoBottom()
	}
	return tea.Batch(
		loadSessionTitleCmd(m.chatCtx, m.title),
		loadSessionContextCmd(m.chatCtx, m.contextLoader, m.getCurrentSessionID()),
	)
}

func (m *model) cancelActiveResponse() tea.Cmd {
	if !m.responding {
		return nil
	}

	m.cancelResponseAndDrainEvents()
	m.interruptRunningToolTranscriptCells(currentTime())
	m.resetResponseState()

	return m.setStatus("response cancelled")
}

func (m *model) cancelResponseAndDrainEvents() {
	if m.responseCancel != nil {
		m.responseCancel()
	}
	if m.events == nil {
		return
	}

	events := m.events
	m.events = nil
	go func() {
		for range events {
		}
	}()
}

func (m model) getCompletedResponseDuration() time.Duration {
	if m.responseStartedAt.IsZero() {
		return 0
	}

	return currentTime().Sub(m.responseStartedAt).Round(time.Second)
}

func (m model) isActiveResponse(responseID int) bool {
	return m.responding && responseID == m.responseID
}

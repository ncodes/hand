package tui

import (
	"context"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	transcriptComposerGapHeight = 1
	inputChromeHeight           = inputFrameChromeHeight + bottomStatusPanelHeight + transcriptComposerGapHeight
)

// model is the root Bubble Tea application state for the interactive shell.
type model struct {
	transcript viewport.Model
	input      textarea.Model
	tuiState
	chatClient rpcclient.ChatAPI
	timeline   sessionTimelineLoader
	chatCtx    context.Context
	events     <-chan tea.Msg
}

// newModel builds the initial TUI state and sizes child components.
func newModel() model {
	return newModelWithClientContext(context.Background(), nil)
}

func newModelWithClient(client rpcclient.ChatAPI) model {
	return newModelWithClientContext(context.Background(), client)
}

func newModelWithClientContext(ctx context.Context, client rpcclient.ChatAPI) model {
	return newModelWithClientContextAndConfig(ctx, client, nil)
}

func newModelWithClientContextAndConfig(ctx context.Context, client rpcclient.ChatAPI, cfg *config.Config) model {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		cfg = config.NewDefaultConfig()
	}

	history, err := loadPromptHistory()
	appModel := model{
		transcript: newTranscript(),
		input:      newInputComposer(),
		tuiState:   newTUIState(history, cfg.TUIThinkingComposerEnabled()),
		chatClient: client,
		chatCtx:    ctx,
	}
	if timeline, ok := client.(sessionTimelineLoader); ok {
		appModel.timeline = timeline
	}
	if err != nil {
		appModel.status.setTransient("prompt history unavailable")
	}
	appModel.resize()
	appModel.setTranscriptContent()

	return appModel
}

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), m.statusExpireCmd(), m.runEffect(loadSessionTimelineEffect{}))
}

// Update handles terminal events and delegates ordinary input to child models.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleAppEvent(viewportResizedEvent{Width: msg.Width, Height: msg.Height})
	case exitConfirmationExpiredMsg:
		return m.expireExitConfirmation(msg), nil
	case statusExpiredMsg:
		m.status.expire(msg)
		return m, nil
	case toolAnimationTickMsg:
		return m.updateToolAnimation()
	case thinkingComposerTickMsg:
		return m.updateThinkingComposer()
	case assistantTextDeltaMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case assistantResponseCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case responseEventMsg:
		if !m.isActiveResponse(msg.ResponseID) {
			return m, nil
		}
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg.Message})
		m = next
		if m.events != nil {
			cmd = tea.Batch(cmd, waitForResponseEvent(msg.ResponseID, m.events))
		}
		return m, cmd
	case responseEventsClosedMsg:
		if !m.isActiveResponse(msg.ResponseID) {
			return m, nil
		}
		return m, nil
	case responseCompletedMsg:
		return m, m.completeResponse(msg)
	case sessionTimelineLoadedMsg:
		return m.handleAppEvent(hydrateTimelineEvent{Timeline: msg.Timeline})
	case sessionTimelineLoadFailedMsg:
		return m, m.setStatus("session timeline unavailable")
	case sessionErrorMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case toolInvocationStartedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case toolInvocationCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case safetyEventMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case tea.PasteMsg:
		msg.Content = normalizeComposerPaste(msg.Content)
		m.resizeInputForValue(m.input.Value() + msg.Content)
		m.input, _ = m.input.Update(msg)
		m.resize()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c":
			return m.confirmExit()
		case "ctrl+y":
			return m.handleAppEvent(copyTranscriptEvent{})
		case "esc":
			return m, nil
		case "ctrl+p":
			return m.handleAppEvent(showPreviousPromptEvent{})
		case "ctrl+n":
			return m.handleAppEvent(showNextPromptEvent{})
		case "shift+enter":
			return m.handleAppEvent(insertInputNewlineEvent{})
		case "ctrl+end":
			return m.handleAppEvent(jumpTranscriptToBottomEvent{})
		case "enter":
			return m.handleAppEvent(submitComposerEvent{})
		}
		if isInputLineDeleteKey(msg) {
			return m.handleAppEvent(deleteInputLineEvent{})
		}
		if m.shouldUseHistoryKey(msg) {
			switch msg.Key().Code {
			case tea.KeyUp:
				return m.handleAppEvent(showPreviousPromptEvent{})
			case tea.KeyDown:
				return m.handleAppEvent(showNextPromptEvent{})
			}
		}
		if m.scrollTranscriptWithKey(msg) {
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.resize()
		return m, cmd
	case tea.MouseWheelMsg:
		return m.updateTranscriptWithScrollTracking(msg)
	case tea.MouseClickMsg:
		if m.clicksJumpToBottomIndicator(msg) {
			return m.handleAppEvent(jumpTranscriptToBottomEvent{})
		}
		if m.startTranscriptSelection(msg) {
			return m, nil
		}
	case tea.MouseMotionMsg:
		if m.updateTranscriptSelection(msg) {
			return m, nil
		}
	case tea.MouseReleaseMsg:
		if cmd := m.finishTranscriptSelection(msg); cmd != nil {
			return m, cmd
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)
	m.resize()

	return m, tea.Batch(cmds...)
}

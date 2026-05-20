package tui

import (
	"context"
	"time"

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
	transcript                 viewport.Model
	input                      textarea.Model
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
	chatClient                 rpcclient.ChatAPI
	timeline                   sessionTimelineLoader
	chatCtx                    context.Context
	responding                 bool
	responseID                 int
	responseTranscriptFollow   bool
	responseTranscriptScrolled bool
	events                     <-chan tea.Msg
	toolAnimationFrame         int
	toolAnimationActive        bool
	thinkingComposerFrame      int
	thinkingComposerActive     bool
	thinkingComposerEnabled    bool
	exitAt                     time.Time
	allowShell                 bool
	selection                  transcriptSelection
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
		transcript:              newTranscript(),
		input:                   newInputComposer(),
		width:                   defaultWidth,
		height:                  defaultHeight,
		status:                  newStatusModel(),
		sessionTitle:            defaultSessionTitle,
		modelName:               "GPT 5.5",
		context:                 "60,000 used · 65%",
		showIntro:               true,
		reasoningMessageIndex:   -1,
		history:                 history,
		chatClient:              client,
		chatCtx:                 ctx,
		thinkingComposerEnabled: cfg.TUIThinkingComposerEnabled(),
	}
	if timeline, ok := client.(sessionTimelineLoader); ok {
		appModel.timeline = timeline
	}
	appModel.historyAt = len(appModel.history)
	if err != nil {
		appModel.status.setTransient("prompt history unavailable")
	}
	appModel.resize()
	appModel.setTranscriptContent()

	return appModel
}

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), m.statusExpireCmd(), loadSessionTimelineCmd(m.chatCtx, m.timeline))
}

// Update handles terminal events and delegates ordinary input to child models.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
		m.height = max(msg.Height, 1)
		m.resize()
		return m, nil
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
		return m, m.applyTUIMessage(msg)
	case assistantResponseCompletedMsg:
		return m, m.applyTUIMessage(msg)
	case responseEventMsg:
		if !m.isActiveResponse(msg.ResponseID) {
			return m, nil
		}
		cmd := m.applyTUIMessage(msg.Message)
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
		return m, m.hydrateSessionTimeline(msg.Timeline)
	case sessionTimelineLoadFailedMsg:
		return m, m.setStatus("session timeline unavailable")
	case sessionErrorMsg:
		return m, m.applyTUIMessage(msg)
	case toolInvocationStartedMsg:
		return m, m.applyTUIMessage(msg)
	case toolInvocationCompletedMsg:
		return m, m.applyTUIMessage(msg)
	case safetyEventMsg:
		return m, m.applyTUIMessage(msg)
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
			return m, m.copyTranscript()
		case "esc":
			return m, nil
		case "ctrl+p":
			m.showPreviousPrompt()
			return m, nil
		case "ctrl+n":
			m.showNextPrompt()
			return m, nil
		case "shift+enter":
			return m.insertInputNewline()
		case "ctrl+end":
			m.jumpTranscriptToBottom()
			return m, nil
		case "enter":
			cmd := m.submitPrompt()
			return m, cmd
		}
		if isInputLineDeleteKey(msg) {
			return m.deleteInputLine()
		}
		if m.shouldUseHistoryKey(msg) {
			switch msg.Key().Code {
			case tea.KeyUp:
				m.showPreviousPrompt()
			case tea.KeyDown:
				m.showNextPrompt()
			}
			return m, nil
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
			m.jumpTranscriptToBottom()
			return m, nil
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

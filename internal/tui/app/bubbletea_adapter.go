package tui

import tea "charm.land/bubbletea/v2"

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.input.Focus(),
		m.statusExpireCmd(),
		m.loadStartupSessionTimeline(),
	)
}

// Update adapts Bubble Tea terminal messages into app-level TUI events.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleLifecycleMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleAsyncMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleTerminalMsg(msg); handled {
		return next, cmd
	}

	return m.updateBubbleTeaChildren(msg)
}

func (m model) handleLifecycleMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		next, cmd := m.handleAppEvent(viewportResizedEvent{Width: msg.Width, Height: msg.Height})
		return next, cmd, true
	case exitConfirmationExpiredMsg:
		return m.expireExitConfirmation(msg), nil, true
	case statusExpiredMsg:
		expireStatus(&m.status, msg)
		return m, nil, true
	case namePromptErrorExpiredMsg:
		return m.expireNamePromptError(msg), nil, true
	case toolAnimationTickMsg:
		next, cmd := m.updateToolAnimation()
		return next, cmd, true
	case thinkingComposerTickMsg:
		next, cmd := m.updateThinkingComposer()
		return next, cmd, true
	case transcriptSelectionAutoScrollTickMsg:
		next, cmd := m.updateTranscriptSelectionAutoScroll()
		return next, cmd, true
	case commandViewSelectionAutoScrollTickMsg:
		next, cmd := m.updateCommandViewSelectionAutoScroll()
		return next, cmd, true
	default:
		return m, nil, false
	}
}

func (m model) handleAsyncMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case assistantTextDeltaMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case assistantResponseCompletedMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case reasoningCompletedMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case responseEventMsg:
		next, cmd := m.handleResponseEvent(msg)
		return next, cmd, true
	case responseEventsClosedMsg:
		if !m.isActiveResponse(msg.ResponseID) {
			return m, nil, true
		}
		return m, nil, true
	case responseCompletedMsg:
		cmd := m.completeResponse(msg)
		return m, cmd, true
	case sessionTimelineLoadedMsg:
		m.chatSwitching = false
		next, cmd := m.handleAppEvent(hydrateTimelineEvent{Timeline: msg.Timeline})
		return next, cmd, true
	case sessionTimelineLoadFailedMsg:
		m.chatSwitching = false
		cmd := m.setStatus("session timeline unavailable")
		return m, cmd, true
	case sessionTitleLoadedMsg:
		m.refreshSessionTitleFromSession(msg.Session)
		return m, nil, true
	case sessionTitleLoadFailedMsg:
		return m, nil, true
	case sessionContextLoadedMsg:
		m.refreshSessionContext(msg.Status)
		return m, nil, true
	case sessionContextLoadFailedMsg:
		return m, nil, true
	case sessionErrorMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case compactSessionCompletedMsg:
		cmd := m.completeCompactSession(msg)
		return m, cmd, true
	case newChatCompletedMsg:
		cmd := m.completeNewChat(msg)
		return m, cmd, true
	case chatsLoadedMsg:
		cmd := m.completeChatsCommand(msg)
		return m, cmd, true
	case toolInvocationStartedMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case toolInvocationCompletedMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	case safetyEventMsg:
		next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg})
		return next, cmd, true
	default:
		return m, nil, false
	}
}

func (m model) handleTerminalMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.shouldShowNamePrompt() {
			next, cmd := m.handleNamePromptPaste(msg)
			return next, cmd, true
		}
		next, cmd := m.handlePasteMsg(msg)
		return next, cmd, true
	case tea.KeyPressMsg:
		if msg.Keystroke() == "ctrl+c" {
			next, cmd := m.confirmExit()
			return next, cmd, true
		}
		if m.shouldShowNamePrompt() {
			next, cmd := m.handleNamePromptKey(msg)
			return next, cmd, true
		}
		if next, cmd, ok := m.handleKeyPressMsg(msg); ok {
			return next, cmd, true
		}
		next, cmd := m.updateInputComposer(msg)
		return next, cmd, true
	case tea.MouseWheelMsg:
		if m.isCommandViewVisible() {
			next, cmd := m.updateCommandView(msg)
			return next, cmd, true
		}
		if m.scrollCommandMenuWithMouse(msg) {
			return m, nil, true
		}
		next, cmd := m.updateTranscriptWithScrollTracking(msg)
		return next, cmd, true
	case tea.MouseClickMsg:
		if m.isCommandViewVisible() {
			if m.startCommandViewSelection(msg) {
				return m, nil, true
			}

			return m, nil, true
		}
		if cmd, ok := m.openTranscriptLinkAtMouse(msg); ok {
			return m, cmd, true
		}
		if m.clicksJumpToBottomIndicator(msg) {
			next, cmd := m.handleAppEvent(jumpTranscriptToBottomEvent{})
			return next, cmd, true
		}
		if m.startTranscriptSelection(msg) {
			return m, nil, true
		}
	case tea.MouseMotionMsg:
		if handled, cmd := m.updateCommandViewSelection(msg); handled {
			return m, cmd, true
		}
		if m.updateCommandMenuHover(msg) {
			return m, nil, true
		}
		if handled, cmd := m.updateTranscriptSelection(msg); handled {
			return m, cmd, true
		}
	case tea.MouseReleaseMsg:
		if handled, cmd := m.finishCommandViewSelection(msg); handled {
			return m, cmd, true
		}
		if handled, cmd := m.finishTranscriptSelection(msg); handled {
			return m, cmd, true
		}
	}

	return m, nil, false
}

func (m model) handleResponseEvent(msg responseEventMsg) (tea.Model, tea.Cmd) {
	if !m.isActiveResponse(msg.ResponseID) {
		return m, nil
	}
	if _, ok := msg.Message.(sessionErrorMsg); ok {
		if m.events != nil {
			return m, waitForResponseEvent(msg.ResponseID, m.events)
		}
		return m, nil
	}

	next, cmd := m.handleAppEvent(applyTUIMessageEvent{Message: msg.Message})
	m = next
	if m.events != nil {
		cmd = tea.Batch(cmd, waitForResponseEvent(msg.ResponseID, m.events))
	}

	return m, cmd
}

func (m model) handlePasteMsg(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if m.manualCompactionActive || m.chatSwitching {
		return m, nil
	}

	msg.Content = normalizeComposerPaste(msg.Content)
	m.resizeInputForValue(m.input.Value() + msg.Content)
	m.input, _ = m.input.Update(msg)
	m.updateCommandMenuForInput(m.input.Value())
	m.resize()

	return m, nil
}

func (m model) handleKeyPressMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.Keystroke() {
	case "ctrl+c":
		next, cmd := m.confirmExit()
		return next, cmd, true
	}
	if m.isCommandViewVisible() {
		if msg.Keystroke() == "esc" {
			return m.hideCommandView(), nil, true
		}
		if msg.Keystroke() == "ctrl+y" {
			return m, m.copyCommandView(), true
		}
		next, cmd := m.updateCommandView(msg)
		return next, cmd, true
	}
	if m.manualCompactionActive {
		return m, nil, true
	}
	if m.chatSwitching {
		return m, nil, true
	}

	switch msg.Keystroke() {
	case "ctrl+y":
		next, cmd := m.handleAppEvent(copyTranscriptEvent{})
		return next, cmd, true
	case "esc":
		cmd := m.cancelActiveResponse()
		return m, cmd, true
	case "ctrl+p":
		next, cmd := m.handleAppEvent(showPreviousPromptEvent{})
		return next, cmd, true
	case "ctrl+n":
		next, cmd := m.handleAppEvent(showNextPromptEvent{})
		return next, cmd, true
	case "shift+enter", "alt+enter", "ctrl+j":
		next, cmd := m.handleAppEvent(insertInputNewlineEvent{})
		return next, cmd, true
	case "ctrl+end":
		next, cmd := m.handleAppEvent(jumpTranscriptToBottomEvent{})
		return next, cmd, true
	case "enter":
		next, cmd := m.handleAppEvent(submitComposerEvent{})
		return next, cmd, true
	}
	if isInputLineDeleteKey(msg) {
		next, cmd := m.handleAppEvent(deleteInputLineEvent{})
		return next, cmd, true
	}
	if m.shouldUseHistoryKey(msg) {
		switch msg.Key().Code {
		case tea.KeyUp:
			if m.scrollCommandMenu(-1) {
				return m, nil, true
			}
			next, cmd := m.handleAppEvent(showPreviousPromptEvent{})
			return next, cmd, true
		case tea.KeyDown:
			if m.scrollCommandMenu(1) {
				return m, nil, true
			}
			next, cmd := m.handleAppEvent(showNextPromptEvent{})
			return next, cmd, true
		}
	}
	if m.scrollTranscriptWithKey(msg) {
		return m, nil, true
	}

	return m, nil, false
}

func (m model) updateBubbleTeaChildren(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.chatSwitching {
		return m, nil
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateCommandMenuForInput(m.input.Value())
	cmds = append(cmds, cmd)
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)
	m.resize()

	return m, tea.Batch(cmds...)
}

func (m model) updateInputComposer(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.chatSwitching {
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateCommandMenuForInput(m.input.Value())
	m.resize()

	return m, cmd
}

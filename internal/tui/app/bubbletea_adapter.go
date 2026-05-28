package tui

import tea "charm.land/bubbletea/v2"

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.input.Focus(),
		m.statusExpireCmd(),
		m.runEffect(loadSessionTimelineEffect{}),
		loadSessionContextCmd(m.chatCtx, m.contextLoader, m.getCurrentSessionID()),
	)
}

// Update adapts Bubble Tea terminal messages into app-level TUI events.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleAppEvent(viewportResizedEvent{Width: msg.Width, Height: msg.Height})
	case exitConfirmationExpiredMsg:
		return m.expireExitConfirmation(msg), nil
	case statusExpiredMsg:
		expireStatus(&m.status, msg)
		return m, nil
	case namePromptErrorExpiredMsg:
		return m.expireNamePromptError(msg), nil
	case toolAnimationTickMsg:
		return m.updateToolAnimation()
	case thinkingComposerTickMsg:
		return m.updateThinkingComposer()
	case transcriptSelectionAutoScrollTickMsg:
		return m.updateTranscriptSelectionAutoScroll()
	case commandViewSelectionAutoScrollTickMsg:
		return m.updateCommandViewSelectionAutoScroll()
	case assistantTextDeltaMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case assistantResponseCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case reasoningCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case responseEventMsg:
		return m.handleResponseEvent(msg)
	case responseEventsClosedMsg:
		if !m.isActiveResponse(msg.ResponseID) {
			return m, nil
		}
		return m, nil
	case responseCompletedMsg:
		cmd := m.completeResponse(msg)
		return m, cmd
	case sessionTimelineLoadedMsg:
		return m.handleAppEvent(hydrateTimelineEvent{Timeline: msg.Timeline})
	case sessionTimelineLoadFailedMsg:
		cmd := m.setStatus("session timeline unavailable")
		return m, cmd
	case sessionTitleLoadedMsg:
		m.refreshSessionTitleFromSession(msg.Session)
		return m, nil
	case sessionTitleLoadFailedMsg:
		return m, nil
	case sessionContextLoadedMsg:
		m.refreshSessionContext(msg.Status)
		return m, nil
	case sessionContextLoadFailedMsg:
		return m, nil
	case sessionErrorMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case compactSessionCompletedMsg:
		cmd := m.completeCompactSession(msg)
		return m, cmd
	case toolInvocationStartedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case toolInvocationCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case safetyEventMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case tea.PasteMsg:
		if m.shouldShowNamePrompt() {
			return m.handleNamePromptPaste(msg)
		}
		return m.handlePasteMsg(msg)
	case tea.KeyPressMsg:
		if msg.Keystroke() == "ctrl+c" {
			return m.confirmExit()
		}
		if m.shouldShowNamePrompt() {
			return m.handleNamePromptKey(msg)
		}
		if next, cmd, ok := m.handleKeyPressMsg(msg); ok {
			return next, cmd
		}
		return m.updateInputComposer(msg)
	case tea.MouseWheelMsg:
		if m.isCommandViewVisible() {
			return m.updateCommandView(msg)
		}
		if m.scrollCommandMenuWithMouse(msg) {
			return m, nil
		}
		return m.updateTranscriptWithScrollTracking(msg)
	case tea.MouseClickMsg:
		if m.isCommandViewVisible() {
			if m.startCommandViewSelection(msg) {
				return m, nil
			}

			return m, nil
		}
		if cmd, ok := m.openTranscriptLinkAtMouse(msg); ok {
			return m, cmd
		}
		if m.clicksJumpToBottomIndicator(msg) {
			return m.handleAppEvent(jumpTranscriptToBottomEvent{})
		}
		if m.startTranscriptSelection(msg) {
			return m, nil
		}
	case tea.MouseMotionMsg:
		if handled, cmd := m.updateCommandViewSelection(msg); handled {
			return m, cmd
		}
		if m.updateCommandMenuHover(msg) {
			return m, nil
		}
		if handled, cmd := m.updateTranscriptSelection(msg); handled {
			return m, cmd
		}
	case tea.MouseReleaseMsg:
		if handled, cmd := m.finishCommandViewSelection(msg); handled {
			return m, cmd
		}
		if cmd := m.finishTranscriptSelection(msg); cmd != nil {
			return m, cmd
		}
	}

	return m.updateBubbleTeaChildren(msg)
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
	if m.manualCompactionActive {
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
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateCommandMenuForInput(m.input.Value())
	m.resize()

	return m, cmd
}

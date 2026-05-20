package tui

import tea "charm.land/bubbletea/v2"

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), m.statusExpireCmd(), m.runEffect(loadSessionTimelineEffect{}))
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
	case toolAnimationTickMsg:
		return m.updateToolAnimation()
	case thinkingComposerTickMsg:
		return m.updateThinkingComposer()
	case assistantTextDeltaMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case assistantResponseCompletedMsg:
		return m.handleAppEvent(applyTUIMessageEvent{Message: msg})
	case responseEventMsg:
		return m.handleResponseEvent(msg)
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
		return m.handlePasteMsg(msg)
	case tea.KeyPressMsg:
		if next, cmd, ok := m.handleKeyPressMsg(msg); ok {
			return next, cmd
		}
		return m.updateInputComposer(msg)
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

	return m.updateBubbleTeaChildren(msg)
}

func (m model) handleResponseEvent(msg responseEventMsg) (tea.Model, tea.Cmd) {
	if !m.isActiveResponse(msg.ResponseID) {
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
	msg.Content = normalizeComposerPaste(msg.Content)
	m.resizeInputForValue(m.input.Value() + msg.Content)
	m.input, _ = m.input.Update(msg)
	m.resize()

	return m, nil
}

func (m model) handleKeyPressMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.Keystroke() {
	case "ctrl+c":
		next, cmd := m.confirmExit()
		return next, cmd, true
	case "ctrl+y":
		next, cmd := m.handleAppEvent(copyTranscriptEvent{})
		return next, cmd, true
	case "esc":
		return m, nil, true
	case "ctrl+p":
		next, cmd := m.handleAppEvent(showPreviousPromptEvent{})
		return next, cmd, true
	case "ctrl+n":
		next, cmd := m.handleAppEvent(showNextPromptEvent{})
		return next, cmd, true
	case "shift+enter":
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
			next, cmd := m.handleAppEvent(showPreviousPromptEvent{})
			return next, cmd, true
		case tea.KeyDown:
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
	cmds = append(cmds, cmd)
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)
	m.resize()

	return m, tea.Batch(cmds...)
}

func (m model) updateInputComposer(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.resize()

	return m, cmd
}

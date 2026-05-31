package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
)

var chatsNow = time.Now

type sessionListLoader interface {
	List(context.Context) ([]storage.Session, error)
}

type sessionSwitcher interface {
	Use(context.Context, string) error
	Timeline(context.Context, rpcclient.SessionTimelineOptions) (rpcclient.SessionTimeline, error)
}

type chatsLoadedMsg struct {
	Sessions []storage.Session
	Err      error
}

func (m *model) startChatsCommand() tea.Cmd {
	client, ok := m.sessionClient.(sessionListLoader)
	if m.sessionClient == nil || !ok {
		return m.setStatus("chats unavailable")
	}

	return tea.Batch(
		m.setStatus("loading chats"),
		loadChatsCmd(m.chatCtx, client),
	)
}

func loadChatsCmd(ctx context.Context, client sessionListLoader) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		sessions, err := client.List(ctx)
		return chatsLoadedMsg{Sessions: sessions, Err: err}
	}
}

func (m *model) completeChatsCommand(msg chatsLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("chats unavailable")
	}

	m.showCommandView(commandViewPayload{
		TitleLeft:       "Chats",
		TitleRight:      "esc to close",
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindChats,
		Chats:           orderChatsCommandSessions(msg.Sessions, m.getCurrentSessionID()),
	})

	return nil
}

func renderChatsCommandContent(
	sessions []storage.Session,
	currentSessionID string,
	width int,
	now time.Time,
) string {
	if len(sessions) == 0 {
		return "No chats yet."
	}

	ordered := orderChatsCommandSessions(sessions, currentSessionID)
	rows := make([]string, 0, len(ordered))
	for _, session := range ordered {
		rows = append(rows, renderChatsCommandRow(session, currentSessionID, width, now))
	}

	return strings.Join(rows, "\n")
}

func renderChatsCommandRow(session storage.Session, _ string, width int, now time.Time) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	title := getSessionDisplayName(session)

	activity := formatChatSessionActivity(session.UpdatedAt, now)
	activityWidth := lipgloss.Width(activity)
	titleWidth := max(contentWidth-activityWidth-2, 1)
	title = truncateCommandMenuText(title, titleWidth)
	gap := max(contentWidth-lipgloss.Width(title)-activityWidth, 1)
	row := title + strings.Repeat(" ", gap) + activity
	if width <= 1 {
		return truncateChatsCommandRow(row, width)
	}

	return " " + truncateChatsCommandRow(row, contentWidth) + " "
}

func orderChatsCommandSessions(sessions []storage.Session, currentSessionID string) []storage.Session {
	ordered := append([]storage.Session(nil), sessions...)
	currentSessionID = strings.TrimSpace(currentSessionID)
	if currentSessionID == "" {
		return ordered
	}

	currentIndex := -1
	for index, session := range ordered {
		if strings.TrimSpace(session.ID) == currentSessionID {
			currentIndex = index
			break
		}
	}
	if currentIndex <= 0 {
		return ordered
	}

	current := ordered[currentIndex]
	copy(ordered[1:currentIndex+1], ordered[:currentIndex])
	ordered[0] = current

	return ordered
}

func (m model) isChatsCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindChats
}

func (m model) renderChatsCommandViewContent(content commandViewContent) string {
	sessions := m.commandView.Chats
	if len(sessions) == 0 {
		return "No chats yet."
	}

	offset := min(max(content.Offset, 0), max(len(sessions)-1, 0))
	height := max(content.Height, 1)
	end := min(offset+height, len(sessions))
	rows := make([]string, 0, end-offset)
	now := chatsNow().UTC()
	for index := offset; index < end; index++ {
		row := renderChatsCommandRow(sessions[index], m.getCurrentSessionID(), content.Width, now)
		isCurrent := isCurrentChatSession(sessions[index], m.getCurrentSessionID())
		if index == m.commandViewItemSelected {
			row = renderSelectedChatsCommandRowWithForeground(
				row,
				content.Width,
				getChatsCommandRowForeground(isCurrent),
			)
		} else if !isCurrent {
			row = lipgloss.NewStyle().
				Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
				Render(row)
		}
		rows = append(rows, row)
	}

	for len(rows) <= height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func renderSelectedChatsCommandRow(row string, width int) string {
	return renderSelectedChatsCommandRowWithForeground(
		row,
		width,
		defaultTUITheme.JumpToBottomForeground,
	)
}

func renderSelectedChatsCommandRowWithForeground(row string, width int, foreground string) string {
	width = max(width, 1)
	row = truncateChatsCommandRow(row, width)
	row += strings.Repeat(" ", max(width-lipgloss.Width(row), 0))
	if strings.TrimSpace(foreground) == "" {
		foreground = defaultTUITheme.JumpToBottomForeground
	}

	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(defaultTUITheme.JumpToBottomBackground)).
		Foreground(lipgloss.Color(foreground)).
		Render(row)
}

func getChatsCommandRowForeground(_ bool) string {
	return defaultTUITheme.JumpToBottomForeground
}

func isCurrentChatSession(session storage.Session, currentSessionID string) bool {
	return strings.TrimSpace(session.ID) == strings.TrimSpace(currentSessionID)
}

func truncateChatsCommandRow(row string, width int) string {
	if width <= 0 || lipgloss.Width(row) <= width {
		return row
	}
	if width <= 1 {
		return ""
	}

	runes := []rune(row)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}

	return string(runes) + "…"
}

func (m *model) updateChatsCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.commandView.Chats) == 0 {
		return *m, nil
	}

	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyUp:
			selection--
		case tea.KeyDown:
			selection++
		case tea.KeyHome:
			selection = 0
		case tea.KeyEnd:
			selection = len(m.commandView.Chats) - 1
		case tea.KeyPgUp:
			selection -= max(m.getCommandViewContentHeight(), 1)
		case tea.KeyPgDown:
			selection += max(m.getCommandViewContentHeight(), 1)
		case tea.KeyEnter:
			return m.selectChatsCommandSession()
		default:
			return *m, nil
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			selection--
		case tea.MouseWheelDown:
			selection++
		default:
			return *m, nil
		}
	default:
		return *m, nil
	}

	m.commandViewItemSelected = min(max(selection, 0), len(m.commandView.Chats)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(m.commandView.Chats),
	)
	m.clearCommandViewSelection()

	return *m, nil
}

func (m *model) selectChatsCommandSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat switch unavailable")
	}

	m.applyAction(hideCommandViewAction{})
	m.resize()
	if isCurrentChatSession(session, m.getCurrentSessionID()) {
		return *m, nil
	}

	client, ok := m.sessionClient.(sessionSwitcher)
	if m.sessionClient == nil || !ok {
		return *m, m.setStatus("chat switch unavailable")
	}
	if m.responseCancel != nil {
		m.responseCancel()
	}
	m.resetResponseState()
	m.chatSwitching = true

	return *m, tea.Batch(
		m.setStatus("switching chat"),
		switchChatSessionCmd(m.chatCtx, client, sessionID),
	)
}

func switchChatSessionCmd(ctx context.Context, client sessionSwitcher, sessionID string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return sessionTimelineLoadFailedMsg{Err: errors.New("chat id is required")}
		}
		if err := client.Use(ctx, sessionID); err != nil {
			return sessionTimelineLoadFailedMsg{Err: err}
		}

		timeline, err := client.Timeline(ctx, rpcclient.SessionTimelineOptions{
			SessionID: sessionID,
		})
		if err != nil {
			return sessionTimelineLoadFailedMsg{Err: err}
		}

		return sessionTimelineLoadedMsg{Timeline: timeline}
	}
}

func getChatsCommandViewOffsetForSelection(selection int, offset int, height int, count int) int {
	height = max(height, 1)
	offset = min(max(offset, 0), max(count-height, 0))
	if selection < offset {
		return selection
	}
	if selection >= offset+height {
		return min(max(selection-height+1, 0), max(count-height, 0))
	}

	return offset
}

func formatChatSessionActivity(value time.Time, now time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	if now.IsZero() {
		now = chatsNow().UTC()
	}

	value = value.UTC()
	now = now.UTC()
	if value.After(now) {
		value = now
	}

	elapsed := now.Sub(value)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return formatChatSessionActivityUnit(int(elapsed/time.Minute), "m")
	case elapsed < 24*time.Hour:
		return formatChatSessionActivityUnit(int(elapsed/time.Hour), "h")
	case elapsed < 30*24*time.Hour:
		return formatChatSessionActivityUnit(int(elapsed/(24*time.Hour)), "d")
	case elapsed < 365*24*time.Hour:
		return formatChatSessionActivityUnit(int(elapsed/(30*24*time.Hour)), "mo")
	default:
		return formatChatSessionActivityUnit(int(elapsed/(365*24*time.Hour)), "y")
	}
}

func formatChatSessionActivityUnit(value int, unit string) string {
	return strconv.Itoa(max(value, 1)) + unit + " ago"
}

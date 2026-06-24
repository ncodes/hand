package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	storage "github.com/wandxy/morph/internal/state/core"
)

var chatsNow = time.Now

type sessionListLoader interface {
	List(context.Context, ...rpcclient.SessionListOptions) ([]storage.Session, error)
}

type sessionSwitcher interface {
	Use(context.Context, string) error
	Timeline(context.Context, rpcclient.SessionTimelineOptions) (rpcclient.SessionTimeline, error)
}

type sessionArchiver interface {
	Archive(context.Context, string) error
}

type sessionUnarchiver interface {
	Unarchive(context.Context, string) (storage.Session, error)
}

type sessionRenamer interface {
	Rename(context.Context, string, string) (storage.Session, error)
}

type chatsLoadedMsg struct {
	Sessions []storage.Session
	Err      error
}

type archivedChatsLoadedMsg struct {
	Sessions []storage.Session
	Err      error
}

type chatArchivedMsg struct {
	ID  string
	Err error
}

type chatUnarchivedMsg struct {
	Session storage.Session
	Err     error
}

type chatRenamedMsg struct {
	Session storage.Session
	Err     error
}

func newChatRenameInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "Title"
	input.CharLimit = 80
	input.Focus()

	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		UnsetBackground()
	styles.Focused.Placeholder = styles.Focused.Placeholder.
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		UnsetBackground()
	styles.Focused.Prompt = styles.Focused.Prompt.
		UnsetBackground()
	input.SetStyles(styles)

	return input
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
		TitleRight:      getChatsCommandTitleRight(false),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindChats,
		Chats:           orderChatsCommandSessions(msg.Sessions, m.getCurrentSessionID()),
	})

	return nil
}

func (m *model) startArchiveCommand() tea.Cmd {
	client, ok := m.sessionClient.(sessionListLoader)
	if m.sessionClient == nil || !ok {
		return m.setStatus("archive unavailable")
	}

	return tea.Batch(
		m.setStatus("loading archive"),
		loadArchiveCmd(m.chatCtx, client),
	)
}

func loadArchiveCmd(ctx context.Context, client sessionListLoader) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		archived := true
		sessions, err := client.List(ctx, rpcclient.SessionListOptions{Archived: &archived})
		return archivedChatsLoadedMsg{Sessions: sessions, Err: err}
	}
}

func (m *model) completeArchiveCommand(msg archivedChatsLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("archive unavailable")
	}

	m.showCommandView(commandViewPayload{
		TitleLeft:       "Archive",
		TitleRight:      getArchiveCommandTitleRight(),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindArchive,
		Chats:           orderChatsCommandSessions(msg.Sessions, ""),
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
	if session.Archived {
		activity = "archived"
	}
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

func (m model) isArchiveCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindArchive
}

func (m model) isSessionListCommandView() bool {
	return m.isChatsCommandView() || m.isArchiveCommandView()
}

func (m model) renderSessionListCommandViewContent(content commandViewContent) string {
	sessions := m.commandView.Chats
	if len(sessions) == 0 {
		if m.isArchiveCommandView() {
			return "No archived chats."
		}
		return "No chats yet."
	}

	offset := min(max(content.Offset, 0), max(len(sessions)-1, 0))
	height := max(content.Height, 1)
	end := min(offset+height, len(sessions))
	rows := make([]string, 0, end-offset)
	now := chatsNow().UTC()
	for index := offset; index < end; index++ {
		isCurrent := isCurrentChatSession(sessions[index], m.getCurrentSessionID())
		if m.chatsRenaming && strings.TrimSpace(sessions[index].ID) == m.chatsRenameSessionID {
			rows = append(rows, m.renderChatsRenameRow(content.Width))
			continue
		}

		row := renderChatsCommandRow(sessions[index], m.getCurrentSessionID(), content.Width, now)
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

	for len(rows) <= height+1 {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func (m model) renderChatsCommandViewContent(content commandViewContent) string {
	return m.renderSessionListCommandViewContent(content)
}

func (m model) renderChatsRenameRow(width int) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	input := m.renameInput
	input.SetWidth(contentWidth)
	row := input.View()
	if width > 1 {
		row = " " + truncateChatsCommandRow(row, contentWidth) + " "
	}

	return renderSelectedChatsCommandRow(row, width)
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
	if archived, ok := msg.(chatArchivedMsg); ok {
		return m.completeArchiveChatSession(archived)
	}
	if renamed, ok := msg.(chatRenamedMsg); ok {
		return m.completeRenameChatSession(renamed)
	}

	if len(m.commandView.Chats) == 0 {
		return *m, nil
	}

	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.chatsRenaming {
			return m.updateChatsRenameInput(msg)
		}

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
		case tea.KeyEsc:
			if m.chatsArchiveConfirm {
				m.chatsArchiveConfirm = false
				m.commandView.TitleRight = getChatsCommandTitleRight(false)
				return *m, m.setStatus("chat archive cancelled")
			}
			return *m, nil
		case tea.KeyEnter:
			if m.chatsArchiveConfirm {
				return m.archiveSelectedChatSession()
			}
			return m.selectChatsCommandSession()
		case 'r':
			return m.startRenameSelectedChatSession()
		case 'd':
			return m.confirmArchiveSelectedChatSession()
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
	m.chatsArchiveConfirm = false
	m.commandView.TitleRight = getChatsCommandTitleRight(false)
	m.clearChatsRename()
	m.clearCommandViewSelection()

	return *m, nil
}

func getChatsCommandTitleRight(confirm bool) string {
	if confirm {
		return "enter to archive · esc to cancel"
	}

	return "enter to open · r to rename · d to archive · esc to close"
}

func getChatsRenameTitleRight() string {
	return "enter to save · esc to cancel"
}

func getArchiveCommandTitleRight() string {
	return "enter to restore · u to restore · esc to close"
}

func (m *model) startRenameSelectedChatSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat rename unavailable")
	}
	if session.Archived {
		return *m, m.setStatus("chat rename unavailable")
	}

	m.chatsArchiveConfirm = false
	m.chatsRenaming = true
	m.chatsRenameSessionID = sessionID
	m.renameInput = newChatRenameInput()
	m.renameInput.SetValue(getSessionDisplayName(session))
	m.commandView.TitleRight = getChatsRenameTitleRight()

	return *m, m.setStatus("editing chat title")
}

func (m *model) updateChatsRenameInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Key().Code {
	case tea.KeyEsc:
		m.clearChatsRename()
		m.commandView.TitleRight = getChatsCommandTitleRight(false)
		return *m, m.setStatus("chat rename cancelled")
	case tea.KeyEnter:
		return m.renameSelectedChatSession()
	}

	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return *m, cmd
}

func (m *model) clearChatsRename() {
	m.chatsRenaming = false
	m.chatsRenameSessionID = ""
	m.renameInput.SetValue("")
}

func (m *model) renameSelectedChatSession() (tea.Model, tea.Cmd) {
	sessionID := strings.TrimSpace(m.chatsRenameSessionID)
	if sessionID == "" {
		return *m, m.setStatus("chat rename unavailable")
	}

	title := strings.TrimSpace(m.renameInput.Value())
	if title == "" {
		return *m, m.setStatus("chat rename unavailable")
	}

	client, ok := m.sessionClient.(sessionRenamer)
	if m.sessionClient == nil || !ok {
		return *m, m.setStatus("chat rename unavailable")
	}

	return *m, tea.Batch(
		m.setStatus("renaming chat"),
		renameChatSessionCmd(m.chatCtx, client, sessionID, title),
	)
}

func renameChatSessionCmd(ctx context.Context, client sessionRenamer, sessionID string, title string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return chatRenamedMsg{Err: errors.New("chat id is required")}
		}

		title = strings.TrimSpace(title)
		if title == "" {
			return chatRenamedMsg{Err: errors.New("chat title is required")}
		}

		session, err := client.Rename(ctx, sessionID, title)
		return chatRenamedMsg{Session: session, Err: err}
	}
}

func (m *model) completeRenameChatSession(msg chatRenamedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return *m, m.setStatus("chat rename unavailable")
	}

	sessionID := strings.TrimSpace(msg.Session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat rename unavailable")
	}

	for index, session := range m.commandView.Chats {
		if strings.TrimSpace(session.ID) == sessionID {
			m.commandView.Chats[index] = msg.Session
			break
		}
	}

	if isCurrentChatSession(msg.Session, m.getCurrentSessionID()) {
		m.sessionTitle = getSessionDisplayName(msg.Session)
	}

	m.clearChatsRename()
	m.commandView.TitleRight = getChatsCommandTitleRight(false)
	return *m, m.setStatus("chat renamed")
}

func (m *model) confirmArchiveSelectedChatSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat archive unavailable")
	}
	if session.Archived {
		return *m, m.setStatus("chat archive unavailable")
	}
	if isCurrentChatSession(session, m.getCurrentSessionID()) {
		return *m, m.setStatus("current chat cannot be archived")
	}

	m.chatsArchiveConfirm = true
	m.commandView.TitleRight = getChatsCommandTitleRight(true)
	return *m, m.setStatus("press enter to archive chat")
}

func (m *model) archiveSelectedChatSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		m.chatsArchiveConfirm = false
		m.commandView.TitleRight = getChatsCommandTitleRight(false)
		return *m, m.setStatus("chat archive unavailable")
	}
	if session.Archived {
		m.chatsArchiveConfirm = false
		m.commandView.TitleRight = getChatsCommandTitleRight(false)
		return *m, m.setStatus("chat archive unavailable")
	}
	if isCurrentChatSession(session, m.getCurrentSessionID()) {
		m.chatsArchiveConfirm = false
		m.commandView.TitleRight = getChatsCommandTitleRight(false)
		return *m, m.setStatus("current chat cannot be archived")
	}

	client, ok := m.sessionClient.(sessionArchiver)
	if m.sessionClient == nil || !ok {
		m.chatsArchiveConfirm = false
		m.commandView.TitleRight = getChatsCommandTitleRight(false)
		return *m, m.setStatus("chat archive unavailable")
	}

	m.chatsArchiveConfirm = false
	m.commandView.TitleRight = getChatsCommandTitleRight(false)
	return *m, tea.Batch(
		m.setStatus("archiving chat"),
		archiveChatSessionCmd(m.chatCtx, client, sessionID),
	)
}

func archiveChatSessionCmd(ctx context.Context, client sessionArchiver, sessionID string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return chatArchivedMsg{Err: errors.New("chat id is required")}
		}

		err := client.Archive(ctx, sessionID)
		return chatArchivedMsg{ID: sessionID, Err: err}
	}
}

func (m *model) completeArchiveChatSession(msg chatArchivedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return *m, m.setStatus("chat archive unavailable")
	}

	sessionID := strings.TrimSpace(msg.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat archive unavailable")
	}

	nextSessions := make([]storage.Session, 0, len(m.commandView.Chats))
	for _, session := range m.commandView.Chats {
		if strings.TrimSpace(session.ID) != sessionID {
			nextSessions = append(nextSessions, session)
		}
	}
	m.commandView.Chats = nextSessions
	if len(nextSessions) == 0 {
		m.commandViewItemSelected = 0
		m.commandViewOffset = 0
		return *m, m.setStatus("chat archived")
	}

	m.commandViewItemSelected = min(m.commandViewItemSelected, len(nextSessions)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(nextSessions),
	)

	return *m, m.setStatus("chat archived")
}

func (m *model) updateArchiveCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if unarchived, ok := msg.(chatUnarchivedMsg); ok {
		return m.completeUnarchiveArchivedChatSession(unarchived)
	}

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
			return m.unarchiveSelectedArchivedChatSession()
		case 'u':
			return m.unarchiveSelectedArchivedChatSession()
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

func (m *model) unarchiveSelectedArchivedChatSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat restore unavailable")
	}

	client, ok := m.sessionClient.(sessionUnarchiver)
	if m.sessionClient == nil || !ok {
		return *m, m.setStatus("chat restore unavailable")
	}

	m.commandView.TitleRight = getArchiveCommandTitleRight()
	return *m, tea.Batch(
		m.setStatus("restoring chat"),
		unarchiveChatSessionCmd(m.chatCtx, client, sessionID),
	)
}

func unarchiveChatSessionCmd(ctx context.Context, client sessionUnarchiver, sessionID string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return chatUnarchivedMsg{Err: errors.New("chat id is required")}
		}

		session, err := client.Unarchive(ctx, sessionID)
		return chatUnarchivedMsg{Session: session, Err: err}
	}
}

func (m *model) completeUnarchiveArchivedChatSession(msg chatUnarchivedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return *m, m.setStatus("chat restore unavailable")
	}

	sessionID := strings.TrimSpace(msg.Session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat restore unavailable")
	}

	nextSessions := make([]storage.Session, 0, len(m.commandView.Chats))
	for _, session := range m.commandView.Chats {
		if strings.TrimSpace(session.ID) != sessionID {
			nextSessions = append(nextSessions, session)
		}
	}
	m.commandView.Chats = nextSessions
	if len(nextSessions) == 0 {
		m.commandViewItemSelected = 0
		m.commandViewOffset = 0
		return *m, m.setStatus("chat restored")
	}

	m.commandViewItemSelected = min(m.commandViewItemSelected, len(nextSessions)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(nextSessions),
	)

	return *m, m.setStatus("chat restored")
}

func (m *model) selectChatsCommandSession() (tea.Model, tea.Cmd) {
	session := m.commandView.Chats[m.commandViewItemSelected]
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return *m, m.setStatus("chat switch unavailable")
	}
	if session.Archived {
		return *m, m.setStatus("archived chat cannot be opened")
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

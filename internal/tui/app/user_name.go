package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/profile"
)

const (
	userNameFilename        = "user.json"
	namePromptTitle         = "Hi, there ☺"
	namePromptPlaceholder   = "What can I call you?"
	namePromptSubmitHint    = "Enter to send →"
	namePromptInvalidHint   = "Use letters, numbers, and hyphen only"
	emptyUserPromptQuestion = "What can I do for you?"
	namePromptMaxWidth      = 52
	namePromptInputMinWidth = 28
	namePromptErrorWindow   = 2 * time.Second
)

var validUserName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

type namePromptErrorExpiredMsg struct {
	startedAt time.Time
}

type profileUser struct {
	Name string `json:"name"`
}

func newNameInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = namePromptPlaceholder
	input.CharLimit = 80
	input.SetWidth(namePromptMaxWidth - 4)
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

func loadProfileUserName() (string, bool, bool, error) {
	path := profileUserPath()
	if path == "" {
		return noticeBarName, true, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, true, nil
		}

		return "", false, false, fmt.Errorf("read user profile: %w", err)
	}

	var user profileUser
	if err := json.Unmarshal(data, &user); err != nil {
		return "", false, false, fmt.Errorf("parse user profile: %w", err)
	}

	name := strings.TrimSpace(user.Name)
	return name, name != "", false, nil
}

func saveProfileUserName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	path := profileUserPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create profile metadata dir: %w", err)
	}

	data, err := json.MarshalIndent(profileUser{Name: name}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func profileUserPath() string {
	active := profile.WithMetadataPaths(profile.Active())
	home := strings.TrimSpace(active.HomeDir)
	if home == "" {
		return ""
	}

	return filepath.Join(home, userNameFilename)
}

func (m model) shouldShowNamePrompt() bool {
	return m.namePromptEnabled &&
		strings.TrimSpace(m.userName) == "" &&
		len(m.messages) == 0 &&
		(m.live == nil || m.live.IsEmpty())
}

func (m model) shouldShowEmptyUserPrompt() bool {
	return !m.shouldShowNamePrompt() &&
		strings.TrimSpace(m.userDisplayName()) != "" &&
		len(m.messages) == 0 &&
		(m.live == nil || m.live.IsEmpty())
}

func (m model) userDisplayName() string {
	if name := strings.TrimSpace(m.userName); name != "" {
		return name
	}

	return noticeBarName
}

func (m model) renderNamePrompt() string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	height := max(m.transcript.Height(), 1)
	boxWidth := min(max(width/2, namePromptInputMinWidth), min(namePromptMaxWidth, width))
	inputWidth := max(boxWidth-4, 1)
	input := m.nameInput
	input.SetWidth(inputWidth)

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render(namePromptTitle)
	mark := renderHandBanner(handHeaderMark)
	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Padding(0, 1).
		Width(boxWidth).
		Render(input.View())
	hintText := namePromptSubmitHint
	hintColor := defaultTUITheme.MutedText
	if errorText := strings.TrimSpace(m.namePromptError); errorText != "" {
		hintText = errorText
		hintColor = defaultTUITheme.ToolDeletion
	}
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color(hintColor)).
		Render(hintText)
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		mark,
		"",
		title,
		"",
		inputBox,
		hint,
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m model) renderEmptyUserPrompt() string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	header := strings.Trim(m.renderHeaderWithWidth(width), "\n")
	headerHeight := lipgloss.Height(header)
	height := max(m.transcript.Height()-headerHeight, 1)
	name := m.userDisplayName()
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render("Hi, " + name + " ☺")
	question := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Render(emptyUserPromptQuestion)
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		question,
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m model) renderEmptyUserPromptContent() string {
	width := m.transcript.Width()
	if width <= 0 {
		width = m.getMainPaneWidth()
	}
	header := strings.Trim(m.renderHeaderWithWidth(width), "\n")

	return strings.TrimRight(lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.renderEmptyUserPrompt(),
	), "\n")
}

func (m model) handleNamePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Key().Code == tea.KeyEnter {
		return m.submitNamePrompt()
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)

	return m, cmd
}

func (m model) handleNamePromptPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	m.nameInput.SetValue(m.nameInput.Value() + normalizeComposerPaste(msg.Content))

	return m, nil
}

func (m model) submitNamePrompt() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return m.setNamePromptError("name is required")
	}
	if !validUserName.MatchString(name) {
		return m.setNamePromptError(namePromptInvalidHint)
	}
	if err := saveProfileUserName(name); err != nil {
		return m, m.setStatus("name unavailable")
	}

	m.userName = name
	m.namePromptEnabled = false
	m.nameInput.SetValue("")
	m.resize()
	m.setTranscriptContent()

	return m, m.setStatus("name saved")
}

func (m model) setNamePromptError(text string) (tea.Model, tea.Cmd) {
	m.namePromptError = strings.TrimSpace(text)
	m.namePromptErrorStartedAt = currentTime()
	startedAt := m.namePromptErrorStartedAt

	return m, tea.Tick(namePromptErrorWindow, func(time.Time) tea.Msg {
		return namePromptErrorExpiredMsg{startedAt: startedAt}
	})
}

func (m model) expireNamePromptError(msg namePromptErrorExpiredMsg) tea.Model {
	if m.namePromptErrorStartedAt.IsZero() || !m.namePromptErrorStartedAt.Equal(msg.startedAt) {
		return m
	}

	m.namePromptError = ""
	m.namePromptErrorStartedAt = time.Time{}
	return m
}

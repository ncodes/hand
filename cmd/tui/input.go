package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	inputFrameHorizontalPadding = 1
	inputFrameVerticalPadding   = 0
	inputFrameBorderWidth       = 2
	inputFrameChromeHeight      = inputFrameBorderWidth + inputFrameVerticalPadding*2
	bottomStatusPanelHeight     = 1
	minInputHeight              = 1
	inputPrompt                 = "❯ "
	inputFrameBackground        = "#050505"
)

// newInputComposer creates the multiline prompt editor.
func newInputComposer() textarea.Model {
	input := textarea.New()
	input.Placeholder = "Ask Hand..."
	input.SetPromptFunc(lipgloss.Width(inputPrompt), renderInputPrompt)
	input.ShowLineNumbers = false
	setInputTransparentStyles(&input)
	input.SetHeight(1)
	input.Focus()

	return input
}

// setInputTransparentStyles removes Bubble's default focused-line background.
func setInputTransparentStyles(input *textarea.Model) {
	styles := input.Styles()
	styles.Focused.Base = styles.Focused.Base.UnsetBackground()
	styles.Focused.Text = styles.Focused.Text.UnsetBackground()
	styles.Focused.Placeholder = styles.Focused.Placeholder.UnsetBackground()
	styles.Focused.Prompt = styles.Focused.Prompt.UnsetBackground()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Focused.EndOfBuffer = styles.Focused.EndOfBuffer.UnsetBackground()
	styles.Blurred.Base = styles.Blurred.Base.UnsetBackground()
	styles.Blurred.Text = styles.Blurred.Text.UnsetBackground()
	styles.Blurred.Placeholder = styles.Blurred.Placeholder.UnsetBackground()
	styles.Blurred.Prompt = styles.Blurred.Prompt.UnsetBackground()
	styles.Blurred.CursorLine = styles.Blurred.CursorLine.UnsetBackground()
	styles.Blurred.EndOfBuffer = styles.Blurred.EndOfBuffer.UnsetBackground()
	input.SetStyles(styles)
}

// renderInputPrompt shows the arrow only on the first visible row.
func renderInputPrompt(info textarea.PromptInfo) string {
	if info.LineNumber == 0 {
		return inputPrompt
	}

	return ""
}

// renderInput draws the composer and its model/context/status row.
func (m model) renderInput() string {
	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderTop(true).
		BorderRight(true).
		BorderBottom(true).
		BorderLeft(true).
		Background(lipgloss.Color(inputFrameBackground)).
		BorderForeground(lipgloss.Color(m.getInputFrameBorderColor())).
		Padding(inputFrameVerticalPadding, inputFrameHorizontalPadding).
		Width(getInputBoxWidth(m.width)).
		Render(m.input.View())

	return lipgloss.JoinVertical(lipgloss.Left, inputBox, m.renderBottomStatusPanel())
}

func (m model) getInputFrameBorderColor() string {
	if !m.isThinkingComposerVisible() {
		return "8"
	}

	return getThinkingComposerBorderColor(m.thinkingComposerFrame)
}

// resize distributes terminal rows between transcript and composer.
func (m *model) resize() {
	m.input.SetWidth(getInputInnerWidth(m.width))
	inputHeight := m.getInputHeight()
	transcriptHeight := max(m.height-inputHeight-inputChromeHeight, 1)

	m.input.SetHeight(inputHeight)
	m.transcript.SetWidth(getPanelContentWidth(m.width))
	m.transcript.SetHeight(transcriptHeight)
}

// getInputHeight returns the visible composer height constrained by the screen.
func (m model) getInputHeight() int {
	return m.getInputHeightForValue(m.input.Value())
}

func (m model) getInputHeightForValue(value string) int {
	availableHeight := max(m.height-inputChromeHeight-1, minInputHeight)
	contentWidth := m.input.Width()
	if contentWidth <= 0 {
		contentWidth = getInputInnerWidth(m.width)
	}
	contentHeight := getInputHeight(value, contentWidth)

	return min(contentHeight, availableHeight)
}

func (m *model) resizeInputForValue(value string) {
	m.input.SetWidth(getInputInnerWidth(m.width))
	m.input.SetHeight(m.getInputHeightForValue(value))
}

// insertInputNewline expands the composer before adding a newline.
func (m model) insertInputNewline() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	inputWidth := getInputInnerWidth(m.width)
	availableHeight := max(m.height-inputChromeHeight-1, minInputHeight)
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(min(getInputHeight(m.input.Value()+"\n", m.input.Width()), availableHeight))
	m.input, cmd = m.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.resize()

	return m, cmd
}

// deleteInputLine clears the current logical composer line.
func (m model) deleteInputLine() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input.CursorEnd()
	m.input, cmd = m.input.Update(tea.KeyPressMsg(tea.Key{
		Code: 'u',
		Mod:  tea.ModCtrl,
	}))
	m.resize()

	return m, cmd
}

// getInputBoxWidth returns the full composer width.
func getInputBoxWidth(width int) int {
	return max(width, 1)
}

// getInputInnerWidth returns the textarea wrapping width inside the composer.
func getInputInnerWidth(width int) int {
	return max(
		getInputBoxWidth(width)-inputFrameBorderWidth-(inputFrameHorizontalPadding*2),
		1,
	)
}

// getInputHeight returns the number of visible rows needed for the value.
func getInputHeight(value string, width int) int {
	if value == "" {
		return minInputHeight
	}

	height := 0
	for _, line := range strings.Split(value, "\n") {
		height += getWrappedLineHeight(line, width)
	}

	return max(height, minInputHeight)
}

// getWrappedLineHeight returns how many terminal rows a line occupies.
func getWrappedLineHeight(line string, width int) int {
	width = max(width, 1)
	lineWidth := lipgloss.Width(line)
	if lineWidth == 0 {
		return 1
	}

	return max((lineWidth+width-1)/width, 1)
}

// isInputLineDeleteKey reports whether a key should clear the current row.
func isInputLineDeleteKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	switch {
	case key.Code == 'u' && key.Mod.Contains(tea.ModCtrl):
		return true
	case key.Code == tea.KeyBackspace || key.Code == tea.KeyDelete:
		return key.Mod.Contains(tea.ModSuper) ||
			key.Mod.Contains(tea.ModMeta) ||
			key.Mod.Contains(tea.ModCtrl)
	default:
		return false
	}
}

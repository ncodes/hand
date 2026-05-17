package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

const (
	inputHorizontalPadding = 2
	minInputHeight         = 1
	inputPrompt            = "❯ "
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
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Width(getInputBoxWidth(m.width)).
		Render(m.input.View())

	return lipgloss.JoinVertical(lipgloss.Left, inputBox, m.renderInputInfo())
}

// renderInputInfo renders the compact metadata row below the composer.
func (m model) renderInputInfo() string {
	availableWidth := getInputBoxWidth(m.width)
	status := m.status.Text()

	left := joinInputInfoSegments([]string{m.modelName, status}, availableWidth)
	right := strings.TrimSpace(m.context)
	if m.hasPendingExitConfirmation() {
		left = status
		right = ""
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Width(availableWidth).
		Render(spaceBetweenInputInfo(left, right, availableWidth))
}

// joinInputInfoSegments joins metadata while preserving narrow-screen fallback.
func joinInputInfoSegments(segments []string, width int) string {
	visible := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment = strings.TrimSpace(segment); segment != "" {
			visible = append(visible, segment)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	if len(visible) == 1 {
		return visible[0]
	}

	separator := "  ·  "
	value := strings.Join(visible, separator)
	if lipgloss.Width(value) <= width {
		return value
	}

	return strings.Join(visible, " · ")
}

// spaceBetweenInputInfo pushes context usage to the right edge when possible.
func spaceBetweenInputInfo(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 0 {
		return left + " · " + right
	}

	return left + strings.Repeat(" ", gap) + right
}

// getInputBoxWidth returns the full composer width.
func getInputBoxWidth(width int) int {
	return max(width, 1)
}

// getInputInnerWidth returns the textarea wrapping width inside the composer.
func getInputInnerWidth(width int) int {
	return max(getInputBoxWidth(width)-inputHorizontalPadding, 1)
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

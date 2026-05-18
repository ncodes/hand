package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderBottomStatusPanel renders the compact bottom status panel below the composer.
func (m model) renderBottomStatusPanel() string {
	availableWidth := getInputBoxWidth(m.width)
	status := m.status.Text()

	left := joinBottomStatusPanelSegments([]string{m.modelName, status}, availableWidth)
	right := strings.TrimSpace(m.context)
	if m.hasPendingExitConfirmation() {
		left = status
		right = ""
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Width(availableWidth).
		Render(spaceBetweenBottomStatusPanel(left, right, availableWidth))
}

// joinBottomStatusPanelSegments joins metadata while preserving narrow-screen fallback.
func joinBottomStatusPanelSegments(segments []string, width int) string {
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

// spaceBetweenBottomStatusPanel pushes context usage to the right edge when possible.
func spaceBetweenBottomStatusPanel(left, right string, width int) string {
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

package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/wandxy/morph/pkg/stringx"
)

type bottomStatusPanelRenderer interface {
	Render(bottomStatusPanel) string
}

type lipglossBottomStatusPanelRenderer struct{}

var defaultBottomStatusPanelRenderer bottomStatusPanelRenderer = lipglossBottomStatusPanelRenderer{}

func (lipglossBottomStatusPanelRenderer) Render(panel bottomStatusPanel) string {
	segments := []string{
		renderBottomStatusMutedCell(panel.ModelName),
		renderBottomStatusMutedCell(panel.Status),
	}
	if panel.Thinking {
		segments = append([]string{renderThinkingStatusCell(panel.ThinkingFrame)}, segments...)
	}

	left := joinBottomStatusPanelRenderedSegments(segments, panel.ContentWidth)
	center := renderBottomStatusMutedCell(panel.SessionTitle)
	right := renderBottomStatusMutedCell(panel.Context)
	if panel.ExitConfirmation {
		left = renderBottomStatusMutedCell(panel.Status)
		center = ""
		right = ""
	}

	return lipgloss.NewStyle().
		Padding(0, panel.HorizontalPadding).
		Width(panel.Width).
		Render(spaceAroundBottomStatusPanel(left, center, right, panel.ContentWidth))
}

func renderBottomStatusMutedCell(text string) string {
	text = stringx.String(text).Trim()
	if text == "" {
		return ""
	}

	return renderBottomStatusMutedText(text)
}

func renderBottomStatusMutedText(text string) string {
	if text == "" {
		return ""
	}

	return lipgloss.NewStyle().
		Inline(true).
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Render(text)
}

func joinBottomStatusPanelRenderedSegments(segments []string, width int) string {
	visible := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment = stringx.String(segment).Trim(); segment != "" {
			visible = append(visible, segment)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	if len(visible) == 1 {
		return visible[0]
	}

	wideSeparator := renderBottomStatusMutedText("  ·  ")
	value := strings.Join(visible, wideSeparator)
	if lipgloss.Width(value) <= width {
		return value
	}

	return strings.Join(visible, renderBottomStatusMutedText(" · "))
}

// joinBottomStatusPanelSegments joins metadata while preserving narrow-screen fallback.
func joinBottomStatusPanelSegments(segments []string, width int) string {
	visible := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment = stringx.String(segment).Trim(); segment != "" {
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
	left = stringx.String(left).Trim()
	right = stringx.String(right).Trim()
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 0 {
		return left + renderBottomStatusMutedText(" · ") + right
	}

	return left + strings.Repeat(" ", gap) + right
}

// spaceAroundBottomStatusPanel centers the session title while keeping metadata at the edges.
func spaceAroundBottomStatusPanel(left, center, right string, width int) string {
	left = stringx.String(left).Trim()
	center = stringx.String(center).Trim()
	right = stringx.String(right).Trim()
	if center == "" {
		return spaceBetweenBottomStatusPanel(left, right, width)
	}

	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)
	centerStart := max((width-centerWidth)/2, leftWidth+1)
	rightStart := width - rightWidth
	if right == "" {
		rightStart = width
	}
	if centerStart+centerWidth >= rightStart {
		return spaceBetweenBottomStatusPanel(
			joinBottomStatusPanelRenderedSegments([]string{left, center}, width),
			right,
			width,
		)
	}

	var out strings.Builder
	out.WriteString(left)
	out.WriteString(strings.Repeat(" ", max(centerStart-lipgloss.Width(out.String()), 1)))
	out.WriteString(center)
	if right != "" {
		out.WriteString(strings.Repeat(" ", max(rightStart-lipgloss.Width(out.String()), 1)))
		out.WriteString(right)
	}

	return out.String()
}

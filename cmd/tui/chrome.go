package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/constants"
)

const handBanner = `░██     ░██  ██████   ███████      ░██
░██████████ ░░░░░░██ ░░██░░░██  ██████
░██░░░░░░██  ███████  ░██  ░██ ██░░░██
░██     ░██ ██░░░░██  ░██  ░██░██  ░██
░██     ░██░░████████ ███  ░██░░██████`

const compactHandBanner = ` _  _              _ 
| || |__ _ _ _  __| |
| __ / _` + "`" + ` | ' \/ _` + "`" + ` |
|_||_\__,_|_||_\__,_|`

const tinyHandBanner = `Hand`

const (
	headerBorderHeight    = 1
	noticeBarHeight       = 1
	noticeBarMarginBottom = 1
	headerInfoKeyWidth    = 9
	headerGapWidth        = 2
)

const (
	noticeBarLeftLead = "Welcome, "
	noticeBarName     = "Kennedy"
	noticeBarLead     = "Use "
	noticeBarLink     = "/changelogs"
	noticeBarTail     = " to see what changed"
	noticeBarBG       = "#292929"
)

var handBannerColors = []color.Color{
	lipgloss.Color("38"),
	lipgloss.Color("44"),
	lipgloss.Color("49"),
	lipgloss.Color("48"),
	lipgloss.Color("83"),
}

// renderHeader draws the fixed title bar.
func (m model) renderHeader() string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("#242424")).
		Width(m.width).
		Render(m.renderHeaderContent())
}

// renderHeaderContent arranges the banner and runtime info panel.
func (m model) renderHeaderContent() string {
	parts := []string{m.renderNoticeBar()}
	for range noticeBarMarginBottom {
		parts = append(parts, "")
	}
	parts = append(parts, m.renderHeaderBody())

	return strings.Join(parts, "\n")
}

// renderNoticeBar draws the solid announcement row above the banner.
func (m model) renderNoticeBar() string {
	content := renderNoticeBarContent(renderNoticeBarLeft(), renderNoticeBarRight(), m.width)

	return lipgloss.NewStyle().
		Background(lipgloss.Color(noticeBarBG)).
		Foreground(lipgloss.Color("15")).
		Width(m.width).
		Render(content)
}

// renderNoticeBarContent joins notice bar segments with a styled spacer.
func renderNoticeBarContent(left string, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if width <= lipgloss.Width(left) || right == "" {
		return left
	}

	spacerWidth := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacerWidth < 1 {
		return left
	}

	spacer := lipgloss.NewStyle().
		Background(lipgloss.Color(noticeBarBG)).
		Render(strings.Repeat(" ", spacerWidth))

	return left + spacer + right
}

// renderNoticeBarLeft highlights the user name while keeping the greeting muted.
func renderNoticeBarLeft() string {
	muted := lipgloss.NewStyle().
		Padding(0, 0, 0, 1).
		Background(lipgloss.Color(noticeBarBG)).
		Foreground(lipgloss.Color("#A0A0A0"))
	highlight := lipgloss.NewStyle().
		Background(lipgloss.Color(noticeBarBG)).
		Foreground(lipgloss.Color("#FFFFFF"))

	return muted.Render(noticeBarLeftLead) +
		highlight.Render(noticeBarName)
}

// renderNoticeBarRight styles the right-side notice command hint.
func renderNoticeBarRight() string {
	muted := lipgloss.NewStyle().
		Background(lipgloss.Color(noticeBarBG)).
		Foreground(lipgloss.Color("#A0A0A0"))
	highlight := lipgloss.NewStyle().
		Background(lipgloss.Color(noticeBarBG)).
		Foreground(lipgloss.Color("#FFFFFF"))

	return muted.Render(noticeBarLead) +
		highlight.Render(noticeBarLink) +
		muted.Padding(0, 1, 0, 0).Render(noticeBarTail)
}

// renderHeaderBody arranges the banner and runtime info panel.
func (m model) renderHeaderBody() string {
	banner := m.getHeaderBanner()
	left := renderHandBanner(banner)
	right := m.renderHeaderInfoPanel()
	if right == "" {
		return left
	}

	right = alignHeaderInfoPanel(right, lipgloss.Height(banner))
	availableLeftWidth := max(m.width-lipgloss.Width(right)-headerGapWidth, lipgloss.Width(banner))
	left = lipgloss.NewStyle().
		Width(availableLeftWidth).
		Render(left)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", headerGapWidth), right)
}

// renderHeaderInfoPanel renders the right-hand runtime information panel.
func (m model) renderHeaderInfoPanel() string {
	rows := getHeaderInfoRows(m)
	infoWidth := getHeaderInfoWidth(rows)
	if m.width < lipgloss.Width(handBanner)+headerGapWidth+infoWidth {
		return ""
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		key := lipgloss.NewStyle().
			Width(headerInfoKeyWidth).
			Align(lipgloss.Right).
			Render(row.key)

		lines = append(lines, key+": "+row.value)
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Width(infoWidth).
		Render(strings.Join(lines, "\n"))
}

type headerInfoRow struct {
	key   string
	value string
}

// getHeaderInfoRows returns the runtime metadata rows shown in the header.
func getHeaderInfoRows(m model) []headerInfoRow {
	return []headerInfoRow{
		{key: "version", value: "v0.1 alpha"},
		{key: "provider", value: "openrouter"},
		{key: "model", value: getModelDisplayName(m.modelName)},
		{key: "embedding", value: getModelDisplayName("text-embedding-3-small")},
		{key: "summary", value: getModelDisplayName(constants.DefaultProfileSummaryModel)},
	}
}

// getHeaderInfoWidth returns the narrowest panel width that keeps values intact.
func getHeaderInfoWidth(rows []headerInfoRow) int {
	maxValueWidth := 0
	for _, row := range rows {
		maxValueWidth = max(maxValueWidth, lipgloss.Width(row.value))
	}

	return headerInfoKeyWidth + 2 + maxValueWidth
}

// alignHeaderInfoPanel pads runtime info so it sits vertically centered beside the banner.
func alignHeaderInfoPanel(info string, targetHeight int) string {
	infoHeight := lipgloss.Height(info)
	if info == "" || infoHeight >= targetHeight {
		return info
	}

	topPadding := (targetHeight - infoHeight) / 2
	bottomPadding := targetHeight - infoHeight - topPadding

	return strings.Repeat("\n", topPadding) + info + strings.Repeat("\n", bottomPadding)
}

// getModelDisplayName removes the provider or owner prefix from a model identifier.
func getModelDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if _, model, ok := strings.Cut(name, "/"); ok {
		return strings.TrimSpace(model)
	}

	return name
}

// getHeaderBanner returns the widest banner that can fit without clipping.
func (m model) getHeaderBanner() string {
	if m.width >= lipgloss.Width(handBanner) {
		return handBanner
	}
	if m.width >= lipgloss.Width(compactHandBanner) {
		return compactHandBanner
	}

	return tinyHandBanner
}

// getHeaderHeight returns the rendered height reserved for the current banner.
func (m model) getHeaderHeight() int {
	headerHeight := lipgloss.Height(m.getHeaderBanner())
	if infoPanel := m.renderHeaderInfoPanel(); infoPanel != "" {
		headerHeight = max(headerHeight, lipgloss.Height(infoPanel))
	}

	return headerHeight + noticeBarHeight + noticeBarMarginBottom + headerBorderHeight
}

// renderHandBanner renders the generated figlet masthead.
func renderHandBanner(banner string) string {
	lines := strings.Split(banner, "\n")
	for index, line := range lines {
		lines[index] = lipgloss.NewStyle().
			Foreground(getHandBannerColor(index)).
			Render(line)
	}

	return strings.Join(lines, "\n")
}

// getHandBannerColor returns the stable lolcat-inspired color for a banner row.
func getHandBannerColor(index int) color.Color {
	if index >= 0 && index < len(handBannerColors) {
		return handBannerColors[index]
	}

	return handBannerColors[len(handBannerColors)-1]
}

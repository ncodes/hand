package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type chromeRenderer interface {
	RenderHeader(headerPanel) string
	RenderNoticeBar(noticePanel) string
}

type lipglossChromeRenderer struct{}

var defaultChromeRenderer chromeRenderer = lipglossChromeRenderer{}

var handBannerColors = []color.Color{
	lipgloss.Color("38"),
	lipgloss.Color("44"),
	lipgloss.Color("49"),
	lipgloss.Color("48"),
	lipgloss.Color("83"),
}

func (lipglossChromeRenderer) RenderHeader(panel headerPanel) string {
	width := max(panel.Width, 1)

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color(defaultTUITheme.NoticeBorder)).
		Width(width).
		Render(renderHeaderContent(panel))
}

func (lipglossChromeRenderer) RenderNoticeBar(panel noticePanel) string {
	return renderNoticePanel(panel)
}

func renderHeaderContent(panel headerPanel) string {
	notice := panel.Notice
	notice.Width = panel.Width
	parts := []string{renderNoticePanel(notice)}
	for range noticeBarMarginBottom {
		parts = append(parts, "")
	}
	parts = append(parts, renderHeaderBody(panel))

	return strings.Join(parts, "\n")
}

func renderNoticePanel(panel noticePanel) string {
	width := max(panel.Width, 1)
	content := renderNoticeBarContent(renderNoticePanelLeft(panel), renderNoticePanelRight(panel), width)

	return lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Width(width).
		Render(content)
}

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
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Render(strings.Repeat(" ", spacerWidth))

	return left + spacer + right
}

func renderNoticePanelLeft(panel noticePanel) string {
	muted := lipgloss.NewStyle().
		Padding(0, 0, 0, 1).
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeMuted))
	highlight := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground))

	return muted.Render(panel.LeftLead) + highlight.Render(panel.Name)
}

func renderNoticePanelRight(panel noticePanel) string {
	muted := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeMuted))
	highlight := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.NoticeBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground))

	return muted.Render(panel.Lead) +
		highlight.Render(panel.Link) +
		muted.Padding(0, 1, 0, 0).Render(panel.Tail)
}

func renderHeaderBody(panel headerPanel) string {
	bodyWidth := getHeaderBodyContentWidth(panel.Width)
	left := renderHandBannerWithColors(panel.Banner, panel.BannerRows)
	right := renderHeaderInfoPanel(panel)
	if right == "" {
		return padHeaderBody(left, panel.Width)
	}

	right = alignHeaderInfoPanel(right, lipgloss.Height(panel.Banner))
	availableLeftWidth := max(bodyWidth-lipgloss.Width(right)-headerGapWidth, lipgloss.Width(panel.Banner))
	left = lipgloss.NewStyle().
		Width(availableLeftWidth).
		Render(left)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", headerGapWidth), right)
	return padHeaderBody(body, panel.Width)
}

func padHeaderBody(body string, width int) string {
	width = max(width, 1)
	lines := strings.Split(body, "\n")
	for index, line := range lines {
		lineWidth := lipgloss.Width(line)
		if width <= headerBodyPadding*2 || lineWidth >= width {
			lines[index] = line
			continue
		}

		rightPadding := max(width-lineWidth-headerBodyPadding, 0)
		lines[index] = strings.Repeat(" ", headerBodyPadding) + line + strings.Repeat(" ", rightPadding)
	}

	return strings.Join(lines, "\n")
}

func renderHeaderInfoPanel(panel headerPanel) string {
	if !panel.ShowInfo {
		return ""
	}

	lines := make([]string, 0, len(panel.InfoRows))
	for _, row := range panel.InfoRows {
		key := lipgloss.NewStyle().
			Width(headerInfoKeyWidth).
			Align(lipgloss.Right).
			Render(row.key)

		lines = append(lines, key+": "+row.value)
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Width(getHeaderInfoWidth(panel.InfoRows)).
		Render(strings.Join(lines, "\n"))
}

func renderHandBannerWithColors(banner string, colors []color.Color) string {
	if len(colors) == 0 {
		colors = handBannerColors
	}

	lines := strings.Split(banner, "\n")
	for index, line := range lines {
		colorIndex := min(index, len(colors)-1)
		lines[index] = lipgloss.NewStyle().
			Foreground(colors[colorIndex]).
			Render(line)
	}

	return strings.Join(lines, "\n")
}

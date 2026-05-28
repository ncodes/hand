package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/brand"
)

const handBanner = brand.Wordmark

const compactHandBanner = ` _  _              _ 
| || |__ _ _ _  __| |
| __ / _` + "`" + ` | ' \/ _` + "`" + ` |
|_||_\__,_|_||_\__,_|`

const tinyHandBanner = `Hand`

const handHeaderMark = brand.Mark

const (
	headerBorderHeight    = 1
	noticeBarHeight       = 1
	noticeBarMarginBottom = 1
	headerBodyPadding     = 1
	headerInfoKeyWidth    = 9
	headerInfoColumnGap   = 2
	headerGapWidth        = 2
)

const (
	noticeBarLeftLead = "Welcome, "
	noticeBarName     = "Kennedy"
	noticeBarLead     = "Use "
	noticeBarLink     = "/changelogs"
	noticeBarTail     = " to see what changed"
)

// renderHeader draws the fixed title bar.
func (m model) renderHeader() string {
	return m.renderHeaderWithWidth(m.width)
}

func (m model) renderHeaderWithWidth(width int) string {
	return defaultChromeRenderer.RenderHeader(getHeaderPanel(m, width))
}

// renderNoticeBar draws the solid announcement row above the banner.
func (m model) renderNoticeBar() string {
	return defaultChromeRenderer.RenderNoticeBar(getNoticePanel(m.width))
}

// renderNoticeBarLeft highlights the user name while keeping the greeting muted.
func renderNoticeBarLeft() string {
	return renderNoticePanelLeft(getNoticePanel(defaultWidth))
}

// renderNoticeBarRight styles the right-side notice command hint.
func renderNoticeBarRight() string {
	return renderNoticePanelRight(getNoticePanel(defaultWidth))
}

// renderHeaderBody arranges the banner and runtime info panel.
func (m model) renderHeaderBody() string {
	return renderHeaderBody(getHeaderPanel(m, m.width))
}

// renderHeaderInfoPanel renders the right-hand runtime information panel.
func (m model) renderHeaderInfoPanel() string {
	return renderHeaderInfoPanel(getHeaderPanel(m, m.width))
}

type headerInfoRow struct {
	key   string
	value string
}

type headerPanel struct {
	Width      int
	Banner     string
	Mark       string
	Notice     noticePanel
	InfoRows   []headerInfoRow
	ShowInfo   bool
	BannerRows []color.Color
}

type noticePanel struct {
	Width    int
	LeftLead string
	Name     string
	Lead     string
	Link     string
	Tail     string
}

func getHeaderPanel(m model, width int) headerPanel {
	width = max(width, 1)
	rows := getHeaderInfoRows(m)
	infoWidth := getHeaderInfoWidth(rows)
	bodyWidth := getHeaderBodyContentWidth(width)
	banner := getHeaderBanner(bodyWidth)
	mark := getHeaderMark(bodyWidth, banner)
	bannerWidth := getHeaderBannerGroupWidth(mark, banner)

	return headerPanel{
		Width:    width,
		Banner:   banner,
		Mark:     mark,
		Notice:   getNoticePanel(width),
		InfoRows: rows,
		ShowInfo: bodyWidth >= bannerWidth+headerGapWidth+infoWidth,
	}
}

func getHeaderBodyContentWidth(width int) int {
	return max(width-headerBodyPadding*2, 1)
}

func getNoticePanel(width int) noticePanel {
	return noticePanel{
		Width:    max(width, 1),
		LeftLead: noticeBarLeftLead,
		Name:     noticeBarName,
		Lead:     noticeBarLead,
		Link:     noticeBarLink,
		Tail:     noticeBarTail,
	}
}

// getHeaderInfoRows returns the runtime metadata rows shown in the header.
func getHeaderInfoRows(m model) []headerInfoRow {
	info := m.runtimeInfo
	return []headerInfoRow{
		{key: "version", value: getRuntimeValue(info.Version, "dev")},
		{key: "commit", value: getRuntimeValue(info.Commit, "unknown")},
		{key: "profile", value: getRuntimeValue(info.Profile, "default")},
		{key: "session", value: getRuntimeValue(m.sessionID, "default")},
		{key: "provider", value: getRuntimeValue(info.Provider, "openrouter")},
		{key: "model", value: getModelDisplayName(getRuntimeValue(m.modelName, info.Model))},
		{key: "summary", value: getModelDisplayName(info.SummaryModel)},
		{key: "embedding", value: getModelDisplayName(info.EmbeddingModel)},
		{key: "storage", value: getRuntimeValue(info.Storage, "sqlite")},
		{key: "streaming", value: getRuntimeValue(info.Streaming, "on")},
	}
}

// getHeaderInfoWidth returns the narrowest panel width that keeps values intact.
func getHeaderInfoWidth(rows []headerInfoRow) int {
	columnWidth := getHeaderInfoColumnWidth(rows)
	if len(rows) <= 1 {
		return columnWidth
	}

	return columnWidth*2 + headerInfoColumnGap
}

func getHeaderInfoColumnWidth(rows []headerInfoRow) int {
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
	return getHeaderBanner(m.width)
}

func getHeaderBanner(width int) string {
	if width >= lipgloss.Width(handBanner) {
		return handBanner
	}
	if width >= lipgloss.Width(compactHandBanner) {
		return compactHandBanner
	}

	return tinyHandBanner
}

func getHeaderMark(width int, banner string) string {
	if banner != handBanner {
		return ""
	}
	if width < lipgloss.Width(handHeaderMark)+headerGapWidth+lipgloss.Width(handBanner) {
		return ""
	}

	return handHeaderMark
}

func getHeaderBannerGroupWidth(mark string, banner string) int {
	width := lipgloss.Width(banner)
	if mark == "" {
		return width
	}

	return lipgloss.Width(mark) + headerGapWidth + width
}

// renderHandBanner renders the generated figlet masthead.
func renderHandBanner(banner string) string {
	return renderHandBannerWithColors(banner, nil)
}

// getHandBannerColor returns the stable lolcat-inspired color for a banner row.
func getHandBannerColor(index int) color.Color {
	if index >= 0 && index < len(handBannerColors) {
		return handBannerColors[index]
	}

	return handBannerColors[len(handBannerColors)-1]
}

package tui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

const transcriptSelectionAutoScrollInterval = 60 * time.Millisecond

type transcriptSelectionAutoScrollTickMsg struct{}

type transcriptSelectionPoint struct {
	line   int
	offset int
}

type transcriptSelection struct {
	active   bool
	dragging bool
	content  string
	start    transcriptSelectionPoint
	end      transcriptSelectionPoint
	mouse    tea.Mouse
	scroll   int
	ticking  bool
}

func (m *model) startTranscriptSelection(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft {
		return false
	}
	if !m.isMouseInTranscript(msg.Mouse()) {
		return false
	}
	if m.selection.active {
		m.restoreTranscriptContentAfterSelection()
	}

	point, ok := m.transcriptSelectionPointFromMouse(msg.Mouse())
	if !ok {
		return false
	}

	m.selection = transcriptSelection{
		active:   true,
		dragging: true,
		content:  m.transcript.GetContent(),
		start:    point,
		end:      point,
		mouse:    msg.Mouse(),
	}
	m.applyTranscriptSelectionStyle()

	return true
}

func (m *model) updateTranscriptSelection(msg tea.MouseMotionMsg) (bool, tea.Cmd) {
	if !m.selection.dragging {
		return false, nil
	}

	cmd := m.updateTranscriptSelectionForMouse(msg.Mouse())

	return true, cmd
}

func (m *model) finishTranscriptSelection(msg tea.MouseReleaseMsg) tea.Cmd {
	if !m.selection.dragging {
		return nil
	}

	m.updateTranscriptSelectionForMouse(msg.Mouse())
	m.selection.dragging = false
	m.selection.scroll = 0
	m.selection.ticking = false
	m.applyTranscriptSelectionStyle()

	text := m.selectedTranscriptText()
	if strings.TrimSpace(text) == "" {
		m.restoreTranscriptContentAfterSelection()
		return nil
	}
	if err := writeClipboard(text); err != nil {
		return m.setStatus("copy failed")
	}

	return nil
}

func (m *model) updateTranscriptSelectionForMouse(mouse tea.Mouse) tea.Cmd {
	m.selection.mouse = mouse
	m.selection.scroll = m.transcriptSelectionScrollDirection(mouse)

	if m.selection.scroll != 0 {
		m.scrollTranscriptSelection(m.selection.scroll)
	}
	if point, ok := m.transcriptSelectionPointFromMouseClamped(mouse); ok {
		m.selection.end = point
	}
	m.applyTranscriptSelectionStyle()

	if m.selection.scroll == 0 || m.selection.ticking {
		return nil
	}

	m.selection.ticking = true
	return transcriptSelectionAutoScrollTickCmd()
}

func (m *model) updateTranscriptSelectionAutoScroll() (tea.Model, tea.Cmd) {
	if !m.selection.dragging {
		m.selection.scroll = 0
		m.selection.ticking = false
		return *m, nil
	}

	m.selection.ticking = false
	cmd := m.updateTranscriptSelectionForMouse(m.selection.mouse)

	return *m, cmd
}

func (m *model) scrollTranscriptSelection(direction int) {
	previousOffset := m.transcript.YOffset()
	switch {
	case direction < 0:
		m.transcript.ScrollUp(1)
	case direction > 0:
		m.transcript.ScrollDown(1)
	}
	m.markResponseTranscriptScrolled(previousOffset, false)
}

func (m model) transcriptSelectionScrollDirection(mouse tea.Mouse) int {
	if m.transcript.Height() <= 0 {
		return 0
	}

	row := mouse.Y - m.getTranscriptTop()
	switch {
	case row <= 0 && m.transcript.YOffset() > 0:
		return -1
	case row >= m.transcript.Height()-1 && !m.transcript.AtBottom():
		return 1
	default:
		return 0
	}
}

func (m model) transcriptSelectionPointFromMouse(mouse tea.Mouse) (transcriptSelectionPoint, bool) {
	top := m.getTranscriptTop()
	row := mouse.Y - top
	if !m.isMouseInTranscript(mouse) {
		return transcriptSelectionPoint{}, false
	}

	x := max(mouse.X-getPanelHorizontalPadding(m.width), 0)

	return m.transcriptSelectionPointFromVisualLine(m.transcript.YOffset()+row, x)
}

func (m model) transcriptSelectionPointFromMouseClamped(mouse tea.Mouse) (transcriptSelectionPoint, bool) {
	if m.transcript.Height() <= 0 {
		return transcriptSelectionPoint{}, false
	}

	top := m.getTranscriptTop()
	row := min(max(mouse.Y-top, 0), m.transcript.Height()-1)
	x := max(mouse.X-getPanelHorizontalPadding(m.width), 0)

	return m.transcriptSelectionPointFromVisualLine(m.transcript.YOffset()+row, x)
}

func (m model) getTranscriptTop() int {
	return getTUILayout(m.width, m.height, m.input.Height()).Transcript.Y
}

func (m model) isMouseInTranscript(mouse tea.Mouse) bool {
	row := mouse.Y - m.getTranscriptTop()

	return row >= 0 && row < m.transcript.Height()
}

func (m model) transcriptSelectionPointFromVisualLine(
	visualLine int,
	x int,
) (transcriptSelectionPoint, bool) {
	if visualLine < 0 {
		return transcriptSelectionPoint{}, false
	}

	document := newRenderedTranscriptDocument(m.transcript.GetContent())
	lines := document.PlainLines()
	if !m.transcript.SoftWrap {
		if visualLine >= len(lines) {
			return transcriptSelectionPoint{}, false
		}

		return getTranscriptSelectionPointFromDocument(document, visualLine, x), true
	}

	width := max(
		m.transcript.Width()-m.transcript.Style.GetHorizontalFrameSize(),
		1,
	)
	offset := 0
	for index, line := range lines {
		height := getWrappedTranscriptLineHeight(line, width)
		if visualLine >= offset && visualLine < offset+height {
			wrappedColumn := (visualLine-offset)*width + max(min(x, width), 0)
			if hasTranscriptBodyIndent(line, getPanelHorizontalPadding(m.width)) {
				wrappedColumn += getPanelHorizontalPadding(m.width)
			}

			return getTranscriptSelectionPointFromDocument(document, index, wrappedColumn), true
		}

		offset += height
	}

	return transcriptSelectionPoint{}, false
}

func hasTranscriptBodyIndent(line string, padding int) bool {
	if padding <= 0 {
		return false
	}

	return strings.HasPrefix(line, strings.Repeat(" ", padding))
}

func getWrappedTranscriptLineHeight(line string, width int) int {
	width = max(width, 1)

	return max(1, int(math.Ceil(float64(ansi.StringWidth(line))/float64(width))))
}

func (m *model) applyTranscriptSelectionStyle() {
	offset := m.transcript.YOffset()
	if !m.selection.active {
		m.transcript.ClearHighlights()
		return
	}

	start, end := m.selection.offsetBounds()
	if start == end {
		m.transcript.ClearHighlights()

		return
	}

	style := lipgloss.NewStyle().Reverse(true)
	m.transcript.SetContent(highlightTranscriptSelection(m.getSelectionContent(), start, end, style))
	m.transcript.SetYOffset(offset)
}

func (m *model) clearTranscriptSelection() {
	m.selection = transcriptSelection{}
	m.transcript.ClearHighlights()
}

func transcriptSelectionAutoScrollTickCmd() tea.Cmd {
	return tea.Tick(transcriptSelectionAutoScrollInterval, func(time.Time) tea.Msg {
		return transcriptSelectionAutoScrollTickMsg{}
	})
}

func (m *model) restoreTranscriptContentAfterSelection() {
	offset := m.transcript.YOffset()
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	m.transcript.SetYOffset(offset)
}

func (m model) selectedTranscriptText() string {
	if !m.selection.active {
		return ""
	}

	document := newRenderedTranscriptDocument(m.getSelectionContent())
	start, end := m.selection.offsetBounds()
	text := document.PlainRange(start, end)
	if text == "" {
		return ""
	}
	text = removeTranscriptSelectionBodyIndent(text, getPanelHorizontalPadding(m.width))

	return compactTranscriptSelectionBlankLines(strings.TrimSpace(text))
}

func removeTranscriptSelectionBodyIndent(text string, padding int) string {
	if padding <= 0 || text == "" {
		return text
	}

	prefix := strings.Repeat(" ", padding)
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.TrimPrefix(lines[index], prefix)
	}

	return strings.Join(lines, "\n")
}

func (m model) getSelectionContent() string {
	if m.selection.content != "" {
		return m.selection.content
	}

	return m.transcript.GetContent()
}

func compactTranscriptSelectionBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	compact := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		if isTranscriptSelectionVisualPaddingLine(line) {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			compact = append(compact, "")
			continue
		}

		blank = false
		compact = append(compact, line)
	}

	return strings.Join(compact, "\n")
}

func isTranscriptSelectionVisualPaddingLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, char := range line {
		if char != '▀' && char != '▄' {
			return false
		}
	}

	return true
}

func (s transcriptSelection) offsetBounds() (int, int) {
	if s.start.offset <= s.end.offset {
		return s.start.offset, s.end.offset
	}

	return s.end.offset, s.start.offset
}

func getTranscriptSelectionPoint(
	lines []string,
	lineIndex int,
	column int,
	lineOffset int,
) transcriptSelectionPoint {
	if lineIndex < 0 || lineIndex >= len(lines) {
		return transcriptSelectionPoint{}
	}

	byteOffset := getByteOffsetForDisplayColumn(lines[lineIndex], column)

	return transcriptSelectionPoint{
		line:   lineIndex,
		offset: lineOffset + byteOffset,
	}
}

func getTranscriptSelectionPointFromDocument(
	document renderedTranscriptDocument,
	lineIndex int,
	column int,
) transcriptSelectionPoint {
	line, ok := document.Line(lineIndex)
	if !ok {
		return transcriptSelectionPoint{}
	}

	byteOffset := getByteOffsetForDisplayColumn(line.PlainText, column)

	return transcriptSelectionPoint{
		line:   lineIndex,
		offset: line.StartOffset + byteOffset,
	}
}

func highlightTranscriptSelection(
	content string,
	start int,
	end int,
	style lipgloss.Style,
) string {
	if start == end {
		return content
	}
	if start > end {
		start, end = end, start
	}

	lines := strings.Split(content, "\n")
	plainOffset := 0
	for index, line := range lines {
		plainLine := ansi.Strip(line)
		lineStart := plainOffset
		lineEnd := lineStart + len(plainLine)
		if start < lineEnd && end > lineStart {
			rangeStart := max(start-lineStart, 0)
			rangeEnd := min(end-lineStart, len(plainLine))
			if rangeStart < rangeEnd {
				styleStart := getDisplayColumnForByteOffset(plainLine, rangeStart)
				styleEnd := getDisplayColumnForByteOffset(plainLine, rangeEnd)
				lines[index] = lipgloss.StyleRanges(
					line,
					lipgloss.NewRange(styleStart, styleEnd, style),
				)
			}
		}

		plainOffset = lineEnd
		if index < len(lines)-1 {
			if start <= plainOffset && end > plainOffset {
				styleColumn := getDisplayColumnForByteOffset(plainLine, len(plainLine))
				lines[index] = lipgloss.StyleRanges(
					lines[index],
					lipgloss.NewRange(styleColumn, styleColumn+1, style),
				)
			}
			plainOffset++
		}
	}

	return strings.Join(lines, "\n")
}

func getByteOffsetForDisplayColumn(line string, column int) int {
	if column <= 0 {
		return 0
	}

	displayColumn := 0
	byteOffset := 0
	for byteOffset < len(line) {
		if nextOffset, ok := skipANSISequence(line, byteOffset); ok {
			byteOffset = nextOffset
			continue
		}

		graphemes := uniseg.NewGraphemes(line[byteOffset:])
		if !graphemes.Next() {
			break
		}
		text := graphemes.Str()
		width := max(1, graphemes.Width())
		nextColumn := displayColumn + width
		if column < nextColumn {
			return byteOffset
		}

		byteOffset += len(text)
		displayColumn = nextColumn
	}

	return len(line)
}

func getDisplayColumnForByteOffset(line string, targetOffset int) int {
	if targetOffset <= 0 {
		return 0
	}

	displayColumn := 0
	byteOffset := 0
	for byteOffset < len(line) && byteOffset < targetOffset {
		if nextOffset, ok := skipANSISequence(line, byteOffset); ok {
			byteOffset = nextOffset
			continue
		}

		graphemes := uniseg.NewGraphemes(line[byteOffset:])
		if !graphemes.Next() {
			break
		}
		text := graphemes.Str()
		nextOffset := byteOffset + len(text)
		if nextOffset > targetOffset {
			return displayColumn
		}

		displayColumn += max(1, graphemes.Width())
		byteOffset = nextOffset
	}

	return displayColumn
}

func skipANSISequence(value string, offset int) (int, bool) {
	if offset < 0 || offset >= len(value) || value[offset] != '\x1b' {
		return offset, false
	}
	if offset+1 >= len(value) {
		return len(value), true
	}

	switch value[offset+1] {
	case '[':
		for index := offset + 2; index < len(value); index++ {
			if value[index] >= 0x40 && value[index] <= 0x7e {
				return index + 1, true
			}
		}
	case ']':
		for index := offset + 2; index < len(value); index++ {
			if value[index] == '\a' {
				return index + 1, true
			}
			if value[index] == '\x1b' && index+1 < len(value) && value[index+1] == '\\' {
				return index + 2, true
			}
		}
	default:
		return offset + 2, true
	}

	return len(value), true
}

func getTranscriptLineOffset(lines []string, lineIndex int) int {
	offset := 0
	for index := range lines {
		if index >= lineIndex {
			return offset
		}

		offset += len(lines[index])
		if index < len(lines)-1 {
			offset++
		}
	}

	return offset
}

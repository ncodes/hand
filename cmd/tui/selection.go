package tui

import (
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type transcriptSelectionPoint struct {
	line   int
	offset int
}

type transcriptSelection struct {
	active   bool
	dragging bool
	start    transcriptSelectionPoint
	end      transcriptSelectionPoint
}

func (m *model) startTranscriptSelection(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft {
		return false
	}

	point, ok := m.transcriptSelectionPointFromMouse(msg.Mouse())
	if !ok {
		return false
	}

	m.selection = transcriptSelection{
		active:   true,
		dragging: true,
		start:    point,
		end:      point,
	}
	m.applyTranscriptSelectionStyle()

	return true
}

func (m *model) updateTranscriptSelection(msg tea.MouseMotionMsg) bool {
	if !m.selection.dragging {
		return false
	}

	point, ok := m.transcriptSelectionPointFromMouse(msg.Mouse())
	if !ok {
		return false
	}

	m.selection.end = point
	m.applyTranscriptSelectionStyle()

	return true
}

func (m *model) finishTranscriptSelection(msg tea.MouseReleaseMsg) tea.Cmd {
	if !m.selection.dragging {
		return nil
	}

	if point, ok := m.transcriptSelectionPointFromMouse(msg.Mouse()); ok {
		m.selection.end = point
	}
	m.selection.dragging = false
	m.applyTranscriptSelectionStyle()

	text := m.selectedTranscriptText()
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := writeClipboard(text); err != nil {
		return m.setStatus("copy failed")
	}

	return m.setStatus("selection copied")
}

func (m model) transcriptSelectionPointFromMouse(mouse tea.Mouse) (transcriptSelectionPoint, bool) {
	top := m.getTranscriptTop()
	row := mouse.Y - top
	if row < 0 || row >= m.transcript.Height() {
		return transcriptSelectionPoint{}, false
	}

	return m.transcriptSelectionPointFromVisualLine(
		m.transcript.YOffset()+row,
		mouse.X,
	)
}

func (m model) getTranscriptTop() int {
	return 0
}

func (m model) transcriptSelectionPointFromVisualLine(
	visualLine int,
	x int,
) (transcriptSelectionPoint, bool) {
	if visualLine < 0 {
		return transcriptSelectionPoint{}, false
	}

	content := m.transcript.GetContent()
	lines := strings.Split(content, "\n")
	if !m.transcript.SoftWrap {
		if visualLine >= len(lines) {
			return transcriptSelectionPoint{}, false
		}

		return getTranscriptSelectionPoint(
			lines,
			visualLine,
			x,
			getTranscriptLineOffset(lines, visualLine),
		), true
	}

	width := max(
		m.transcript.Width()-m.transcript.Style.GetHorizontalFrameSize(),
		1,
	)
	offset := 0
	lineOffset := 0
	for index, line := range lines {
		height := max(1, int(math.Ceil(float64(ansi.StringWidth(line))/float64(width))))
		if visualLine >= offset && visualLine < offset+height {
			wrappedColumn := (visualLine-offset)*width + max(min(x, width), 0)

			return getTranscriptSelectionPoint(lines, index, wrappedColumn, lineOffset), true
		}
		offset += height
		lineOffset += len(line)
		if index < len(lines)-1 {
			lineOffset++
		}
	}

	return transcriptSelectionPoint{}, false
}

func (m *model) applyTranscriptSelectionStyle() {
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
	m.transcript.HighlightStyle = style
	m.transcript.SelectedHighlightStyle = style
	m.transcript.SetHighlights([][]int{{start, end}})
}

func (m *model) clearTranscriptSelection() {
	m.selection = transcriptSelection{}
	m.applyTranscriptSelectionStyle()
}

func (m model) selectedTranscriptText() string {
	if !m.selection.active {
		return ""
	}

	content := m.transcript.GetContent()
	start, end := m.selection.offsetBounds()
	if start == end || start >= len(content) {
		return ""
	}
	if end > len(content) {
		end = len(content)
	}

	return strings.TrimSpace(ansi.Strip(content[start:end]))
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

func getByteOffsetForDisplayColumn(line string, column int) int {
	if column <= 0 {
		return 0
	}

	displayColumn := 0
	byteOffset := 0
	graphemes := uniseg.NewGraphemes(line)
	for graphemes.Next() {
		width := max(1, graphemes.Width())
		nextColumn := displayColumn + width
		if column < nextColumn {
			return byteOffset
		}

		text := graphemes.Str()
		byteOffset += len(text)
		displayColumn = nextColumn
	}

	return len(line)
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

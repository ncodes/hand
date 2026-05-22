package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/trace"
)

const (
	sidebarHorizontalPadding = 0
	sidebarHeaderLeftPadding = 2
	sidebarHeaderTopBlock    = "▄"
	sidebarBodyPadding       = 1
	sidebarHeaderHeight      = 3
)

type sidebarPlanModel struct {
	Steps        []trace.PlanStepPayload
	Summary      trace.PlanSummaryPayload
	ActiveStepID string
	Explanation  string
}

type planSidebarUpdatedMsg struct {
	Plan    sidebarPlanModel
	Cleared bool
}

func (m *model) updateSidebarPlan(msg planSidebarUpdatedMsg) {
	if msg.Cleared {
		m.sidebarPlan = sidebarPlanModel{}
		return
	}

	m.sidebarPlan = cloneSidebarPlan(msg.Plan)
}

func cloneSidebarPlan(plan sidebarPlanModel) sidebarPlanModel {
	return sidebarPlanModel{
		Steps:        append([]trace.PlanStepPayload(nil), plan.Steps...),
		Summary:      plan.Summary,
		ActiveStepID: strings.TrimSpace(plan.ActiveStepID),
		Explanation:  strings.TrimSpace(plan.Explanation),
	}
}

func renderRightSidebar(width int, height int, plan sidebarPlanModel) string {
	width = max(width, 1)
	height = max(height, 1)

	content := renderSidebarContent(width, height, plan)

	return lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Width(width).
		Height(height).
		Render(content)
}

func renderSidebarContent(width int, height int, plan sidebarPlanModel) string {
	innerWidth := max(width-sidebarHorizontalPadding*2, 1)
	header := renderSidebarHeader(width, innerWidth)
	body := renderSidebarChecklist(plan, width, innerWidth, max(height-sidebarHeaderHeight-1, 1))

	return strings.TrimRight(header+"\n"+body, "\n")
}

func renderSidebarHeader(width int, innerWidth int) string {
	title := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render("[*] Checklist")
	divider := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeBorder)).
		Render(strings.Repeat("─", max(innerWidth, 1)))

	lines := make([]string, 0, sidebarHeaderHeight)
	lines = append(lines, renderSidebarHeaderTopSpacer(width))
	lines = append(lines, renderSidebarPaddedLineWithLeftPadding(title, width, innerWidth, sidebarHeaderLeftPadding))
	lines = append(lines, renderSidebarPaddedLine(divider, width, innerWidth))

	return strings.Join(lines, "\n")
}

func renderSidebarHeaderTopSpacer(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Render(strings.Repeat(sidebarHeaderTopBlock, max(width, 1)))
}

func renderSidebarChecklist(plan sidebarPlanModel, width int, innerWidth int, maxHeight int) string {
	if len(plan.Steps) == 0 {
		content := lipgloss.NewStyle().
			Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
			Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
			Render("No active plan")

		return renderSidebarBodyLine(content, width, innerWidth)
	}

	lines := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		row := renderSidebarChecklistRow(step, width, innerWidth)
		if strings.TrimSpace(row) == "" {
			continue
		}
		lines = append(lines, strings.Split(row, "\n")...)
		if maxHeight > 0 && len(lines) >= maxHeight {
			return strings.Join(lines[:maxHeight], "\n")
		}
	}

	return strings.Join(lines, "\n")
}

func renderSidebarChecklistRow(step trace.PlanStepPayload, width int, innerWidth int) string {
	content := strings.TrimSpace(step.Content)
	if content == "" {
		content = strings.TrimSpace(step.ID)
	}
	if content == "" {
		return ""
	}

	icon := renderSidebarPlanStatusIcon(step.Status)
	iconWidth := 1
	textWidth := max(innerWidth-(sidebarBodyPadding*2)-iconWidth-1, 1)
	text := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Foreground(lipgloss.Color(getSidebarPlanTextColor(step.Status))).
		Width(textWidth).
		Render(content)

	lines := strings.Split(text, "\n")
	for index := range lines {
		if index == 0 {
			lines[index] = renderSidebarPaddedLineWithLeftPadding(
				icon+renderSidebarSpaces(1)+lines[index],
				width,
				innerWidth,
				sidebarBodyPadding,
			)
			continue
		}
		lines[index] = renderSidebarPaddedLineWithLeftPadding(
			renderSidebarSpaces(iconWidth+1)+lines[index],
			width,
			innerWidth,
			sidebarBodyPadding,
		)
	}

	return strings.Join(lines, "\n")
}

func renderSidebarPlanStatusIcon(status string) string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Foreground(lipgloss.Color(getSidebarPlanIconColor(status)))
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "completed":
		return style.Render("●")
	case "in_progress":
		return style.Render("●")
	case "cancelled":
		return style.Render("×")
	default:
		return style.Render("○")
	}
}

func renderSidebarPaddedLine(content string, width int, innerWidth int) string {
	return renderSidebarPaddedLineWithLeftPadding(content, width, innerWidth, 0)
}

func renderSidebarBodyLine(content string, width int, innerWidth int) string {
	contentWidth := lipgloss.Width(content) + sidebarBodyPadding
	rightPadding := max(innerWidth-contentWidth-sidebarBodyPadding, 0)

	return renderSidebarSpaces(sidebarBodyPadding) +
		content +
		renderSidebarSpaces(rightPadding+sidebarBodyPadding)
}

func renderSidebarPaddedLineWithLeftPadding(content string, width int, innerWidth int, leftPadding int) string {
	leftPadding = min(max(leftPadding, 0), max(innerWidth-1, 0))
	contentWidth := lipgloss.Width(content) + leftPadding
	rightPadding := max(innerWidth-contentWidth, 0)

	return renderSidebarSpaces(sidebarHorizontalPadding) +
		renderSidebarSpaces(leftPadding) +
		content +
		renderSidebarSpaces(rightPadding+sidebarHorizontalPadding)
}

func renderSidebarSpaces(width int) string {
	if width <= 0 {
		return ""
	}

	return lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.SidebarBackground)).
		Render(strings.Repeat(" ", width))
}

func getSidebarPlanIconColor(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "completed":
		return defaultTUITheme.ToolCompletedDot
	case "in_progress":
		return defaultTUITheme.ToolRunningDot
	case "cancelled":
		return defaultTUITheme.MutedText
	default:
		return defaultTUITheme.ToolBranch
	}
}

func getSidebarPlanTextColor(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "cancelled":
		return defaultTUITheme.MutedText
	default:
		return defaultTUITheme.ToolDetail
	}
}

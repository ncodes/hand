package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/trace"
)

type toolTranscriptRenderer struct{}

var defaultToolTranscriptRenderer = toolTranscriptRenderer{}

func (toolTranscriptRenderer) RenderGroup(
	group toolTranscriptGroup,
	ctx transcriptRenderContext,
) string {
	return renderToolTranscriptGroupContent(group, ctx)
}

func renderToolTranscriptGroupContent(group toolTranscriptGroup, ctx transcriptRenderContext) string {
	action := strings.TrimSpace(group.action)
	if action == "" {
		action = "Tool"
	}
	if action == "Run" {
		return renderRunTranscriptGroup(group, ctx)
	}
	completed := group.isCompleted()

	headerTitle := getToolTranscriptTitle(action, completed, group.details)
	headerDuration := ""
	if len(group.details) == 1 {
		headerDuration = renderToolTranscriptDuration(group.details[0], ctx.Now)
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(getToolTranscriptDotColor(completed))).
		Bold(true).
		Render(getToolTranscriptDot(completed, ctx.Frame)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.ToolTitle)).
			Render(" "+headerTitle) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.ToolDetail)).
			Render(headerDuration)

	details := make([]toolTranscriptDetail, 0, len(group.details))
	if shouldRenderToolTranscriptBranches(action) {
		for _, detail := range group.details {
			if strings.TrimSpace(detail.text) == "" && detail.planState == nil {
				continue
			}
			if shouldSkipToolTranscriptBranch(action, completed, detail) {
				continue
			}
			if strings.TrimSpace(action) == "Plan" && getPlanToolBranchDetail(detail.planState, detail.completed) == "" {
				continue
			}
			details = append(details, detail)
		}
	}
	if len(details) == 0 {
		return header
	}

	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolBranch))
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolDetail))
	lines := []string{header}
	for index, detail := range details {
		branch := "├"
		if index == len(details)-1 {
			branch = "└"
		}
		detailText := getToolTranscriptBranchDisplayDetail(group.action, detail)
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+renderToolBranchDetail(detailText, renderToolTranscriptDuration(detail, ctx.Now), detailStyle))
	}

	return strings.Join(lines, "\n")
}

func shouldRenderToolTranscriptBranches(action string) bool {
	switch strings.TrimSpace(action) {
	case "Session Messages", "Session Search", "Web Extract":
		return false
	default:
		return true
	}
}

func shouldSkipToolTranscriptBranch(action string, completed bool, detail toolTranscriptDetail) bool {
	if strings.TrimSpace(action) != "Plan" || !completed || detail.planState == nil {
		return false
	}

	return isGenericPlanInputState(detail.planState)
}

func isGenericPlanInputState(state *trace.PlanToolState) bool {
	if state == nil {
		return false
	}
	if len(state.Changes) > 0 || state.TotalCount > 0 || state.CompletedCount > 0 {
		return false
	}

	switch state.Operation {
	case trace.PlanToolOperationUpdate, trace.PlanToolOperationClearCompleted:
		return state.ChangedCount > 0
	default:
		return false
	}
}

func renderToolBranchDetail(detail string, duration string, style lipgloss.Style) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return style.Render(duration)
	}

	parts := strings.Fields(detail)
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		switch {
		case isToolDiffAdditionToken(part):
			rendered = append(rendered, lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolAddition)).Render(part))
		case isToolDiffRemovalToken(part):
			rendered = append(rendered, lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolDeletion)).Render(part))
		default:
			rendered = append(rendered, style.Render(part))
		}
	}

	return strings.Join(rendered, style.Render(" ")) + style.Render(duration)
}

func isToolDiffAdditionToken(value string) bool {
	return isToolSignedNumberToken(value, '+')
}

func isToolDiffRemovalToken(value string) bool {
	return isToolSignedNumberToken(value, '-')
}

func isToolSignedNumberToken(value string, sign byte) bool {
	if len(value) < 2 || value[0] != sign {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func renderRunTranscriptGroup(group toolTranscriptGroup, ctx transcriptRenderContext) string {
	count := len(group.details)
	if count == 0 {
		count = 1
	}

	noun := "shell command"
	if count != 1 {
		noun = "shell commands"
	}
	verb := "Running"
	suffix := "…"
	completed := group.isCompleted()
	if completed {
		verb = "Ran"
		suffix = ""
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(getToolTranscriptDotColor(completed))).
		Bold(true).
		Render(getToolTranscriptDot(completed, ctx.Frame)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.ToolTitle)).
			Render(" "+verb+" ") +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.UserTranscriptText)).
			Bold(true).
			Render(fmt.Sprintf("%d", count)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.ToolTitle)).
			Render(" "+noun+suffix)

	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolDetail))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolBranch))
	lines := []string{header}
	for index, detail := range group.details {
		branch := "├"
		if index == len(group.details)-1 {
			branch = "└"
		}
		detailText := getToolTranscriptBranchDisplayDetail(group.action, detail)
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+detailStyle.Render("$ "+detailText+renderToolTranscriptDuration(detail, ctx.Now)))
	}

	return strings.Join(lines, "\n")
}

func renderToolTranscriptDuration(detail toolTranscriptDetail, now time.Time) string {
	duration := getToolTranscriptDuration(detail, now)
	if duration <= 0 {
		return ""
	}

	return " (" + formatToolTranscriptDuration(duration) + ")"
}

func getToolTranscriptDuration(detail toolTranscriptDetail, now time.Time) time.Duration {
	if detail.startedAt.IsZero() {
		return 0
	}
	end := detail.completedAt
	if end.IsZero() {
		end = now
	}
	if end.IsZero() {
		end = currentTime()
	}
	if end.Before(detail.startedAt) {
		return 0
	}

	return end.Sub(detail.startedAt).Round(time.Second)
}

func formatToolTranscriptDuration(duration time.Duration) string {
	seconds := int(duration.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	return fmt.Sprintf("%ds", seconds)
}

func getToolTranscriptDotColor(completed bool) string {
	if completed {
		return defaultTUITheme.ToolCompletedDot
	}

	return defaultTUITheme.ToolRunningDot
}

func getToolTranscriptDot(completed bool, frame int) string {
	if completed {
		return "●"
	}

	frames := []string{"●", "◖", "◐", "◗", "●", "◔"}
	index := frame % len(frames)
	if index < 0 {
		index += len(frames)
	}

	return frames[index]
}

func getToolTranscriptTitle(action string, completed bool, details []toolTranscriptDetail) string {
	switch strings.TrimSpace(action) {
	case "Plan":
		return getPlanToolTranscriptTitle(getPlanToolTranscriptOperation(details), completed)
	case "Memory Search":
		if completed {
			return "Searched Memory"
		}

		return "Searching Memory"
	case "Memory Extract":
		if completed {
			return "Extracted Memory"
		}

		return "Extracting Memory"
	case "Memory Add":
		if completed {
			return "Added Memory"
		}

		return "Adding Memory"
	case "Memory Update":
		if completed {
			return "Updated Memory"
		}

		return "Updating Memory"
	case "Memory Delete":
		if completed {
			return "Deleted Memory"
		}

		return "Deleting Memory"
	case "Search Files":
		if completed {
			return "Searched Files"
		}

		return "Searching Files"
	case "Session Messages":
		if completed {
			return "Fetched Session Messages"
		}

		return "Fetching Session Messages"
	case "Session Search":
		if completed {
			return "Searched Session"
		}

		return "Searching Session"
	case "Web Extract":
		if completed {
			return "Extraction finished"
		}

		return "Extracting from web"
	}

	if !completed {
		return action
	}

	switch strings.TrimSpace(action) {
	case "Run":
		return "Ran"
	case "Write":
		return "Wrote"
	case "Web Search":
		return "Searched"
	case "Read":
		return "Read"
	case "Patch":
		return "Patch"
	default:
		return strings.TrimSpace(action)
	}
}

func getPlanToolTranscriptTitle(operation string, completed bool) string {
	switch operation {
	case "read":
		if completed {
			return "Plan read"
		}

		return "Reading plan"
	case "clear_completed":
		if completed {
			return "Plan cleared"
		}

		return "Clearing completed plan steps"
	default:
		if completed {
			return "Plan updated"
		}

		return "Updating plan"
	}
}

func getPlanToolTranscriptOperation(details []toolTranscriptDetail) string {
	for _, detail := range details {
		if detail.planState == nil {
			continue
		}
		switch detail.planState.Operation {
		case trace.PlanToolOperationRead:
			return "read"
		case trace.PlanToolOperationClearCompleted:
			return "clear_completed"
		case trace.PlanToolOperationUpdate:
			return "update"
		}
	}

	return "update"
}

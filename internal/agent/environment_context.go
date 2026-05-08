package agent

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
)

var (
	environmentContextNow   = time.Now
	environmentContextGetwd = os.Getwd
)

func (t *Turn) buildEnvironmentContextInstruction(activeToolDefinitions []models.ToolDefinition) instruct.Instruction {
	if t == nil {
		return instruct.Instruction{Name: instruct.EnvironmentContextInstructionName}
	}

	now := environmentContextNow()
	workingDirectory, _ := environmentContextGetwd()

	ctx := instruct.EnvironmentContext{
		Now:              now,
		Timezone:         getEnvironmentTimezone(now),
		OS:               runtime.GOOS,
		Architecture:     runtime.GOARCH,
		WorkingDirectory: workingDirectory,
		SessionID:        t.sessionID,
	}

	if t.cfg != nil {
		ctx.Platform = t.cfg.Platform
		ctx.FilesystemRoots = getFilesystemRoots(t.cfg.FS.Roots, workingDirectory)
		ctx.Model = t.cfg.Models.Main.Name
		if summaryModel := t.cfg.SummaryModelEffective(); summaryModel != "" && summaryModel != t.cfg.Models.Main.Name {
			ctx.SummaryModel = summaryModel
		}
		ctx.ModelProvider = t.cfg.Models.Main.Provider
		if summaryProvider := t.cfg.SummaryProviderEffective(); summaryProvider != "" &&
			summaryProvider != t.cfg.Models.Main.Provider {
			ctx.SummaryProvider = summaryProvider
		}
		ctx.APIMode = t.cfg.Models.Main.APIMode
		ctx.WebProvider = t.cfg.Web.Provider
	}

	if t.env != nil {
		policy := t.env.ToolPolicy()
		ctx.Platform = getFirstNonEmpty(ctx.Platform, policy.Platform)
		ctx.Capabilities = instruct.EnvironmentCapabilities{
			Filesystem: policy.Capabilities.Filesystem,
			Network:    policy.Capabilities.Network,
			Exec:       policy.Capabilities.Exec,
			Memory:     policy.Capabilities.Memory,
			Browser:    policy.Capabilities.Browser,
		}
		ctx.HasCapabilities = true

		if len(activeToolDefinitions) > 0 && t.env.Tools() != nil {
			ctx.ActiveToolGroups = getActiveToolGroups(t.env.Tools().ListGroups())
		}
	}

	ctx.ActiveTools = getActiveToolNames(activeToolDefinitions)

	return instruct.BuildEnvironmentContext(ctx)
}

func getEnvironmentTimezone(now time.Time) string {
	if now.IsZero() || now.Location() == nil {
		return ""
	}

	location := now.Location().String()
	if location != "" && location != "Local" {
		return location
	}

	name, offset := now.Zone()
	if name == "" {
		return location
	}

	return strings.TrimSpace(
		location + " (" + name + ", UTC" + getTimezoneOffset(offset) + ")",
	)
}

func getTimezoneOffset(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return sign + fmt.Sprintf("%02d:%02d", offset/3600, (offset%3600)/60)
}

func getFilesystemRoots(configured []string, workingDirectory string) []string {
	roots := configured
	if len(roots) == 0 && strings.TrimSpace(workingDirectory) != "" {
		roots = []string{workingDirectory}
	}
	return guardrails.NormalizeRoots(roots)
}

func getActiveToolNames(definitions []models.ToolDefinition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return sortedUnique(names)
}

func getActiveToolGroups(groups []tools.Group) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return sortedUnique(names)
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	sort.Strings(cleaned)
	return cleaned
}

func getFirstNonEmpty(first, second string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return strings.TrimSpace(second)
}

package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
)

func TestTurn_RequestInstructionsWithTools_AppendsEnvironmentContextBeforeExtras(t *testing.T) {
	originalNow := environmentContextNow
	originalGetwd := environmentContextGetwd
	t.Cleanup(func() {
		environmentContextNow = originalNow
		environmentContextGetwd = originalGetwd
	})

	location := time.FixedZone("WAT", 3600)
	environmentContextNow = func() time.Time {
		return time.Date(2026, 4, 11, 17, 30, 0, 0, location)
	}
	environmentContextGetwd = func() (string, error) {
		return "/workspace/hand", nil
	}

	turn := &Turn{
		cfg: &config.Config{
			Platform:        "cli",
			FSRoots:         []string{"/workspace/hand"},
			Model:           "openai/gpt-5.1",
			SummaryModel:    "openai/gpt-4o-mini",
			ModelProvider:   "openrouter",
			SummaryProvider: "openai",
			ModelAPIMode:    "responses",
			WebProvider:     "exa",
			MaxIterations:   3,
			ContextLength:   128000,
			StorageBackend:  "memory",
		},
		env: &mocks.EnvironmentStub{
			Policy: tools.Policy{
				Platform: "cli",
				Capabilities: tools.Capabilities{
					Filesystem: true,
					Network:    true,
					Exec:       true,
					Memory:     true,
				},
			},
			ToolRegistry: &mocks.ToolRegistryStub{
				Groups: []tools.Group{{Name: "core"}},
			},
		},
		sessionID:    "ses_123",
		instructions: instructions.New("base"),
		memory:       &memory.Memory{},
	}

	rendered := turn.buildRequestInstructions(
		[]models.ToolDefinition{{Name: "time"}, {Name: "read_file"}},
		instructions.New("extra"),
	)

	require.Contains(t, rendered, "# Environment Context")
	require.Contains(t, rendered, "- Current date: 2026-04-11")
	require.Contains(t, rendered, "- Current time: 2026-04-11T17:30:00+01:00")
	require.Contains(t, rendered, "- OS:")
	require.Contains(t, rendered, "- Working directory: /workspace/hand")
	require.Contains(t, rendered, "- Filesystem roots: /workspace/hand")
	require.Contains(t, rendered, "- Capabilities: filesystem=true, network=true, exec=true, memory=true, browser=false")
	require.Contains(t, rendered, "- Active tool groups: core")
	require.Contains(t, rendered, "- Active tools: read_file, time")
	require.Contains(t, rendered, "- Model: openai/gpt-5.1")
	require.Contains(t, rendered, "- Summary model: openai/gpt-4o-mini")
	require.Contains(t, rendered, "- Model provider: openrouter")
	require.Contains(t, rendered, "- Summary model provider: openai")
	require.Contains(t, rendered, "- API mode: responses")
	require.Contains(t, rendered, "- Web provider: exa")
	require.Contains(t, rendered, "- Session ID: ses_123")
	require.True(t, strings.Index(rendered, "base") < strings.Index(rendered, "# Environment Context"))
	require.True(t, strings.Index(rendered, "# Environment Context") < strings.Index(rendered, "extra"))
}

func TestTurn_RequestInstructionsWithTools_OmitsMatchingSummaryModelAndProvider(t *testing.T) {
	originalNow := environmentContextNow
	originalGetwd := environmentContextGetwd
	t.Cleanup(func() {
		environmentContextNow = originalNow
		environmentContextGetwd = originalGetwd
	})

	location := time.FixedZone("WAT", 3600)
	environmentContextNow = func() time.Time {
		return time.Date(2026, 4, 11, 17, 30, 0, 0, location)
	}
	environmentContextGetwd = func() (string, error) {
		return "/workspace/hand", nil
	}

	turn := &Turn{
		cfg: &config.Config{
			Platform:        "cli",
			FSRoots:         []string{"/workspace/hand"},
			Model:           "openai/gpt-5.1",
			SummaryModel:    "openai/gpt-5.1",
			ModelProvider:   "openrouter",
			SummaryProvider: "openrouter",
			WebProvider:     "exa",
		},
		env: &mocks.EnvironmentStub{
			Policy: tools.Policy{
				Platform: "cli",
				Capabilities: tools.Capabilities{
					Filesystem: true,
				},
			},
			ToolRegistry: tools.NewInMemoryRegistry(),
		},
		instructions: instructions.New("base"),
		memory:       &memory.Memory{},
	}

	rendered := turn.buildRequestInstructions(nil)

	require.Contains(t, rendered, "# Environment Context")
	require.Contains(t, rendered, "- Model: openai/gpt-5.1")
	require.Contains(t, rendered, "- Model provider: openrouter")
	require.Contains(t, rendered, "- Web provider: exa")
	require.NotContains(t, rendered, "- Summary model:")
	require.NotContains(t, rendered, "- Summary model provider:")
}

func TestTurn_BuildEnvironmentContextInstructionHandlesNilTurn(t *testing.T) {
	var turn *Turn
	instruction := turn.buildEnvironmentContextInstruction(nil)
	require.Equal(t, instructions.EnvironmentContextInstructionName, instruction.Name)
	require.Empty(t, instruction.Value)
}

func TestEnvironmentTimezone(t *testing.T) {
	lagos, err := time.LoadLocation("Africa/Lagos")
	require.NoError(t, err)

	require.Empty(t, environmentTimezone(time.Time{}))
	require.Empty(t, environmentTimezone(time.Date(2026, 4, 11, 17, 30, 0, 0, time.FixedZone("", -5*60*60))))
	require.Equal(t, "Africa/Lagos", environmentTimezone(time.Date(2026, 4, 11, 17, 30, 0, 0, lagos)))

	localTime := time.Date(2026, 4, 11, 17, 30, 0, 0, time.Local)
	name, offset := localTime.Zone()
	if name != "" {
		require.Equal(t, "Local ("+name+", UTC"+timezoneOffset(offset)+")", environmentTimezone(localTime))
	}
}

func TestTimezoneOffset(t *testing.T) {
	require.Equal(t, "+01:00", timezoneOffset(60*60))
	require.Equal(t, "-05:30", timezoneOffset(-5*60*60-30*60))
}

func TestSortedUnique(t *testing.T) {
	require.Equal(t, []string{"alpha", "beta"}, sortedUnique([]string{" beta ", "", "alpha", "beta"}))
}

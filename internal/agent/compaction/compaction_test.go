package compaction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func TestEstimateTextRough_ReturnsZeroWhenEmpty(t *testing.T) {
	require.Zero(t, EstimateTextRough(""))
}

func TestEstimateRequestRough_IncludesInstructionsMessagesAndTools(t *testing.T) {
	req := models.Request{
		Instructions: "follow the instructions",
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "hello world",
		}},
		Tools: []models.ToolDefinition{{
			Name:        "search_files",
			Description: "Search files",
			InputSchema: map[string]any{"type": "object"},
		}},
	}

	estimate := EstimateRequestRough(req)
	require.Positive(t, estimate)
	require.Equal(t, estimate, EstimateRequestRough(req))
}

func TestEvaluator_UsesActualPromptTokensWhenAvailable(t *testing.T) {
	evaluator := NewEvaluator(100, 0.5, 0.9)

	estimate := evaluator.Evaluate(models.Request{Instructions: "hello"}, 77)
	require.Equal(t, ActualSource, estimate.Source)
	require.Equal(t, 77, estimate.PromptTokens)
	require.Equal(t, 50, estimate.TriggerThreshold)
	require.Equal(t, 90, estimate.WarnThreshold)
	require.True(t, estimate.Triggered())
	require.False(t, estimate.Warning())
}

func TestEvaluator_FallsBackToEstimatedPromptTokens(t *testing.T) {
	evaluator := NewEvaluator(100, 0.5, 0.6)
	req := models.Request{
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "abcdefgh",
		}},
	}

	estimate := evaluator.Evaluate(req, 0)
	require.Equal(t, EstimatedSource, estimate.Source)
	require.Equal(t, EstimateRequestRough(req), estimate.PromptTokens)
}

func TestEvaluator_UsesEstimatedPromptTokensWhenCurrentRequestExceedsStoredActual(t *testing.T) {
	evaluator := NewEvaluator(1000, 0.5, 0.6)
	req := models.Request{
		Instructions: strings.Repeat("a", 600),
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: strings.Repeat("b", 600),
		}},
	}

	estimate := evaluator.Evaluate(req, 50)
	require.Equal(t, EstimatedSource, estimate.Source)
	require.Equal(t, EstimateRequestRough(req), estimate.PromptTokens)
	require.Greater(t, estimate.PromptTokens, 50)
}

func TestEvaluator_ReportsWarningAtBoundary(t *testing.T) {
	evaluator := NewEvaluator(100, 0.5, 0.7)

	estimate := evaluator.Evaluate(models.Request{}, 70)
	require.True(t, estimate.Warning())
	require.True(t, estimate.Triggered())
}

func TestNewEvaluator_DefaultsInvalidInputs(t *testing.T) {
	evaluator := NewEvaluator(0, 0, 0)

	estimate := evaluator.Evaluate(models.Request{}, 0)
	require.Equal(t, 128000, estimate.ContextLimit)
	require.Equal(t, 108800, estimate.TriggerThreshold)
	require.Equal(t, 121600, estimate.WarnThreshold)
}

func TestEstimateRequestRough_FallsBackToInstructionsWhenJSONMarshalFails(t *testing.T) {
	req := models.Request{
		Instructions: "hello",
		Tools: []models.ToolDefinition{{
			Name:        "bad_tool",
			Description: "contains unsupported schema value",
			InputSchema: map[string]any{"broken": func() {}},
		}},
	}

	require.Equal(t, EstimateTextRough("hello"), EstimateRequestRough(req))
}

func TestEvaluator_NilReceiverUsesDefaults(t *testing.T) {
	var evaluator *Evaluator

	estimate := evaluator.Evaluate(models.Request{Instructions: "hello"}, 0)
	require.Equal(t, EstimatedSource, estimate.Source)
	require.Equal(t, EstimateRequestRough(models.Request{Instructions: "hello"}), estimate.PromptTokens)
	require.Equal(t, 128000, estimate.ContextLimit)
}

func TestEstimate_WarningAndTriggeredRequirePositiveThresholds(t *testing.T) {
	estimate := Estimate{PromptTokens: 10}
	require.False(t, estimate.Warning())
	require.False(t, estimate.Triggered())
}

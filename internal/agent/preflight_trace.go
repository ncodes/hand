package agent

import (
	"github.com/wandxy/hand/internal/agent/compaction"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/trace"
)

func compactionEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.CompactionEnabled == nil {
		return true
	}

	return *cfg.CompactionEnabled
}

func compactionEvaluator(cfg *config.Config) *compaction.Evaluator {
	if cfg == nil {
		return compaction.NewEvaluator(0, 0, 0)
	}

	return compaction.NewEvaluator(
		cfg.ContextLength,
		cfg.CompactionTriggerPercent,
		cfg.CompactionWarnPercent,
	)
}

func recordPreflightCompactionTrace(
	traceSession trace.Session,
	cfg *config.Config,
	request models.Request,
	lastPromptTokens int,
) {
	if !compactionEnabled(cfg) {
		return
	}

	estimate := compactionEvaluator(cfg).Evaluate(request, lastPromptTokens)
	payload := map[string]any{
		"source":            estimate.Source,
		"prompt_tokens":     estimate.PromptTokens,
		"context_limit":     estimate.ContextLimit,
		"trigger_threshold": estimate.TriggerThreshold,
		"warn_threshold":    estimate.WarnThreshold,
	}

	traceSession.Record("context.preflight", payload)

	if estimate.Triggered() {
		traceSession.Record("context.compaction.triggered", payload)
	}

	if estimate.Warning() {
		traceSession.Record("context.compaction.warning", payload)
	}
}

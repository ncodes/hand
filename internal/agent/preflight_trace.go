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

	traceSession.Record(trace.EvtContextPreflight, payload)

	if estimate.Triggered() {
		agentLog.Info().
			Str("source", estimate.Source).
			Int("prompt_tokens", estimate.PromptTokens).
			Int("trigger_threshold", estimate.TriggerThreshold).
			Int("context_limit", estimate.ContextLimit).
			Msg("preflight context estimate exceeded compaction threshold")
		traceSession.Record(trace.EvtContextCompactionTriggered, payload)
	}

	if estimate.Warning() && !estimate.Triggered() {
		agentLog.Warn().
			Int("prompt_tokens", estimate.PromptTokens).
			Int("warn_threshold", estimate.WarnThreshold).
			Int("context_limit", estimate.ContextLimit).
			Msg("context approaching compaction threshold")
	}
	if estimate.Warning() {
		traceSession.Record(trace.EvtContextCompactionWarning, payload)
	}
}

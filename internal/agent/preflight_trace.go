package agent

import (
	"github.com/wandxy/hand/internal/agent/context/compaction"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/trace"
)

func isCompactionEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.Compaction.Enabled == nil {
		return true
	}

	return *cfg.Compaction.Enabled
}

func getCompactionEvaluator(cfg *config.Config) *compaction.Evaluator {
	if cfg == nil {
		return compaction.NewEvaluator(0, 0, 0)
	}

	return compaction.NewEvaluator(
		cfg.Models.Main.ContextLength,
		cfg.Compaction.TriggerPercent,
		cfg.Compaction.WarnPercent,
	)
}

func recordPreflightCompactionTrace(
	traceSession trace.Session,
	cfg *config.Config,
	request models.Request,
	lastPromptTokens int,
	canCompact bool,
) {
	if !isCompactionEnabled(cfg) {
		return
	}

	estimate := getCompactionEvaluator(cfg).Evaluate(request, lastPromptTokens)
	payload := trace.ContextEventPayload{
		Source:           estimate.Source,
		PromptTokens:     estimate.PromptTokens,
		ContextLimit:     estimate.ContextLimit,
		TriggerThreshold: estimate.TriggerThreshold,
		WarnThreshold:    estimate.WarnThreshold,
	}

	traceSession.Record(trace.EvtContextPreflight, payload)

	if estimate.Triggered() && canCompact {
		agentLog.Info().
			Str("source", estimate.Source).
			Int("prompt_tokens", estimate.PromptTokens).
			Int("trigger_threshold", estimate.TriggerThreshold).
			Int("context_limit", estimate.ContextLimit).
			Msg("preflight context estimate exceeded compaction threshold")
		traceSession.Record(trace.EvtContextCompactionTriggered, payload)
	}

	if estimate.Warning() && (!estimate.Triggered() || !canCompact) {
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

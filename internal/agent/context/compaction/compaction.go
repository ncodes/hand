package compaction

import (
	"encoding/json"

	"github.com/wandxy/hand/internal/constants"
	models "github.com/wandxy/hand/pkg/agent/model"
)

const (
	ActualSource    = "actual"
	EstimatedSource = "estimated"
)

// Estimate is a context budget reading used for warning and compaction decisions.
type Estimate struct {
	Source           string
	PromptTokens     int
	ContextLimit     int
	TriggerThreshold int
	WarnThreshold    int
}

// Triggered reports whether the estimate has crossed the compaction threshold.
func (e Estimate) Triggered() bool {
	return e.PromptTokens >= e.TriggerThreshold && e.TriggerThreshold > 0
}

// Warning reports whether the estimate has crossed the warning threshold.
func (e Estimate) Warning() bool {
	return e.PromptTokens >= e.WarnThreshold && e.WarnThreshold > 0
}

// Evaluator compares prompt token estimates against configured context thresholds.
type Evaluator struct {
	contextLimit     int
	triggerThreshold int
	warnThreshold    int
}

// NewEvaluator builds an Evaluator with defaults for unset limits or percentages.
func NewEvaluator(contextLimit int, triggerPercent, warnPercent float64) *Evaluator {
	if contextLimit <= 0 {
		contextLimit = constants.DefaultContextLength
	}
	if triggerPercent <= 0 {
		triggerPercent = constants.DefaultCompactionTrigger
	}
	if warnPercent <= 0 {
		warnPercent = constants.DefaultCompactionWarn
	}

	return &Evaluator{
		contextLimit:     contextLimit,
		triggerThreshold: int(float64(contextLimit) * triggerPercent),
		warnThreshold:    int(float64(contextLimit) * warnPercent),
	}
}

// EstimateTextRough estimates tokens from text length using a cheap character ratio.
func EstimateTextRough(text string) int {
	if text == "" {
		return 0
	}

	return len(text) / constants.RoughTokenCharRatio
}

// EstimateCharsFromTokensRough converts a token estimate back to a rough character budget.
func EstimateCharsFromTokensRough(tokens int) int {
	if tokens <= 0 {
		return 0
	}

	return tokens * constants.RoughTokenCharRatio
}

// EstimateRequestRough estimates prompt tokens from instructions, messages, and tool schema.
func EstimateRequestRough(req models.Request) int {
	payload := struct {
		Instructions string
		Messages     any
		Tools        any
	}{
		Instructions: req.Instructions,
		Messages:     req.Messages,
		Tools:        req.Tools,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return EstimateTextRough(req.Instructions)
	}

	return EstimateTextRough(string(raw))
}

// Evaluate chooses actual prompt tokens when trustworthy, otherwise a rough request estimate.
func (e *Evaluator) Evaluate(req models.Request, lastActualPromptTokens int) Estimate {
	if e == nil {
		e = NewEvaluator(0, 0, 0)
	}

	estimatedPromptTokens := EstimateRequestRough(req)
	estimate := Estimate{
		ContextLimit:     e.contextLimit,
		TriggerThreshold: e.triggerThreshold,
		WarnThreshold:    e.warnThreshold,
	}

	// Actual provider usage is more authoritative, but only use it when it is at
	// least as large as the local estimate. After compaction, callers clear stale
	// actual counts so pre-compaction usage does not keep forcing warnings.
	if lastActualPromptTokens > 0 && lastActualPromptTokens >= estimatedPromptTokens {
		estimate.Source = ActualSource
		estimate.PromptTokens = lastActualPromptTokens
		return estimate
	}

	estimate.Source = EstimatedSource
	estimate.PromptTokens = estimatedPromptTokens
	return estimate
}

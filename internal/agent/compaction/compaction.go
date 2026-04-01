package compaction

import (
	"encoding/json"

	"github.com/wandxy/hand/internal/models"
)

const (
	ActualSource    = "actual"
	EstimatedSource = "estimated"
)

type Estimate struct {
	Source           string
	PromptTokens     int
	ContextLimit     int
	TriggerThreshold int
	WarnThreshold    int
}

func (e Estimate) Triggered() bool {
	return e.PromptTokens >= e.TriggerThreshold && e.TriggerThreshold > 0
}

func (e Estimate) Warning() bool {
	return e.PromptTokens >= e.WarnThreshold && e.WarnThreshold > 0
}

type Evaluator struct {
	contextLimit     int
	triggerThreshold int
	warnThreshold    int
}

func NewEvaluator(contextLimit int, triggerPercent, warnPercent float64) *Evaluator {
	if contextLimit <= 0 {
		contextLimit = 128000
	}
	if triggerPercent <= 0 {
		triggerPercent = 0.85
	}
	if warnPercent <= 0 {
		warnPercent = 0.95
	}

	return &Evaluator{
		contextLimit:     contextLimit,
		triggerThreshold: int(float64(contextLimit) * triggerPercent),
		warnThreshold:    int(float64(contextLimit) * warnPercent),
	}
}

func EstimateTextRough(text string) int {
	if text == "" {
		return 0
	}

	return len(text) / 4
}

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

	if lastActualPromptTokens > 0 && lastActualPromptTokens >= estimatedPromptTokens {
		estimate.Source = ActualSource
		estimate.PromptTokens = lastActualPromptTokens
		return estimate
	}

	estimate.Source = EstimatedSource
	estimate.PromptTokens = estimatedPromptTokens
	return estimate
}

package agent

import (
	"context"
	"strings"

	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	memoryobservability "github.com/wandxy/hand/internal/memory/observability"
	"github.com/wandxy/hand/internal/trace"
)

const (
	memoryRetrievalLimit            = 5
	memoryRetrievalItemChars        = 800
	memoryContextInstructionMaxChar = 4000
)

var sanitizeMemoryPromptValue = guardrails.Sanitize

func (t *Turn) retrieveMemoryInstruction(
	ctx context.Context,
	userText string,
	traceSession trace.Session,
) instruct.Instruction {
	if t == nil || t.cfg == nil || !t.cfg.MemoryEnabled() || t.env == nil {
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	provider := t.env.MemoryProvider()
	if provider == nil {
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	searchProvider, ok := provider.(memory.SearchProvider)
	if !ok {
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	if err := provider.ConfigureObservability(memoryobservability.New(agentLog, traceSession)); err != nil {
		recordMemoryRetrievalFailed(traceSession, provider.Name(), "configure_observability", err)
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	caps, err := provider.Capabilities(ctx)
	if err != nil {
		recordMemoryRetrievalFailed(traceSession, provider.Name(), "capabilities", err)
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}
	if !caps.SupportsSearch {
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrievalStarted, map[string]any{
		"provider":  provider.Name(),
		"operation": "search",
		"limit":     memoryRetrievalLimit,
		"max_chars": memoryRetrievalItemChars,
	})

	query := memory.SearchQuery{
		Text:     strings.TrimSpace(userText),
		Statuses: []memory.Status{memory.StatusActive, memory.StatusCandidate},
		Limit:    memoryRetrievalLimit,
		MaxChars: memoryRetrievalItemChars,
	}
	result, err := searchProvider.Search(ctx, query)
	if err != nil {
		recordMemoryRetrievalFailed(traceSession, provider.Name(), "search", err)
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	items := sanitizeMemoryHitsForPrompt(result.Hits)

	recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrieved, map[string]any{
		"provider":       provider.Name(),
		"hit_count":      len(result.Hits),
		"injected_count": len(items),
	})

	return instruct.BuildMemoryContext(
		memoryContextItems(items),
		memoryContextInstructionMaxChar,
	)
}

func recordMemoryRetrievalEvent(traceSession trace.Session, event string, payload map[string]any) {
	if traceSession == nil {
		return
	}
	traceSession.Record(event, payload)
}

func recordMemoryRetrievalFailed(
	traceSession trace.Session,
	providerName string,
	operation string,
	err error,
) {
	if traceSession != nil {
		traceSession.Record(trace.EvtMemoryRetrievalFailed, map[string]any{
			"provider":  strings.TrimSpace(providerName),
			"operation": strings.TrimSpace(operation),
			"error":     err.Error(),
		})
	}
	agentLog.Warn().
		Str("event", "memory retrieval failed").
		Str("provider", strings.TrimSpace(providerName)).
		Str("operation", strings.TrimSpace(operation)).
		Err(err).
		Msg("memory retrieval failed")
}

func sanitizeMemoryHitsForPrompt(hits []memory.SearchHit) []memory.MemoryItem {
	items := make([]memory.MemoryItem, 0, len(hits))
	for _, hit := range hits {
		item, ok := sanitizeMemoryItemForPrompt(hit.Item)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items
}

func sanitizeMemoryItemForPrompt(item memory.MemoryItem) (memory.MemoryItem, bool) {
	if item.Status == memory.StatusDeleted || item.Status == memory.StatusSuperseded {
		return memory.MemoryItem{}, false
	}

	item.Title = memoryPromptText(item.Title)
	item.Text = memoryPromptText(item.Text)
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Text) == "" {
		return memory.MemoryItem{}, false
	}

	scanned := guardrails.SafetyScan(
		strings.Join([]string{item.Title, item.Text}, "\n"),
		item.GuardrailSource(),
	)
	if scanned.Blocked {
		return memory.MemoryItem{}, false
	}

	return item, true
}

func memoryPromptText(value string) string {
	sanitized, ok := sanitizeMemoryPromptValue(value).(string)
	if !ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(sanitized)
}

func memoryContextItems(items []memory.MemoryItem) []instruct.MemoryContextItem {
	contextItems := make([]instruct.MemoryContextItem, 0, len(items))
	for _, item := range items {
		contextItems = append(contextItems, instruct.MemoryContextItem{
			Kind:  string(item.Kind),
			Title: item.Title,
			Text:  item.Text,
		})
	}
	return contextItems
}

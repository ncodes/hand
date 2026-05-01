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
	pinnedMemoryRetrievalLimit      = 3
	pinnedMemoryRetrievalItemChars  = 1000
	searchMemoryRetrievalLimit      = 5
	searchMemoryRetrievalItemChars  = 700
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

	if err := provider.ConfigureObservability(memoryobservability.New(agentLog, traceSession)); err != nil {
		recordMemoryRetrievalFailed(traceSession, provider.Name(), "configure_observability", err)
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	caps, err := provider.Capabilities(ctx)
	if err != nil {
		recordMemoryRetrievalFailed(traceSession, provider.Name(), "capabilities", err)
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}
	searchProvider, supportsSearchProvider := provider.(memory.SearchProvider)
	pinnedProvider, supportsPinnedProvider := provider.(memory.PinnedProvider)
	if (!caps.SupportsSearch || !supportsSearchProvider) && (!caps.SupportsPinned || !supportsPinnedProvider) {
		return instruct.Instruction{Name: instruct.MemoryContextInstructionName}
	}

	items := make([]memory.MemoryItem, 0, pinnedMemoryRetrievalLimit+searchMemoryRetrievalLimit)

	if caps.SupportsPinned && supportsPinnedProvider {
		recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrievalStarted, map[string]any{
			"provider":  provider.Name(),
			"operation": "load_pinned",
			"limit":     pinnedMemoryRetrievalLimit,
			"max_chars": pinnedMemoryRetrievalItemChars,
		})

		pinned, err := pinnedProvider.LoadPinned(ctx, memory.SearchQuery{
			Statuses: []memory.Status{memory.StatusActive},
			Limit:    pinnedMemoryRetrievalLimit,
			MaxChars: pinnedMemoryRetrievalItemChars,
		})
		if err != nil {
			recordMemoryRetrievalFailed(traceSession, provider.Name(), "load_pinned", err)
		} else {
			items = append(items, pinned...)
		}
	}

	if caps.SupportsSearch && supportsSearchProvider {
		recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrievalStarted, map[string]any{
			"provider":  provider.Name(),
			"operation": "search",
			"limit":     searchMemoryRetrievalLimit,
			"max_chars": searchMemoryRetrievalItemChars,
		})

		query := memory.SearchQuery{
			Text: strings.TrimSpace(userText),
			Kinds: []memory.Kind{
				memory.KindSemantic,
				memory.KindEpisodic,
				memory.KindProcedural,
			},
			Statuses: []memory.Status{memory.StatusActive},
			Limit:    searchMemoryRetrievalLimit,
			MaxChars: searchMemoryRetrievalItemChars,
		}
		result, err := searchProvider.Search(ctx, query)
		if err != nil {
			recordMemoryRetrievalFailed(traceSession, provider.Name(), "search", err)
		} else {
			items = append(items, toMemoryHitItems(result.Hits)...)
		}
	}

	hitCount := len(items)
	items = sanitizeMemoryItemsForPrompt(items)

	recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrieved, map[string]any{
		"provider":       provider.Name(),
		"hit_count":      hitCount,
		"injected_count": len(items),
	})

	return instruct.BuildMemoryContext(
		toMemoryContextItems(items),
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

func toMemoryHitItems(hits []memory.SearchHit) []memory.MemoryItem {
	items := make([]memory.MemoryItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, hit.Item)
	}
	return items
}

func sanitizeMemoryItemsForPrompt(input []memory.MemoryItem) []memory.MemoryItem {
	items := make([]memory.MemoryItem, 0, len(input))
	for _, item := range input {
		sanitized, ok := sanitizeMemoryItemForPrompt(item)
		if !ok {
			continue
		}
		items = append(items, sanitized)
	}
	return items
}

func sanitizeMemoryItemForPrompt(item memory.MemoryItem) (memory.MemoryItem, bool) {
	if item.Status != memory.StatusActive {
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

func toMemoryContextItems(items []memory.MemoryItem) []instruct.MemoryContextItem {
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

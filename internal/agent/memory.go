package agent

import (
	"context"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	memoryobservability "github.com/wandxy/hand/internal/memory/observability"
	"github.com/wandxy/hand/internal/trace"
)

const (
	pinnedMemoryRetrievalLimit      = constants.AgentPinnedMemoryRetrievalLimit
	pinnedMemoryRetrievalItemChars  = constants.AgentPinnedMemoryRetrievalItemChars
	searchMemoryRetrievalLimit      = constants.AgentSearchMemoryRetrievalLimit
	searchMemoryRetrievalItemChars  = constants.AgentSearchMemoryRetrievalItemChars
	searchMemoryRetrievalMinScore   = constants.AgentSearchMemoryRetrievalMinScore
	memoryContextInstructionMaxChar = constants.AgentMemoryContextInstructionChars
)

var sanitizeMemoryPromptValue = guardrails.Sanitize

func (t *Turn) retrieveMemoryInstruction(
	ctx context.Context,
	userText string,
	traceSession trace.Session,
) instruct.Instruction {
	if t == nil || t.cfg == nil || !t.cfg.MemoryEnabled() || !t.cfg.MemoryRetrievalEnabled() || t.env == nil {
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
	pinnedTraceItems := []map[string]any(nil)
	searchTraceHits := []map[string]any(nil)
	retrievedHitCount := 0
	filteredSearchHitCount := 0

	if caps.SupportsPinned && supportsPinnedProvider {
		recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrievalStarted, map[string]any{
			"provider":  provider.Name(),
			"operation": "load_pinned",
			"limit":     pinnedMemoryRetrievalLimit,
			"max_chars": pinnedMemoryRetrievalItemChars,
		})

		pinned, err := pinnedProvider.LoadPinned(ctx, memory.SearchQuery{
			RerankerUseCase: memory.RerankerUseCasePinned,
			Statuses:        []memory.Status{memory.StatusActive},
			Limit:           pinnedMemoryRetrievalLimit,
			MaxChars:        pinnedMemoryRetrievalItemChars,
		})
		if err != nil {
			recordMemoryRetrievalFailed(traceSession, provider.Name(), "load_pinned", err)
		} else {
			pinnedTraceItems = memoryRetrievalTraceItems(pinned)
			retrievedHitCount += len(pinned)
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
			Text:            strings.TrimSpace(userText),
			RerankerUseCase: memory.RerankerUseCaseTurnRetrieval,
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
			filteredHits := filterSearchHitsForTurnMemory(result.Hits)
			filteredSearchHitCount = len(result.Hits) - len(filteredHits)
			retrievedHitCount += len(result.Hits)
			searchTraceHits = memoryRetrievalTraceHits(result.Hits)
			items = append(items, searchHitsToMemoryItems(filteredHits)...)
		}
	}

	items = sanitizeMemoryItemsForPrompt(items, traceSession)

	recordMemoryRetrievalEvent(traceSession, trace.EvtMemoryRetrieved, map[string]any{
		"provider":              provider.Name(),
		"hit_count":             retrievedHitCount,
		"injected_count":        len(items),
		"pinned_items":          pinnedTraceItems,
		"search_hits":           searchTraceHits,
		"search_min_score":      searchMemoryRetrievalMinScore,
		"search_filtered_count": filteredSearchHitCount,
		"injected_items":        memoryRetrievalTraceItems(items),
	})

	return instruct.BuildMemoryContext(
		memoryItemsToContextItems(items),
		memoryContextInstructionMaxChar,
	)
}

func filterSearchHitsForTurnMemory(hits []memory.SearchHit) []memory.SearchHit {
	filtered := make([]memory.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Score < searchMemoryRetrievalMinScore {
			continue
		}
		filtered = append(filtered, hit)
	}
	return filtered
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

func searchHitsToMemoryItems(hits []memory.SearchHit) []memory.MemoryItem {
	items := make([]memory.MemoryItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, hit.Item)
	}
	return items
}

func memoryRetrievalTraceHits(hits []memory.SearchHit) []map[string]any {
	items := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		item := memoryRetrievalTraceItem(hit.Item)
		item["score"] = hit.Score
		item["lexical_score"] = hit.LexicalScore
		item["vector_score"] = hit.VectorScore
		items = append(items, item)
	}
	return items
}

func memoryRetrievalTraceItems(memoryItems []memory.MemoryItem) []map[string]any {
	items := make([]map[string]any, 0, len(memoryItems))
	for _, item := range memoryItems {
		items = append(items, memoryRetrievalTraceItem(item))
	}
	return items
}

func memoryRetrievalTraceItem(item memory.MemoryItem) map[string]any {
	return map[string]any{
		"id":           strings.TrimSpace(item.ID),
		"kind":         string(item.Kind),
		"status":       string(item.Status),
		"title":        truncateMemoryTraceText(item.Title),
		"text_chars":   len([]rune(strings.TrimSpace(item.Text))),
		"confidence":   item.Confidence,
		"reflected":    item.Reflected,
		"source_count": len(item.SourceLinks),
	}
}

func truncateMemoryTraceText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 120 {
		return value
	}
	return string(runes[:120])
}

func sanitizeMemoryItemsForPrompt(input []memory.MemoryItem, traceSession trace.Session) []memory.MemoryItem {
	items := make([]memory.MemoryItem, 0, len(input))
	for _, item := range input {
		sanitized, ok := sanitizeMemoryItemForPromptWithTrace(item, traceSession)
		if !ok {
			continue
		}
		items = append(items, sanitized)
	}
	return items
}

func sanitizeMemoryItemForPrompt(item memory.MemoryItem) (memory.MemoryItem, bool) {
	return sanitizeMemoryItemForPromptWithTrace(item, nil)
}

func sanitizeMemoryItemForPromptWithTrace(item memory.MemoryItem, traceSession trace.Session) (memory.MemoryItem, bool) {
	if item.Status != memory.StatusActive {
		return memory.MemoryItem{}, false
	}

	item.Title = getMemoryPromptText(item.Title)
	item.Text = getMemoryPromptText(item.Text)
	content := strings.Join([]string{item.Title, item.Text}, "\n")
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Text) == "" {
		return memory.MemoryItem{}, false
	}

	scanned := guardrails.SafetyScan(
		content,
		item.GuardrailSource(),
	)
	if scanned.Blocked {
		recordMemorySafetyBlocked(traceSession, item.GuardrailSource(), content, scanned.Findings)
		return memory.MemoryItem{}, false
	}

	return item, true
}

func recordMemorySafetyBlocked(
	traceSession trace.Session,
	source string,
	content string,
	findings []guardrails.SafetyFinding,
) {
	if traceSession == nil {
		return
	}

	traceSession.Record(trace.EvtMemorySafetyBlocked, guardrails.SafetyTracePayload(guardrails.SafetyTracePayloadOptions{
		Source:        source,
		Action:        "blocked",
		ContentLength: len([]rune(content)),
		Blocked:       true,
		Findings:      findings,
	}))
}

func getMemoryPromptText(value string) string {
	sanitized, ok := sanitizeMemoryPromptValue(value).(string)
	if !ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(sanitized)
}

func memoryItemsToContextItems(items []memory.MemoryItem) []instruct.MemoryContextItem {
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

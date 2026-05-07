package memory

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/trace"
)

const (
	defaultReflectionLimit        = 10
	maxReflectionLimit            = 50
	defaultReflectionRelatedLimit = 3
	maxReflectionRelatedLimit     = 10

	defaultReflectionBackgroundInterval = time.Minute

	reflectionSimilarScoreThreshold = 0.75
)

func (p *MemoryProvider) Reflect(ctx context.Context, req ReflectionRequest) (ReflectionResult, error) {
	started := time.Now().UTC()
	if p == nil || p.manager == nil {
		return ReflectionResult{}, errors.New("memory provider is required")
	}
	if p.reflectionGenerator == nil {
		return ReflectionResult{}, errors.New("memory reflection is not configured")
	}

	normalized, err := p.normalizeReflectionRequest(ctx, req)
	if err != nil {
		p.recordReflectionFailure(ctx, ReflectionResult{}, err)
		return ReflectionResult{}, err
	}

	fields := observationFields(p.Name(), "reflect", map[string]any{
		"session_id":    normalized.SessionID,
		"limit":         normalized.Limit,
		"related_limit": normalized.RelatedLimit,
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection started", trace.EvtMemoryReflectionStarted, fields)

	result := ReflectionResult{SessionID: normalized.SessionID}
	sources, err := p.loadReflectionSources(ctx, normalized)
	if err != nil {
		p.recordReflectionFailure(ctx, result, err)
		return ReflectionResult{}, err
	}
	result.SourceCount = len(sources)

	fields = observationFields(p.Name(), "reflect", map[string]any{
		"session_id":   normalized.SessionID,
		"source_count": result.SourceCount,
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection sources loaded", trace.EvtMemoryReflectionSourceLoaded, fields)

	if len(sources) == 0 {
		p.recordReflectionCompleted(ctx, result, started)
		return result, nil
	}

	related, err := p.loadReflectionRelated(ctx, sources, normalized)
	if err != nil {
		p.recordReflectionFailure(ctx, result, err)
		return ReflectionResult{}, err
	}
	result.RelatedCount = len(related)

	fields = observationFields(p.Name(), "reflect", map[string]any{
		"session_id":    normalized.SessionID,
		"related_count": result.RelatedCount,
		"bounded_limit": normalized.RelatedLimit,
		"source_count":  result.SourceCount,
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection related memories loaded", trace.EvtMemoryReflectionRelatedLoaded, fields)

	generated, err := p.reflectionGenerator.GenerateReflectionCandidates(ctx, ReflectionGenerationRequest{
		SessionID: normalized.SessionID,
		Sources:   cloneMemoryItems(sources),
		Related:   cloneMemoryItems(related),
		Limit:     normalized.Limit,
	})
	if err != nil {
		p.recordReflectionFailure(ctx, result, err)
		return ReflectionResult{}, err
	}

	sourceLinks := reflectionSourceLinks(sources)
	sourceIDs := memoryIDs(sources)
	written := make([]MemoryItem, 0, normalized.Limit)
	for _, candidate := range limitReflectionItems(generated.Items, normalized.Limit) {
		item, ok, rejection := prepareReflectionCandidate(
			candidate,
			normalized.SessionID,
			sourceLinks,
			sourceIDs,
		)
		if !ok {
			p.recordReflectionRejection(ctx, normalized.SessionID, rejection)
			continue
		}

		rejection, err = p.reflectionCandidateRejection(ctx, item, written)
		if err != nil {
			p.recordReflectionFailure(ctx, result, err)
			return ReflectionResult{}, err
		}
		if rejection != "" {
			p.recordReflectionRejection(ctx, normalized.SessionID, rejection)
			continue
		}

		fields = observationFields(p.Name(), "reflect", map[string]any{
			"session_id":  normalized.SessionID,
			"memory_id":   item.ID,
			"memory_kind": string(item.Kind),
			"confidence":  item.Confidence,
		})
		logDebugAndTrace(ctx, p.observability(), "memory reflection candidate generated", trace.EvtMemoryReflectionCandidateGenerated, fields)

		item, err = p.recordReflectionCandidate(ctx, item)
		if err != nil {
			p.recordReflectionFailure(ctx, result, err)
			return ReflectionResult{}, err
		}

		result.WriteCount++
		result.Items = append(result.Items, item.Clone())
		written = append(written, item.Clone())

		fields = observationFields(p.Name(), "reflect", map[string]any{
			"session_id":   normalized.SessionID,
			"memory_id":    item.ID,
			"memory_kind":  string(item.Kind),
			"write_status": string(item.Status),
		})
		logDebugAndTrace(ctx, p.observability(), "memory reflection candidate written", trace.EvtMemoryReflectionMemoryWritten, fields)
	}

	if err := p.markReflectionSourcesReflected(ctx, sources); err != nil {
		p.recordReflectionFailure(ctx, result, err)
		return ReflectionResult{}, err
	}

	p.recordReflectionCompleted(ctx, result, started)
	return result, nil
}

func normalizeReflectionBackgroundOptions(opts ReflectionBackgroundOptions) ReflectionBackgroundOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultReflectionBackgroundInterval
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultReflectionLimit
	}
	if opts.Limit > maxReflectionLimit {
		opts.Limit = maxReflectionLimit
	}
	if opts.RelatedLimit <= 0 {
		opts.RelatedLimit = defaultReflectionRelatedLimit
	}
	if opts.RelatedLimit > maxReflectionRelatedLimit {
		opts.RelatedLimit = maxReflectionRelatedLimit
	}

	return opts
}

func (p *MemoryProvider) startReflectionBackground(ctx context.Context) error {
	if !p.reflectionBackground.Enabled {
		return nil
	}
	if p.reflectionGenerator == nil {
		return errors.New("memory reflection is not configured")
	}

	opts := normalizeReflectionBackgroundOptions(p.reflectionBackground)
	p.reflectionBackgroundStartOnce.Do(func() {
		go p.runReflectionBackgroundLoop(ctx, opts)
	})

	return nil
}

func (p *MemoryProvider) runReflectionBackgroundLoop(ctx context.Context, opts ReflectionBackgroundOptions) {
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = p.RunReflectionBackground(ctx, opts)
		}
	}
}

func (p *MemoryProvider) RunReflectionBackground(
	ctx context.Context,
	opts ReflectionBackgroundOptions,
) (ReflectionResult, error) {
	if p == nil || p.manager == nil {
		return ReflectionResult{}, errors.New("memory provider is required")
	}

	opts = normalizeReflectionBackgroundOptions(opts)
	return p.Reflect(ctx, ReflectionRequest{
		Limit:        opts.Limit,
		RelatedLimit: opts.RelatedLimit,
	})
}

type normalizedReflectionRequest struct {
	SessionID    string
	Limit        int
	RelatedLimit int
}

func (p *MemoryProvider) normalizeReflectionRequest(ctx context.Context, req ReflectionRequest) (normalizedReflectionRequest, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		currentSessionID, err := p.manager.CurrentSession(ctx)
		if err != nil {
			return normalizedReflectionRequest{}, err
		}
		sessionID = strings.TrimSpace(currentSessionID)
	}
	if sessionID == "" {
		return normalizedReflectionRequest{}, errors.New("reflection session id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultReflectionLimit
	}
	if limit > maxReflectionLimit {
		limit = maxReflectionLimit
	}

	relatedLimit := req.RelatedLimit
	if relatedLimit <= 0 {
		relatedLimit = defaultReflectionRelatedLimit
	}
	if relatedLimit > maxReflectionRelatedLimit {
		relatedLimit = maxReflectionRelatedLimit
	}

	return normalizedReflectionRequest{SessionID: sessionID, Limit: limit, RelatedLimit: relatedLimit}, nil
}

func (p *MemoryProvider) loadReflectionSources(
	ctx context.Context,
	req normalizedReflectionRequest,
) ([]MemoryItem, error) {
	unreflected := false
	result, err := p.manager.SearchMemory(ctx, SearchQuery{
		SessionID: req.SessionID,
		Kinds:     []Kind{KindEpisodic},
		Statuses:  []Status{StatusCandidate, StatusActive},
		Limit:     req.Limit,
		Reflected: &unreflected,
	})
	if err != nil {
		return nil, err
	}

	items := make([]MemoryItem, 0, len(result.Hits))
	for _, hit := range result.Hits {
		items = append(items, hit.Item.Clone())
	}

	return items, nil
}

func (p *MemoryProvider) loadReflectionRelated(
	ctx context.Context,
	sources []MemoryItem,
	req normalizedReflectionRequest,
) ([]MemoryItem, error) {
	seen := make(map[string]struct{})
	items := make([]MemoryItem, 0, len(sources)*req.RelatedLimit)
	for _, source := range sources {
		text := reflectionSearchText(source)
		if text == "" {
			continue
		}

		result, err := p.manager.SearchMemory(ctx, SearchQuery{
			Text:     text,
			Kinds:    []Kind{KindPinned, KindSemantic, KindProcedural},
			Statuses: []Status{StatusCandidate, StatusActive},
			Limit:    req.RelatedLimit,
		})
		if err != nil {
			return nil, err
		}

		for _, hit := range result.Hits {
			id := strings.TrimSpace(hit.Item.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			items = append(items, hit.Item.Clone())
		}
	}

	return items, nil
}

func (p *MemoryProvider) reflectionCandidateRejection(
	ctx context.Context,
	item MemoryItem,
	written []MemoryItem,
) (string, error) {
	if reflectionMatchesExistingCandidate(item, written) {
		return "duplicate_reflection_candidate", nil
	}

	text := reflectionSearchText(item)
	if text == "" {
		return "", nil
	}

	reflected := true
	result, err := p.manager.SearchMemory(ctx, SearchQuery{
		Text:      text,
		Statuses:  []Status{StatusCandidate, StatusActive},
		Limit:     5,
		Reflected: &reflected,
	})
	if err != nil {
		return "", err
	}

	for _, hit := range result.Hits {
		related := hit.Item
		if strings.TrimSpace(related.ID) == strings.TrimSpace(item.ID) {
			continue
		}
		switch {
		case normalizedLifecycleText(related) == normalizedLifecycleText(item):
			return "duplicate_reflection_memory", nil
		case hit.Score >= reflectionSimilarScoreThreshold:
			return "similar_reflection_memory", nil
		}
	}

	return "", nil
}

func reflectionMatchesExistingCandidate(item MemoryItem, existing []MemoryItem) bool {
	normalized := normalizedLifecycleText(item)
	if normalized == "" {
		return false
	}

	for _, candidate := range existing {
		if normalizedLifecycleText(candidate) == normalized {
			return true
		}
	}

	return false
}

func reflectionSearchText(item MemoryItem) string {
	text := strings.TrimSpace(item.Title)
	if text == "" {
		text = strings.TrimSpace(item.Text)
	}
	if len([]rune(text)) > 240 {
		text = string([]rune(text)[:240])
	}
	return text
}

func prepareReflectionCandidate(
	item MemoryItem,
	sessionID string,
	sourceLinks []SourceLink,
	sourceIDs []string,
) (MemoryItem, bool, string) {
	item = item.Clone()

	item.ID = ""
	if item.Status == "" {
		item.Status = StatusCandidate
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		item.Metadata["source_session_id"] = sessionID
	}
	item.Metadata["reflection_source_memory_ids"] = strings.Join(sourceIDs, ",")
	item.Metadata["reflection_origin"] = "episodic"
	item.Reflected = true

	item.SourceLinks = cloneSourceLinks(sourceLinks)
	item.Tags = append(item.Tags, "reflection")
	for _, id := range sourceIDs {
		if tag := reflectionSourceTag(id); tag != "" {
			item.Tags = append(item.Tags, tag)
		}
	}
	item.Tags = normalizeMemoryTags(item.Tags)

	item.ID = generateKindAwareMemoryID(item.Kind)
	if err := validateReflectionCandidate(item); err != nil {
		return MemoryItem{}, false, err.Error()
	}

	return item, true, ""
}

func (p *MemoryProvider) recordReflectionCandidate(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}

	item, err := p.manager.UpsertMemory(ctx, item)
	if err != nil {
		return MemoryItem{}, err
	}

	return item.Clone(), nil
}

func (p *MemoryProvider) markReflectionSourcesReflected(ctx context.Context, sources []MemoryItem) error {
	for _, source := range sources {
		reflected := true
		if _, err := p.manager.PatchMemory(ctx, MemoryPatch{
			ID:        source.ID,
			Reflected: &reflected,
		}); err != nil {
			return err
		}
	}

	return nil
}

func validateReflectionCandidate(item MemoryItem) error {
	switch item.Kind {
	case KindPinned, KindSemantic, KindEpisodic, KindProcedural:
	default:
		return errors.New("reflection candidate kind must be pinned, semantic, episodic, or procedural")
	}
	if item.Status != StatusCandidate {
		return errors.New("reflection candidate must be stored as candidate")
	}
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Text) == "" {
		return errors.New("reflection candidate text or title is required")
	}
	if !hasCandidateProvenance(item) {
		return errors.New("reflection candidate source provenance is required")
	}
	if reason := candidateAdmissionRejectionReason(item); reason != "" {
		return errors.New(reason)
	}

	return nil
}

func reflectionSourceLinks(sources []MemoryItem) []SourceLink {
	links := make([]SourceLink, 0, len(sources))
	for _, source := range sources {
		for _, link := range source.SourceLinks {
			links = append(links, link)
		}
		if len(source.SourceLinks) == 0 {
			if sessionID := strings.TrimSpace(source.Metadata["source_session_id"]); sessionID != "" {
				links = append(links, SourceLink{
					SessionID:     sessionID,
					CreatedBy:     "reflection",
					CreatedReason: "reflection_source_memory",
				})
			}
		}
	}
	return cloneSourceLinks(links)
}

func limitReflectionItems(items []MemoryItem, limit int) []MemoryItem {
	if limit <= 0 || len(items) <= limit {
		return items
	}

	return items[:limit]
}

func cloneSourceLinks(links []SourceLink) []SourceLink {
	cloned := make([]SourceLink, 0, len(links))
	for _, link := range links {
		link.MessageIDs = append([]uint(nil), link.MessageIDs...)
		link.Offsets = append([]int(nil), link.Offsets...)
		cloned = append(cloned, link)
	}
	return cloned
}

func cloneMemoryItems(items []MemoryItem) []MemoryItem {
	cloned := make([]MemoryItem, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, item.Clone())
	}
	return cloned
}

func memoryIDs(items []MemoryItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func reflectionSourceTag(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "reflection-source-" + id
}

func (p *MemoryProvider) recordReflectionRejection(ctx context.Context, sessionID string, reason string) {
	fields := observationFields(p.Name(), "reflect", map[string]any{
		"session_id":        sessionID,
		"rejection_reason":  reason,
		"admission_outcome": "rejected",
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection candidate rejected", trace.EvtMemoryReflectionCandidateRejected, fields)
}

func (p *MemoryProvider) recordReflectionFailure(ctx context.Context, result ReflectionResult, err error) {
	fields := observationFields(p.Name(), "reflect", map[string]any{
		"session_id":    result.SessionID,
		"source_count":  result.SourceCount,
		"related_count": result.RelatedCount,
		"write_count":   result.WriteCount,
		"error":         err.Error(),
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection failed", trace.EvtMemoryReflectionFailed, fields)
}

func (p *MemoryProvider) recordReflectionCompleted(ctx context.Context, result ReflectionResult, started time.Time) {
	fields := observationFields(p.Name(), "reflect", map[string]any{
		"session_id":    result.SessionID,
		"source_count":  result.SourceCount,
		"related_count": result.RelatedCount,
		"write_count":   result.WriteCount,
		"duration_ms":   time.Since(started).Milliseconds(),
	})
	logDebugAndTrace(ctx, p.observability(), "memory reflection completed", trace.EvtMemoryReflectionCompleted, fields)
}

func normalizeMemoryTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	slices.Sort(normalized)
	return normalized
}

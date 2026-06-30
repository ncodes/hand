package memory

import (
	"context"
	"time"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/stringx"
)

func (p *MemoryProvider) recordPromotionStarted(ctx context.Context, memoryID string, operation string) {
	fields := buildObservationFields(p.Name(), "promote", map[string]any{
		"memory_id": memoryID,
		"action":    operation,
	})
	logDebugAndTrace(ctx, p.observability(), "memory promotion started for candidate activation", trace.EvtMemoryPromotionStarted, fields)
}

func (p *MemoryProvider) recordPromotionDecision(
	ctx context.Context,
	memoryID string,
	decision PromotionDecision,
	related []SearchHit,
) {
	fields := buildObservationFields(p.Name(), "promote", map[string]any{
		"memory_id":      memoryID,
		"approved":       decision.Approved,
		"policy":         decision.Policy,
		"reason":         decision.Reason,
		"confidence":     decision.Confidence,
		"conflict_state": decision.ConflictState,
	})
	if len(related) > 0 {
		fields["related_count"] = len(related)
		fields["related_memory_ids"] = getPromotionRelatedHitIDs(related)
		fields["related_top_score"] = getPromotionTopRelatedScore(related)
	}
	logDebugAndTrace(ctx, p.observability(), "memory promotion decision", trace.EvtMemoryPromotionDecision, fields)
}

func (p *MemoryProvider) recordPromotionCompleted(
	ctx context.Context,
	memoryID string,
	decision PromotionDecision,
	started time.Time,
) {
	fields := buildObservationFields(p.Name(), "promote", map[string]any{
		"memory_id":   memoryID,
		"approved":    decision.Approved,
		"reason":      decision.Reason,
		"duration_ms": time.Since(started).Milliseconds(),
	})
	logDebugAndTrace(ctx, p.observability(), "memory promotion completed for candidate activation", trace.EvtMemoryPromotionCompleted, fields)
}

func (p *MemoryProvider) recordPromotionFailure(ctx context.Context, memoryID string, err error) {
	fields := buildObservationFields(p.Name(), "promote", map[string]any{
		"memory_id": memoryID,
		"error":     err.Error(),
	})
	logDebugAndTrace(ctx, p.observability(), "memory promotion failed", trace.EvtMemoryPromotionFailed, fields)
}

func (p *MemoryProvider) recordPromotionFallback(ctx context.Context, memoryID string) {
	fields := buildObservationFields(p.Name(), "promote", map[string]any{
		"memory_id": memoryID,
		"fallback":  "default_policy",
	})
	logDebugAndTrace(ctx, p.observability(), "memory promotion fallback", trace.EvtMemoryPromotionFallback, fields)
}

func getPromotionRelatedHitIDs(hits []SearchHit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		if id := stringx.String(hit.Item.ID).Trim(); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func getPromotionTopRelatedScore(hits []SearchHit) float64 {
	var score float64
	for _, hit := range hits {
		if hit.Score > score {
			score = hit.Score
		}
	}
	return score
}

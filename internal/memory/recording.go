package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/pkg/nanoid"
)

func (p *MemoryProvider) RecordSemanticMemory(ctx context.Context, record SemanticRecord) (MemoryItem, error) {
	item := record.Item
	item.Kind = KindSemantic

	return p.recordMemoryCandidate(ctx, item)
}

func (p *MemoryProvider) RecordProceduralMemory(
	ctx context.Context,
	record ProceduralRecord,
) (MemoryItem, error) {
	item := record.Item
	item.Kind = KindProcedural

	return p.recordMemoryCandidate(ctx, item)
}

func (p *MemoryProvider) recordMemoryCandidate(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if p == nil || p.manager == nil {
		return MemoryItem{}, errors.New("memory provider is required")
	}

	item = prepareMemoryCandidate(item)
	if err := validateMemoryCandidate(item); err != nil {
		return MemoryItem{}, err
	}
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}

	item, err := p.manager.UpsertMemory(ctx, item)
	if err != nil {
		return MemoryItem{}, err
	}

	fields := observationFields(p.Name(), "record_candidate", map[string]any{"memory_id": item.ID})
	logDebugAndTrace(ctx, p.observability(), "memory candidate recorded", "memory.candidate_write.completed", fields)

	return item.Clone(), nil
}

func prepareMemoryCandidate(item MemoryItem) MemoryItem {
	item = item.Clone()
	item.ID = strings.TrimSpace(item.ID)
	if item.Status == "" {
		item.Status = StatusCandidate
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	if item.ID == "" {
		item.ID = generateKindAwareMemoryID(item.Kind)
	}

	return item
}

func validateMemoryCandidate(item MemoryItem) error {
	switch item.Kind {
	case KindSemantic, KindProcedural:
	default:
		return errors.New("memory candidate kind must be semantic or procedural")
	}
	if item.Status != StatusCandidate {
		return errors.New("memory candidate must be stored as candidate")
	}
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Text) == "" {
		return errors.New("memory candidate text or title is required")
	}
	if !hasCandidateProvenance(item) {
		return errors.New("memory candidate source provenance is required")
	}
	if reason := candidateAdmissionRejectionReason(item); reason != "" {
		return errors.New(reason)
	}

	return nil
}

func candidateAdmissionRejectionReason(item MemoryItem) string {
	switch strings.ToLower(strings.TrimSpace(item.Metadata["memory_importance"])) {
	case "low":
		return "low_importance_candidate"
	}
	switch strings.ToLower(strings.TrimSpace(item.Metadata["memory_granularity"])) {
	case "execution_detail":
		return "execution_detail"
	}

	return ""
}

func hasCandidateProvenance(item MemoryItem) bool {
	for _, link := range item.SourceLinks {
		if strings.TrimSpace(link.SessionID) != "" ||
			len(link.MessageIDs) > 0 ||
			len(link.Offsets) > 0 ||
			strings.TrimSpace(link.SummaryID) != "" {
			return true
		}
	}

	return strings.TrimSpace(item.Metadata["source_session_id"]) != ""
}

func generateKindAwareMemoryID(kind Kind) string {
	return kindAwareMemoryIDPrefix(kind) + strings.TrimPrefix(nanoid.MustGenerate("mem_"), "mem_")
}

func kindAwareMemoryIDPrefix(kind Kind) string {
	kindPart := strings.TrimSpace(string(kind))
	if kindPart == "" {
		kindPart = "unknown"
	}
	return "mem_" + kindPart + "_"
}

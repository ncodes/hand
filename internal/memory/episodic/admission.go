package episodic

import (
	"strings"

	storage "github.com/wandxy/hand/internal/state/core"
)

func admitCandidateItems(
	items []storage.MemoryItem,
	rejections []candidateRejection,
) ([]storage.MemoryItem, []candidateRejection) {
	admitted := make([]storage.MemoryItem, 0, len(items))
	for _, item := range items {
		if reason := candidateRejectionReason(item); reason != "" {
			rejections = append(rejections, candidateRejection{
				Kind:   candidateKind(item),
				Reason: reason,
			})
			continue
		}

		admitted = append(admitted, item)
	}

	return collapseCandidateGroups(admitted, rejections)
}

func candidateRejectionReason(item storage.MemoryItem) string {
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

func collapseCandidateGroups(
	items []storage.MemoryItem,
	rejections []candidateRejection,
) ([]storage.MemoryItem, []candidateRejection) {
	bestByGroup := make(map[string]int)
	for idx, item := range items {
		group := canonicalCandidateGroup(item)
		if group == "" {
			continue
		}
		bestIndex, ok := bestByGroup[group]
		if !ok || candidateAdmissionScore(item) > candidateAdmissionScore(items[bestIndex]) {
			bestByGroup[group] = idx
			continue
		}
	}

	admitted := make([]storage.MemoryItem, 0, len(items))
	for idx, item := range items {
		group := canonicalCandidateGroup(item)
		if group == "" || bestByGroup[group] == idx {
			admitted = append(admitted, item)
			continue
		}
		rejections = append(rejections, candidateRejection{
			Kind:   candidateKind(item),
			Reason: "redundant_candidate_group",
		})
	}

	return admitted, rejections
}

func canonicalCandidateGroup(item storage.MemoryItem) string {
	return normalizeMemoryIDText(item.Metadata["canonical_group"])
}

func candidateAdmissionScore(item storage.MemoryItem) int {
	score := candidateKindPriority(candidateKind(item)) * 100
	switch strings.ToLower(strings.TrimSpace(item.Metadata["memory_importance"])) {
	case "high":
		score += 30
	case "medium":
		score += 20
	}
	switch strings.ToLower(strings.TrimSpace(item.Metadata["memory_granularity"])) {
	case "summary":
		score += 10
	case "episode":
		score += 5
	}
	score += int(clampConfidence(item.Confidence) * 10)

	return score
}

func candidateKind(item storage.MemoryItem) string {
	return strings.TrimSpace(item.Metadata["candidate_kind"])
}

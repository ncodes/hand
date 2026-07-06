package episodic

import (
	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/str"
)

// admitCandidateItems applies deterministic admission after model extraction
// and before persistence. The model can propose candidates, but local code
// decides which proposals are too low-signal or redundant.
func admitCandidateItems(
	items []storage.MemoryItem,
	rejections []candidateRejection,
) ([]storage.MemoryItem, []candidateRejection) {
	admitted := make([]storage.MemoryItem, 0, len(items))
	for _, item := range items {
		if reason := checkEpisodeCandidateAdmissionRejection(item); reason != "" {
			rejections = append(rejections, candidateRejection{
				Kind:   getCandidateKind(item),
				Reason: reason,
			})
			continue
		}

		admitted = append(admitted, item)
	}

	return collapseCandidateGroups(admitted, rejections)
}

// checkEpisodeCandidateAdmissionRejection rejects metadata that explicitly marks a proposal as
// not worth durable memory. This mirrors the prompt guidance and gives a stable
// reason independent of model wording.
func checkEpisodeCandidateAdmissionRejection(item storage.MemoryItem) string {
	stringValue1 := str.String(item.Metadata["memory_importance"])
	switch stringValue1.Normalized() {
	case "low":
		return "low_importance_candidate"
	}
	stringValue2 := str.String(item.Metadata["memory_granularity"])
	switch stringValue2.Normalized() {
	case "execution_detail":
		return "execution_detail"
	}

	return ""
}

// collapseCandidateGroups keeps the strongest candidate per canonical group.
// This lets the model emit related alternatives while the service stores only
// the best representative for a repeated fact or outcome.
func collapseCandidateGroups(
	items []storage.MemoryItem,
	rejections []candidateRejection,
) ([]storage.MemoryItem, []candidateRejection) {
	bestByGroup := make(map[string]int)
	for idx, item := range items {
		group := getCanonicalCandidateGroup(item)
		if group == "" {
			continue
		}
		bestIndex, ok := bestByGroup[group]
		if !ok || getCandidateAdmissionScore(item) > getCandidateAdmissionScore(items[bestIndex]) {
			bestByGroup[group] = idx
			continue
		}
	}

	admitted := make([]storage.MemoryItem, 0, len(items))
	for idx, item := range items {
		group := getCanonicalCandidateGroup(item)
		if group == "" || bestByGroup[group] == idx {
			admitted = append(admitted, item)
			continue
		}
		rejections = append(rejections, candidateRejection{
			Kind:   getCandidateKind(item),
			Reason: "redundant_candidate_group",
		})
	}

	return admitted, rejections
}

func getCanonicalCandidateGroup(item storage.MemoryItem) string {
	return normalizeMemoryIDText(item.Metadata["canonical_group"])
}

// getCandidateAdmissionScore ranks candidates within the same canonical group.
// Higher-priority kinds, importance, summary-level granularity, and confidence
// all improve the chance a candidate survives group collapse.
func getCandidateAdmissionScore(item storage.MemoryItem) int {
	score := getCandidateKindPriority(getCandidateKind(item)) * 100
	stringValue3 := str.String(item.Metadata["memory_importance"])
	switch stringValue3.Normalized() {
	case "high":
		score += 30
	case "medium":
		score += 20
	}
	stringValue4 := str.String(item.Metadata["memory_granularity"])
	switch stringValue4.Normalized() {
	case "summary":
		score += 10
	case "episode":
		score += 5
	}
	score += int(clampConfidence(item.Confidence) * 10)

	return score
}

func getCandidateKind(item storage.MemoryItem) string {
	stringValue5 := str.String(item.Metadata["candidate_kind"])
	return stringValue5.Trim()
}

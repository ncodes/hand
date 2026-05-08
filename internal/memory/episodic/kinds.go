package episodic

const (
	episodeKindDecision       = "decision"
	episodeKindOutcome        = "outcome"
	episodeKindToolEvent      = "tool_event"
	episodeKindBlocker        = "blocker"
	episodeKindUserCorrection = "user_correction"
	episodeKindTaskTrace      = "task_trace"
	episodeKindResolvedIssue  = "resolved_issue"
	episodeKindMilestone      = "milestone"
	episodeKindDiscarded      = "discarded_approach"
	episodeKindReflection     = "reflection"
)

type episodeKindSpec struct {
	kind       string
	usefulness string
	priority   int
}

// episodeKindSpecs is both the model schema allowlist and the local ranking
// table used during admission. Higher priority kinds win when candidates share a
// canonical group.
var episodeKindSpecs = []episodeKindSpec{
	{kind: episodeKindOutcome, usefulness: "high", priority: 9},
	{kind: episodeKindMilestone, usefulness: "high", priority: 8},
	{kind: episodeKindDecision, usefulness: "high", priority: 7},
	{kind: episodeKindResolvedIssue, usefulness: "high", priority: 6},
	{kind: episodeKindBlocker, usefulness: "medium", priority: 5},
	{kind: episodeKindUserCorrection, usefulness: "high", priority: 4},
	{kind: episodeKindReflection, usefulness: "high", priority: 4},
	{kind: episodeKindDiscarded, usefulness: "high", priority: 3},
	{kind: episodeKindTaskTrace, usefulness: "medium", priority: 2},
	{kind: episodeKindToolEvent, usefulness: "medium", priority: 1},
}

// getEpisodeCandidateKinds returns the schema enum passed to the model.
func getEpisodeCandidateKinds() []string {
	kinds := make([]string, 0, len(episodeKindSpecs))
	for _, spec := range episodeKindSpecs {
		kinds = append(kinds, spec.kind)
	}

	return kinds
}

func isValidCandidateKind(kind string) bool {
	return getEpisodeKindSpecFor(kind).kind != ""
}

// usefulness is stored as metadata so later reflection/promotion steps can see
// why an extracted episode was considered worth remembering.
func getUsefulness(kind string) string {
	if spec := getEpisodeKindSpecFor(kind); spec.kind != "" {
		return spec.usefulness
	}

	return "low"
}

func getCandidateKindPriority(kind string) int {
	return getEpisodeKindSpecFor(kind).priority
}

func getEpisodeKindSpecFor(kind string) episodeKindSpec {
	for _, spec := range episodeKindSpecs {
		if spec.kind == kind {
			return spec
		}
	}

	return episodeKindSpec{}
}

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

func episodeCandidateKinds() []string {
	kinds := make([]string, 0, len(episodeKindSpecs))
	for _, spec := range episodeKindSpecs {
		kinds = append(kinds, spec.kind)
	}

	return kinds
}

func validCandidateKind(kind string) bool {
	return episodeKindSpecFor(kind).kind != ""
}

func usefulness(kind string) string {
	if spec := episodeKindSpecFor(kind); spec.kind != "" {
		return spec.usefulness
	}

	return "low"
}

func candidateKindPriority(kind string) int {
	return episodeKindSpecFor(kind).priority
}

func episodeKindSpecFor(kind string) episodeKindSpec {
	for _, spec := range episodeKindSpecs {
		if spec.kind == kind {
			return spec
		}
	}

	return episodeKindSpec{}
}

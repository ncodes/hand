package summary

import (
	"strings"

	storage "github.com/wandxy/hand/internal/state/core"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/logutils"
)

var summaryLog = logutils.Module("agent.summary")

// State holds the active persisted summary used to replace older session
// messages in model context.
type State struct {
	Current *SummaryState
}

// Recall contains the message slices used when building a recall-only summary
// view without changing the persisted session summary.
type Recall struct {
	PrefixMessages []handmsg.Message
	SessionHistory []handmsg.Message
}

// Summary returns the current summary as a storage record suitable for callers
// that need the persisted shape.
func (s *State) Summary() storage.SessionSummary {
	if s == nil || s.Current == nil {
		return storage.SessionSummary{}
	}

	return storage.CloneSessionSummary(storage.SessionSummary{
		SessionID:          s.Current.SessionID,
		SourceEndOffset:    s.Current.SourceEndOffset,
		SourceMessageCount: s.Current.SourceMessageCount,
		UpdatedAt:          s.Current.UpdatedAt,
		SessionSummary:     s.Current.SessionSummary,
		CurrentTask:        s.Current.CurrentTask,
		Discoveries:        s.Current.Discoveries,
		OpenQuestions:      s.Current.OpenQuestions,
		NextActions:        s.Current.NextActions,
	})
}

// RenderSummaryInstructions renders the current summary into instruction text
// for the model context and reports whether any summary was available.
func (s *State) RenderSummaryInstructions() (string, bool) {
	if s == nil || s.Current == nil {
		return "", false
	}

	sessionSummary := strings.TrimSpace(s.Current.SessionSummary)
	if sessionSummary == "" {
		return "", false
	}

	var sections []string
	sections = append(sections, "# Session Summary\n\n"+sessionSummary)
	if currentTask := strings.TrimSpace(s.Current.CurrentTask); currentTask != "" {
		sections = append(sections, "# Current Task\n\n"+currentTask)
	}
	if discoveries := renderSummaryList("Discoveries", s.Current.Discoveries); discoveries != "" {
		sections = append(sections, discoveries)
	}
	if openQuestions := renderSummaryList("Open Questions", s.Current.OpenQuestions); openQuestions != "" {
		sections = append(sections, openQuestions)
	}
	if nextActions := renderSummaryList("Next Actions", s.Current.NextActions); nextActions != "" {
		sections = append(sections, nextActions)
	}

	return strings.Join(sections, "\n\n"), true
}

// Recall prepares cloned session history for temporary recall summarization.
func (s *State) Recall(sessionHistory []handmsg.Message) Recall {
	return Recall{
		PrefixMessages: nil,
		SessionHistory: handmsg.CloneMessages(sessionHistory),
	}
}

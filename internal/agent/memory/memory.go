package memory

import (
	"strings"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
	"github.com/wandxy/hand/pkg/logutils"
)

var memLog = logutils.InitLogger("agent.memory")

type Memory struct {
	Summary *SummaryState
}

type Recall struct {
	PrefixMessages []handmsg.Message
	SessionHistory []handmsg.Message
}

func (m *Memory) SummaryToStorage() storage.SessionSummary {
	if m == nil || m.Summary == nil {
		return storage.SessionSummary{}
	}

	return common.CloneSessionSummary(storage.SessionSummary{
		SessionID:          m.Summary.SessionID,
		SourceEndOffset:    m.Summary.SourceEndOffset,
		SourceMessageCount: m.Summary.SourceMessageCount,
		UpdatedAt:          m.Summary.UpdatedAt,
		SessionSummary:     m.Summary.SessionSummary,
		CurrentTask:        m.Summary.CurrentTask,
		Discoveries:        m.Summary.Discoveries,
		OpenQuestions:      m.Summary.OpenQuestions,
		NextActions:        m.Summary.NextActions,
	})
}

func (m *Memory) RenderSummaryInstructions() (string, bool) {
	if m == nil || m.Summary == nil {
		return "", false
	}

	sessionSummary := strings.TrimSpace(m.Summary.SessionSummary)
	if sessionSummary == "" {
		return "", false
	}

	var sections []string
	sections = append(sections, "# Session Summary\n\n"+sessionSummary)
	if currentTask := strings.TrimSpace(m.Summary.CurrentTask); currentTask != "" {
		sections = append(sections, "# Current Task\n\n"+currentTask)
	}
	if discoveries := renderSummaryList("Discoveries", m.Summary.Discoveries); discoveries != "" {
		sections = append(sections, discoveries)
	}
	if openQuestions := renderSummaryList("Open Questions", m.Summary.OpenQuestions); openQuestions != "" {
		sections = append(sections, openQuestions)
	}
	if nextActions := renderSummaryList("Next Actions", m.Summary.NextActions); nextActions != "" {
		sections = append(sections, nextActions)
	}

	return strings.Join(sections, "\n\n"), true
}

func (m *Memory) Recall(sessionHistory []handmsg.Message) Recall {
	return Recall{
		PrefixMessages: nil,
		SessionHistory: handmsg.CloneMessages(sessionHistory),
	}
}

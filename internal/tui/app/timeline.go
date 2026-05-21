package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/hand/internal/agent"
	handmsg "github.com/wandxy/hand/internal/messages"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
	tuirpc "github.com/wandxy/hand/internal/tui/rpc"
)

type sessionTimelineLoader = tuirpc.SessionTimelineLoader
type SessionTimelineLoader = tuirpc.SessionTimelineLoader
type sessionTimelineLoadedMsg = tuirpc.SessionTimelineLoaded
type sessionTimelineLoadFailedMsg = tuirpc.SessionTimelineLoadFailed

func loadSessionTimelineCmd(ctx context.Context, client sessionTimelineLoader) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		timeline, err := client.GetSessionTimeline(ctx, rpcclient.SessionTimelineOptions{})
		if err != nil {
			return sessionTimelineLoadFailedMsg{Err: err}
		}

		return sessionTimelineLoadedMsg{Timeline: timeline}
	}
}

func (m *model) hydrateSessionTimeline(timeline rpcclient.SessionTimeline) tea.Cmd {
	cells := sessionTimelineToTranscriptCells(timeline)
	if len(cells) == 0 {
		cells = []transcriptCell{
			systemTranscriptCell{text: fmt.Sprintf("%s has no visible timeline yet.", getSessionTimelineDisplayName(timeline))},
		}
	}

	m.applyAction(setTranscriptCellsAction{Cells: cells})
	m.applyAction(setLiveTranscriptCellAction{})
	m.showIntro = false
	m.stream.Reset()
	m.setTranscriptContent()
	displayName := getSessionTimelineDisplayName(timeline)
	m.applyAction(setSessionTitleAction{Title: displayName})
	m.setDefaultStatus(defaultStatus)
	m.resize()

	return nil
}

func getSessionTimelineDisplayName(timeline rpcclient.SessionTimeline) string {
	title := strings.TrimSpace(timeline.Title)
	sessionID := strings.TrimSpace(timeline.SessionID)
	if title != "" {
		if sessionID == storage.DefaultSessionID {
			return fmt.Sprintf("%s (%s)", title, sessionID)
		}
		return title
	}
	if sessionID != "" {
		return sessionID
	}

	return "session"
}

type transcriptTimelineEntry struct {
	at    time.Time
	order int
	cell  transcriptCell
}

func (entry transcriptTimelineEntry) less(other transcriptTimelineEntry) bool {
	if entry.at.IsZero() || other.at.IsZero() {
		return !entry.at.IsZero() && other.at.IsZero()
	}
	if entry.at.Equal(other.at) {
		return entry.order < other.order
	}

	return entry.at.Before(other.at)
}

func isMessageBackedTimelineEvent(msg any) bool {
	switch msg.(type) {
	case userMessageAcceptedMsg, assistantResponseCompletedMsg:
		return true
	default:
		return false
	}
}

func sessionTimelineToTranscriptCells(timeline rpcclient.SessionTimeline) []transcriptCell {
	entries := make([]transcriptTimelineEntry, 0, len(timeline.Messages)+len(timeline.TraceEvents))
	toolCalls := getTimelineToolCallDetails(timeline.Messages)
	for index, message := range timeline.Messages {
		if cell := timelineMessageToTranscriptCell(message.Message, toolCalls); cell != nil && !cell.IsEmpty() {
			entries = append(entries, transcriptTimelineEntry{
				at:    message.Message.CreatedAt,
				order: index * 2,
				cell:  cell,
			})
		}
	}
	hasMessageTranscript := len(entries) > 0
	for index, event := range timeline.TraceEvents {
		traceEvent := trace.Event{
			Type:      event.Event.Type,
			Timestamp: event.Event.Timestamp,
			Payload:   event.Event.Payload,
		}
		if msg, ok := traceEventToTUIMessage(traceEvent); ok {
			if hasMessageTranscript && isMessageBackedTimelineEvent(msg) {
				continue
			}
			if cell := tuiMessageToTranscriptCell(msg); cell != nil && !cell.IsEmpty() {
				entries = append(entries, transcriptTimelineEntry{
					at:    event.Event.Timestamp,
					order: index*2 + 1,
					cell:  cell,
				})
			}
		}
	}

	sort.SliceStable(entries, func(left int, right int) bool {
		return entries[left].less(entries[right])
	})

	cells := make([]transcriptCell, 0, len(entries))
	for _, entry := range entries {
		cells = append(cells, entry.cell)
	}

	return cells
}

func timelineMessageToTranscriptCell(message handmsg.Message, toolCalls map[string]timelineToolCallDetail) transcriptCell {
	return defaultTranscriptCellFactory.FromTimelineMessage(message, toolCalls)
}

type timelineToolCallDetail struct {
	detail       string
	planState    *trace.PlanToolState
	processState *trace.ProcessToolState
	startedAt    time.Time
}

func getTimelineToolCallDetails(messages []agent.SessionTimelineMessage) map[string]timelineToolCallDetail {
	details := map[string]timelineToolCallDetail{}
	for _, message := range messages {
		for _, toolCall := range message.Message.ToolCalls {
			id := strings.TrimSpace(toolCall.ID)
			if id == "" {
				continue
			}
			startedMsg, _ := toolInvocationStartedMsgFromMessageToolCall(
				toolCall,
				message.Message.CreatedAt,
			)
			details[id] = timelineToolCallDetail{
				detail:       startedMsg.Detail,
				planState:    startedMsg.PlanState,
				processState: startedMsg.ProcessState,
				startedAt:    startedMsg.StartedAt,
			}
		}
	}

	return details
}

func tuiMessageToTranscriptCell(msg any) transcriptCell {
	return defaultTranscriptCellFactory.FromTUIMessage(msg)
}

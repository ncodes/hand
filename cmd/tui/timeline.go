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
)

type sessionTimelineLoader interface {
	GetSessionTimeline(context.Context, rpcclient.SessionTimelineOptions) (rpcclient.SessionTimeline, error)
}

type sessionTimelineLoadedMsg struct {
	Timeline rpcclient.SessionTimeline
}

type sessionTimelineLoadFailedMsg struct {
	Err error
}

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
		cells = []string{fmt.Sprintf("%s has no visible timeline yet.", getSessionTimelineDisplayName(timeline))}
	}

	m.messages = cells
	m.live = ""
	m.showIntro = false
	m.stream.Reset()
	m.setTranscriptContent()
	displayName := getSessionTimelineDisplayName(timeline)
	m.setSessionTitle(displayName)
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

func sessionTimelineToTranscriptCells(timeline rpcclient.SessionTimeline) []string {
	entries := make([]transcriptTimelineEntry, 0, len(timeline.Messages)+len(timeline.TraceEvents))
	toolDetails := getTimelineToolCallDetails(timeline.Messages)
	for index, message := range timeline.Messages {
		if cell := timelineMessageToTranscriptCell(message.Message, toolDetails); cell != "" {
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
			if cell := tuiMessageToTranscriptCell(msg); cell != "" {
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

	cells := make([]string, 0, len(entries))
	for _, entry := range entries {
		cells = append(cells, entry.cell)
	}

	return cells
}

type transcriptTimelineEntry struct {
	at    time.Time
	order int
	cell  string
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

func timelineMessageToTranscriptCell(message handmsg.Message, toolDetails map[string]string) string {
	content := strings.TrimSpace(message.Content)
	if content == "" && len(message.ToolCalls) == 0 {
		return ""
	}

	switch message.Role {
	case handmsg.RoleUser:
		if content == "" {
			return ""
		}
		return "You: " + content
	case handmsg.RoleAssistant:
		if content == "" {
			return ""
		}
		return "Hand: " + content
	case handmsg.RoleTool:
		name := strings.TrimSpace(message.Name)
		if name == "" {
			name = "tool"
		}
		return toolOperationTranscriptCell(
			message.ToolCallID,
			name,
			toolDetails[strings.TrimSpace(message.ToolCallID)],
			true,
		)
	default:
		if content == "" {
			return ""
		}
		return strings.TrimSpace(string(message.Role)) + ": " + content
	}
}

func getTimelineToolCallDetails(messages []agent.SessionTimelineMessage) map[string]string {
	details := map[string]string{}
	for _, message := range messages {
		for _, toolCall := range message.Message.ToolCalls {
			id := strings.TrimSpace(toolCall.ID)
			if id == "" {
				continue
			}
			if detail := getToolInputDisplayDetail(toolCall.Name, toolCall.Input); detail != "" {
				details[id] = detail
			}
		}
	}

	return details
}

func tuiMessageToTranscriptCell(msg any) string {
	switch value := msg.(type) {
	case userMessageAcceptedMsg:
		if text := strings.TrimSpace(value.Text); text != "" {
			return "You: " + text
		}
	case assistantTextDeltaMsg:
		if text := strings.TrimSpace(value.Text); text != "" {
			return "Hand: " + text
		}
	case assistantResponseCompletedMsg:
		if text := strings.TrimSpace(value.Text); text != "" {
			return "Hand: " + text
		}
	case toolInvocationStartedMsg:
		return toolOperationTranscriptCellWithTiming(value.ID, value.Name, value.Detail, value.StartedAt, time.Time{}, false)
	case toolInvocationCompletedMsg:
		return toolOperationTranscriptCellWithTiming(value.ID, value.Name, value.Detail, time.Time{}, value.CompletedAt, true)
	case safetyEventMsg:
		return safetyEventToTranscriptCell(value)
	case sessionErrorMsg:
		if message := strings.TrimSpace(value.Message); message != "" {
			return "Error: " + message
		}
	}

	return ""
}

func safetyEventToTranscriptCell(msg safetyEventMsg) string {
	parts := []string{"Safety"}
	if action := strings.TrimSpace(msg.Action); action != "" {
		parts = append(parts, action)
	}
	if len(msg.FindingIDs) > 0 {
		parts = append(parts, strings.Join(msg.FindingIDs, ", "))
	}

	return strings.Join(parts, ": ")
}

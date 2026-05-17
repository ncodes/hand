package tui

import (
	"fmt"
	"strings"

	handmsg "github.com/wandxy/hand/internal/messages"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/trace"
)

func (m *model) hydrateSessionTimeline(timeline rpcclient.SessionTimeline) {
	cells := sessionTimelineToTranscriptCells(timeline)
	if len(cells) == 0 {
		sessionID := strings.TrimSpace(timeline.SessionID)
		if sessionID == "" {
			sessionID = "session"
		}
		cells = []string{fmt.Sprintf("%s has no visible timeline yet.", sessionID)}
	}

	m.messages = cells
	m.transcript.SetContent(strings.Join(cells, "\n\n"))
	m.transcript.GotoTop()
	if sessionID := strings.TrimSpace(timeline.SessionID); sessionID != "" {
		m.status = sessionID + " · hydrated"
	}
	m.resize()
}

func sessionTimelineToTranscriptCells(timeline rpcclient.SessionTimeline) []string {
	cells := make([]string, 0, len(timeline.Messages)+len(timeline.TraceEvents))
	for _, message := range timeline.Messages {
		if cell := timelineMessageToTranscriptCell(message.Message); cell != "" {
			cells = append(cells, cell)
		}
	}
	for _, event := range timeline.TraceEvents {
		traceEvent := trace.Event{
			Type:      event.Event.Type,
			Timestamp: event.Event.Timestamp,
			Payload:   event.Event.Payload,
		}
		if msg, ok := traceEventToTUIMessage(traceEvent); ok {
			if cell := tuiMessageToTranscriptCell(msg); cell != "" {
				cells = append(cells, cell)
			}
		}
	}

	return cells
}

func timelineMessageToTranscriptCell(message handmsg.Message) string {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return ""
	}

	switch message.Role {
	case handmsg.RoleUser:
		return "You: " + content
	case handmsg.RoleAssistant:
		return "Hand: " + content
	case handmsg.RoleTool:
		name := strings.TrimSpace(message.Name)
		if name == "" {
			name = "tool"
		}
		return "Tool " + name + ": " + content
	default:
		return strings.TrimSpace(string(message.Role)) + ": " + content
	}
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
		if name := strings.TrimSpace(value.Name); name != "" {
			return "Tool started: " + name
		}
	case toolInvocationCompletedMsg:
		if name := strings.TrimSpace(value.Name); name != "" {
			return "Tool completed: " + name
		}
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

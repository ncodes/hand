package tui

import (
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/trace"
)

type transcriptCellFactory struct{}

type toolTranscriptCellInput struct {
	ID           string
	Name         string
	Detail       string
	PlanState    *trace.PlanToolState
	ProcessState *trace.ProcessToolState
	StartedAt    time.Time
	CompletedAt  time.Time
	Completed    bool
}

var defaultTranscriptCellFactory = transcriptCellFactory{}

func (transcriptCellFactory) User(text string) transcriptCell {
	if text = strings.TrimSpace(text); text == "" {
		return nil
	}

	return userTranscriptCell{text: text}
}

func (transcriptCellFactory) Assistant(text string) transcriptCell {
	if text = strings.TrimSpace(text); text == "" {
		return nil
	}

	return assistantTranscriptCell{text: text}
}

func (transcriptCellFactory) Reasoning(text string, startedAt time.Time) transcriptCell {
	return newReasoningTranscriptCell(text, startedAt)
}

func (transcriptCellFactory) Thought(duration time.Duration) transcriptCell {
	if duration <= 0 {
		return nil
	}

	return thoughtTranscriptCell{duration: duration}
}

func (transcriptCellFactory) Tool(input toolTranscriptCellInput) transcriptCell {
	return newToolTranscriptCell(
		input.ID,
		input.Name,
		input.Detail,
		input.PlanState,
		input.ProcessState,
		input.StartedAt,
		input.CompletedAt,
		input.Completed,
	)
}

func (transcriptCellFactory) Safety(msg safetyEventMsg) transcriptCell {
	return safetyTranscriptCell{
		action:     strings.TrimSpace(msg.Action),
		findingIDs: msg.FindingIDs,
	}
}

func (transcriptCellFactory) Error(message string) transcriptCell {
	if message = strings.TrimSpace(message); message == "" {
		return nil
	}

	return errorTranscriptCell{message: message}
}

func (transcriptCellFactory) System(text string) transcriptCell {
	if text = strings.TrimSpace(text); text == "" {
		return nil
	}

	return systemTranscriptCell{text: text}
}

func (transcriptCellFactory) ManualCompaction(state manualCompactionState) transcriptCell {
	cell := manualCompactionTranscriptCell{state: state}
	if cell.IsEmpty() {
		return nil
	}

	return cell
}

func (factory transcriptCellFactory) FromTUIMessage(msg any) transcriptCell {
	switch value := msg.(type) {
	case userMessageAcceptedMsg:
		return factory.User(value.Text)
	case assistantTextDeltaMsg:
		if isReasoningDeltaChannel(value.Channel) {
			return factory.Reasoning(value.Text, currentTime())
		}

		return factory.Assistant(value.Text)
	case assistantResponseCompletedMsg:
		return factory.Assistant(value.Text)
	case reasoningCompletedMsg:
		return factory.Thought(value.Duration)
	case toolInvocationStartedMsg:
		return factory.Tool(toolTranscriptCellInput{
			ID:           value.ID,
			Name:         value.Name,
			Detail:       value.Detail,
			PlanState:    value.PlanState,
			ProcessState: value.ProcessState,
			StartedAt:    value.StartedAt,
		})
	case toolInvocationCompletedMsg:
		return factory.Tool(toolTranscriptCellInput{
			ID:           value.ID,
			Name:         value.Name,
			Detail:       value.Detail,
			PlanState:    value.PlanState,
			ProcessState: value.ProcessState,
			CompletedAt:  value.CompletedAt,
			Completed:    true,
		})
	case safetyEventMsg:
		return factory.Safety(value)
	case sessionErrorMsg:
		return factory.Error(value.Message)
	case manualCompactionMsg:
		return factory.ManualCompaction(value.State)
	default:
		return nil
	}
}

func (factory transcriptCellFactory) FromTimelineMessage(
	message handmsg.Message,
	toolCalls map[string]timelineToolCallDetail,
) transcriptCell {
	content := strings.TrimSpace(message.Content)
	if content == "" && len(message.ToolCalls) == 0 {
		return nil
	}

	switch message.Role {
	case handmsg.RoleUser:
		return factory.User(content)
	case handmsg.RoleAssistant:
		return factory.Assistant(content)
	case handmsg.RoleTool:
		name := strings.TrimSpace(message.Name)
		if name == "" {
			name = "tool"
		}
		toolCall := toolCalls[strings.TrimSpace(message.ToolCallID)]
		planState := mergePlanToolDisplayState(toolCall.planState, getToolOutputDisplayState(name, content))
		processState := mergeProcessToolDisplayState(toolCall.processState, getToolOutputProcessDisplayState(name, content))
		return factory.Tool(toolTranscriptCellInput{
			ID:           message.ToolCallID,
			Name:         name,
			Detail:       toolCall.detail,
			PlanState:    planState,
			ProcessState: processState,
			StartedAt:    toolCall.startedAt,
			CompletedAt:  message.CreatedAt,
			Completed:    true,
		})
	default:
		return factory.System(strings.TrimSpace(string(message.Role)) + ": " + content)
	}
}

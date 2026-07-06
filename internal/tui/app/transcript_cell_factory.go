package tui

import (
	"time"

	"github.com/wandxy/morph/internal/trace"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(text)
	if text = stringValue1.Trim(); text == "" {
		return nil
	}

	return userTranscriptCell{text: text}
}

func (transcriptCellFactory) Assistant(text string) transcriptCell {
	stringValue2 := str.String(text)
	if text = stringValue2.Trim(); text == "" {
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
	stringValue3 := str.String(msg.Action)
	return safetyTranscriptCell{
		action:     stringValue3.Trim(),
		findingIDs: msg.FindingIDs,
	}
}

func (transcriptCellFactory) Error(message string) transcriptCell {
	if message = getUserFacingErrorMessage(message); message == "" {
		return nil
	}

	return errorTranscriptCell{message: message}
}

func (transcriptCellFactory) System(text string) transcriptCell {
	stringValue4 := str.String(text)
	if text = stringValue4.Trim(); text == "" {
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
	message morphmsg.Message,
	toolCalls map[string]timelineToolCallDetail,
) transcriptCell {
	stringValue5 := str.String(message.Content)
	content := stringValue5.Trim()
	if content == "" && len(message.ToolCalls) == 0 {
		return nil
	}

	switch message.Role {
	case morphmsg.RoleUser:
		return factory.User(content)
	case morphmsg.RoleAssistant:
		return factory.Assistant(content)
	case morphmsg.RoleTool:
		stringValue6 := str.String(message.Name)
		name := stringValue6.Trim()
		if name == "" {
			name = "tool"
		}
		stringValue7 := str.String(message.ToolCallID)
		toolCall := toolCalls[stringValue7.Trim()]
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
		stringValue8 := str.String(string(message.Role))
		return factory.System(stringValue8.Trim() + ": " + content)
	}
}

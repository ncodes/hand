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
	Failed       bool
	Artifact     *browserArtifact
}

var defaultTranscriptCellFactory = transcriptCellFactory{}

func (transcriptCellFactory) User(text string) transcriptCell {
	textValue := str.String(text)
	if text = textValue.Trim(); text == "" {
		return nil
	}

	return userTranscriptCell{text: text}
}

func (transcriptCellFactory) Assistant(text string) transcriptCell {
	textValue2 := str.String(text)
	if text = textValue2.Trim(); text == "" {
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
	cell := newToolTranscriptCell(
		input.ID,
		input.Name,
		input.Detail,
		input.PlanState,
		input.ProcessState,
		input.StartedAt,
		input.CompletedAt,
		input.Completed,
	)
	toolCell, ok := cell.(toolTranscriptCell)
	if ok && input.Artifact != nil {
		toolCell.artifact = *input.Artifact
		toolCell.hasArtifact = true
		cell = toolCell
	}
	if !ok || !input.Failed {
		return cell
	}

	toolCell.completed = false
	toolCell.terminalStatus = toolTranscriptTerminalStatusFailed
	return toolCell
}

func (transcriptCellFactory) Safety(msg safetyEventMsg) transcriptCell {
	actionValue := str.String(msg.Action)
	return safetyTranscriptCell{
		action:     actionValue.Trim(),
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
	textValue3 := str.String(text)
	if text = textValue3.Trim(); text == "" {
		return nil
	}

	return systemTranscriptCell{text: text}
}

func (transcriptCellFactory) PermissionApproval(message permissionApprovalMsg) transcriptCell {
	message.Effects = append([]string(nil), message.Effects...)
	cell := permissionApprovalTranscriptCell{message: message}
	if cell.IsEmpty() {
		return nil
	}

	return cell
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
			Completed:    !value.Failed,
			Failed:       value.Failed,
			Artifact:     value.Artifact,
		})
	case safetyEventMsg:
		return factory.Safety(value)
	case sessionErrorMsg:
		return factory.Error(value.Message)
	case manualCompactionMsg:
		return factory.ManualCompaction(value.State)
	case permissionApprovalMsg:
		return factory.PermissionApproval(value)
	default:
		return nil
	}
}

func (factory transcriptCellFactory) FromTimelineMessage(
	message morphmsg.Message,
	toolCalls map[string]timelineToolCallDetail,
) transcriptCell {
	contentValue := str.String(message.Content)
	content := contentValue.Trim()
	if content == "" && len(message.ToolCalls) == 0 {
		return nil
	}

	switch message.Role {
	case morphmsg.RoleUser:
		return factory.User(content)
	case morphmsg.RoleAssistant:
		return factory.Assistant(content)
	case morphmsg.RoleTool:
		nameValue := str.String(message.Name)
		name := nameValue.Trim()
		if name == "" {
			name = "tool"
		}
		toolCallIDValue := str.String(message.ToolCallID)
		toolCall := toolCalls[toolCallIDValue.Trim()]
		planState := mergePlanToolDisplayState(toolCall.planState, getToolOutputDisplayState(name, content))
		processState := mergeProcessToolDisplayState(toolCall.processState, getToolOutputProcessDisplayState(name, content))
		failed := trace.ToolInvocationFailed(content)
		return factory.Tool(toolTranscriptCellInput{
			ID:           message.ToolCallID,
			Name:         name,
			Detail:       toolCall.detail,
			PlanState:    planState,
			ProcessState: processState,
			StartedAt:    toolCall.startedAt,
			CompletedAt:  message.CreatedAt,
			Completed:    !failed,
			Failed:       failed,
			Artifact:     getBrowserArtifact(name, content),
		})
	default:
		roleValue := str.String(string(message.Role))
		return factory.System(roleValue.Trim() + ": " + content)
	}
}

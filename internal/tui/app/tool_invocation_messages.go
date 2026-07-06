package tui

import (
	"time"

	models "github.com/wandxy/morph/internal/model"
	"github.com/wandxy/morph/internal/trace"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/str"
)

func newToolInvocationStartedMsg(
	id string,
	name string,
	detail string,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	stringValue1 := str.String(id)
	id = stringValue1.Trim()
	stringValue2 := str.String(name)
	name = stringValue2.Trim()
	stringValue3 := str.String(detail)
	detail = stringValue3.Trim()
	if name == "" && id == "" {
		return toolInvocationStartedMsg{}, false
	}

	return toolInvocationStartedMsg{
		ID:        id,
		Name:      name,
		Detail:    detail,
		StartedAt: startedAt,
	}, true
}

func newToolInvocationStartedMsgWithState(
	id string,
	name string,
	detail string,
	planState *trace.PlanToolState,
	processState *trace.ProcessToolState,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	msg, ok := newToolInvocationStartedMsg(id, name, detail, startedAt)
	if !ok {
		return toolInvocationStartedMsg{}, false
	}
	msg.PlanState = planState
	msg.ProcessState = processState
	return msg, true
}

func newToolInvocationCompletedMsg(
	id string,
	name string,
	detail string,
	completedAt time.Time,
) (toolInvocationCompletedMsg, bool) {
	stringValue4 := str.String(id)
	id = stringValue4.Trim()
	stringValue5 := str.String(name)
	name = stringValue5.Trim()
	stringValue6 := str.String(detail)
	detail = stringValue6.Trim()
	if name == "" && id == "" {
		return toolInvocationCompletedMsg{}, false
	}

	return toolInvocationCompletedMsg{
		ID:          id,
		Name:        name,
		Detail:      detail,
		CompletedAt: completedAt,
	}, true
}

func newToolInvocationCompletedMsgWithState(
	id string,
	name string,
	detail string,
	planState *trace.PlanToolState,
	processState *trace.ProcessToolState,
	completedAt time.Time,
) (toolInvocationCompletedMsg, bool) {
	msg, ok := newToolInvocationCompletedMsg(id, name, detail, completedAt)
	if !ok {
		return toolInvocationCompletedMsg{}, false
	}
	msg.PlanState = planState
	msg.ProcessState = processState
	return msg, true
}

func toolInvocationStartedMsgFromModelToolCall(
	toolCall models.ToolCall,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	return newToolInvocationStartedMsgWithState(
		toolCall.ID,
		toolCall.Name,
		getToolInputDisplayDetail(toolCall.Name, toolCall.Input),
		getToolInputDisplayState(toolCall.Name, toolCall.Input),
		getToolInputProcessDisplayState(toolCall.Name, toolCall.Input),
		startedAt,
	)
}

func toolInvocationStartedMsgFromMessageToolCall(
	toolCall morphmsg.ToolCall,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	return newToolInvocationStartedMsgWithState(
		toolCall.ID,
		toolCall.Name,
		getToolInputDisplayDetail(toolCall.Name, toolCall.Input),
		getToolInputDisplayState(toolCall.Name, toolCall.Input),
		getToolInputProcessDisplayState(toolCall.Name, toolCall.Input),
		startedAt,
	)
}

func toolInvocationCompletedMsgFromMessage(
	message morphmsg.Message,
	completedAt time.Time,
) (toolInvocationCompletedMsg, bool) {
	return newToolInvocationCompletedMsgWithState(
		message.ToolCallID,
		message.Name,
		getToolOutputDisplayDetail(message.Name, message.Content),
		getToolOutputDisplayState(message.Name, message.Content),
		getToolOutputProcessDisplayState(message.Name, message.Content),
		completedAt,
	)
}

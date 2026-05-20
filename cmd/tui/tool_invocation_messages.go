package tui

import (
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func newToolInvocationStartedMsg(
	id string,
	name string,
	detail string,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	detail = strings.TrimSpace(detail)
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

func newToolInvocationCompletedMsg(
	id string,
	name string,
	detail string,
	completedAt time.Time,
) (toolInvocationCompletedMsg, bool) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	detail = strings.TrimSpace(detail)
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

func toolInvocationStartedMsgFromModelToolCall(
	toolCall models.ToolCall,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	return newToolInvocationStartedMsg(
		toolCall.ID,
		toolCall.Name,
		getToolInputDisplayDetail(toolCall.Name, toolCall.Input),
		startedAt,
	)
}

func toolInvocationStartedMsgFromMessageToolCall(
	toolCall handmsg.ToolCall,
	startedAt time.Time,
) (toolInvocationStartedMsg, bool) {
	return newToolInvocationStartedMsg(
		toolCall.ID,
		toolCall.Name,
		getToolInputDisplayDetail(toolCall.Name, toolCall.Input),
		startedAt,
	)
}

func toolInvocationCompletedMsgFromMessage(
	message handmsg.Message,
	completedAt time.Time,
) (toolInvocationCompletedMsg, bool) {
	return newToolInvocationCompletedMsg(
		message.ToolCallID,
		message.Name,
		"",
		completedAt,
	)
}

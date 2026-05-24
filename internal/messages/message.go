package messages

import agentmessage "github.com/wandxy/hand/pkg/agent/message"

type Role = agentmessage.Role

const (
	RoleDeveloper = agentmessage.RoleDeveloper
	RoleUser      = agentmessage.RoleUser
	RoleAssistant = agentmessage.RoleAssistant
	RoleTool      = agentmessage.RoleTool
)

type Message = agentmessage.Message

type ToolCall = agentmessage.ToolCall

func New(role Role, content string) (Message, error) {
	return agentmessage.New(role, content)
}

func NewMessage(role Role, content string) (Message, error) {
	return agentmessage.NewMessage(role, content)
}

func Normalize(message Message) (Message, error) {
	return agentmessage.Normalize(message)
}

func NormalizeMessage(message Message) (Message, error) {
	return agentmessage.NormalizeMessage(message)
}

func CloneMessages(messages []Message) []Message {
	return agentmessage.CloneMessages(messages)
}

package messages

import (
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	ID         uint
	Role       Role
	Content    string
	SearchText string
	Name       string
	ToolCallID string
	ToolCalls  []ToolCall
	CreatedAt  time.Time
}

type ToolCall struct {
	ID    string
	Name  string
	Input string
}

func New(role Role, content string) (Message, error) {
	normalizedRole, err := normalizeRole(role)
	if err != nil {
		return Message{}, err
	}

	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return Message{}, errors.New("message content is required")
	}

	return Message{
		Role:      normalizedRole,
		Content:   trimmedContent,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func NewMessage(role Role, content string) (Message, error) {
	return New(role, content)
}

func Normalize(message Message) (Message, error) {
	role, err := normalizeRole(message.Role)
	if err != nil {
		return Message{}, err
	}

	toolCalls, err := normalizeToolCalls(message.ToolCalls)
	if err != nil {
		return Message{}, err
	}

	toolCallID := strings.TrimSpace(message.ToolCallID)
	name := strings.TrimSpace(message.Name)
	content := strings.TrimSpace(message.Content)

	if content == "" && !(role == RoleAssistant && len(toolCalls) > 0) {
		return Message{}, errors.New("message content is required")
	}

	if role == RoleTool && toolCallID == "" {
		return Message{}, errors.New("tool call id is required")
	}

	normalized := Message{
		ID:         message.ID,
		Role:       role,
		Content:    content,
		SearchText: strings.TrimSpace(message.SearchText),
		Name:       name,
		ToolCallID: toolCallID,
		ToolCalls:  toolCalls,
		CreatedAt:  message.CreatedAt.UTC(),
	}

	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}

	return normalized, nil
}

func NormalizeMessage(message Message) (Message, error) {
	return Normalize(message)
}

func CloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]Message, len(messages))
	for i, message := range messages {
		copyMessage := message
		if len(message.ToolCalls) > 0 {
			copyMessage.ToolCalls = make([]ToolCall, len(message.ToolCalls))
			copy(copyMessage.ToolCalls, message.ToolCalls)
		}
		cloned[i] = copyMessage
	}

	return cloned
}

func normalizeToolCalls(toolCalls []ToolCall) ([]ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Name)
		input := strings.TrimSpace(toolCall.Input)

		if id == "" {
			return nil, errors.New("tool call id is required")
		}

		if name == "" {
			return nil, errors.New("tool call name is required")
		}

		normalized = append(normalized, ToolCall{ID: id, Name: name, Input: input})
	}

	return normalized, nil
}

func normalizeRole(role Role) (Role, error) {
	switch Role(strings.ToLower(strings.TrimSpace(string(role)))) {
	case RoleDeveloper:
		return RoleDeveloper, nil
	case RoleUser:
		return RoleUser, nil
	case RoleAssistant:
		return RoleAssistant, nil
	case RoleTool:
		return RoleTool, nil
	default:
		return "", errors.New("message role must be one of developer, user, assistant, or tool")
	}
}

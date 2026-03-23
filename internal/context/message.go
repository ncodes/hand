package context

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
	Role      Role
	Content   string
	CreatedAt time.Time
}

func NewMessage(role Role, content string) (Message, error) {
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

func normalizeMessage(message Message) (Message, error) {
	role, err := normalizeRole(message.Role)
	if err != nil {
		return Message{}, err
	}

	content := strings.TrimSpace(message.Content)
	if content == "" {
		return Message{}, errors.New("message content is required")
	}

	normalized := Message{
		Role:      role,
		Content:   content,
		CreatedAt: message.CreatedAt.UTC(),
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
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

package context

import "errors"

type Conversation struct {
	messages []Message
}

func NewConversation() Conversation {
	return Conversation{messages: []Message{}}
}

func (c *Conversation) Append(message Message) error {
	if c == nil {
		return errors.New("conversation is required")
	}

	normalized, err := normalizeMessage(message)
	if err != nil {
		return err
	}

	c.messages = append(c.messages, normalized)
	return nil
}

func (c *Conversation) AppendUser(content string) error {
	message, err := NewMessage(RoleUser, content)
	if err != nil {
		return err
	}

	return c.Append(message)
}

func (c *Conversation) AppendAssistant(content string) error {
	message, err := NewMessage(RoleAssistant, content)
	if err != nil {
		return err
	}

	return c.Append(message)
}

func (c Conversation) Messages() []Message {
	if len(c.messages) == 0 {
		return []Message{}
	}

	messages := make([]Message, len(c.messages))
	copy(messages, c.messages)
	return messages
}

func (c Conversation) Len() int {
	return len(c.messages)
}

func (c Conversation) Empty() bool {
	return len(c.messages) == 0
}

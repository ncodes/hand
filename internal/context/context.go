package context

import (
	gctx "context"

	"github.com/wandxy/hand/internal/config"
)

// Context holds the agent's runtime context and instructions.
type Context struct {
	cfg          *config.Config
	ctx          gctx.Context
	instructions Instructions
	conversation Conversation
}

// NewContext creates a new context with the given configuration and go context.
func NewContext(ctx gctx.Context, cfg *config.Config) *Context {
	return &Context{
		cfg:          cfg,
		ctx:          ctx,
		instructions: Instructions{},
		conversation: NewConversation(),
	}
}

// AddInstruction adds a new instruction to the context.
func (c *Context) AddInstruction(instruction Instruction) {
	c.instructions = append(c.instructions, instruction)
}

// GetInstructions returns the instructions for the context.
func (c *Context) GetInstructions() Instructions {
	return c.instructions
}

func (c *Context) AddMessage(message Message) error {
	return c.conversation.Append(message)
}

func (c *Context) AddUserMessage(content string) error {
	return c.conversation.AppendUser(content)
}

func (c *Context) AddAssistantMessage(content string) error {
	return c.conversation.AppendAssistant(content)
}

func (c *Context) GetMessages() []Message {
	return c.conversation.Messages()
}

func (c *Context) GetConversation() Conversation {
	conversation := NewConversation()
	for _, message := range c.conversation.Messages() {
		_ = conversation.Append(message)
	}

	return conversation
}

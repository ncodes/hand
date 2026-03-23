package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/models"
)

type runtimeEnvironment interface {
	Prepare() error
	Context() environment.Context
}

var newRuntimeEnvironment = func(ctx context.Context, cfg *config.Config) runtimeEnvironment {
	return environment.NewEnvironment(ctx, cfg)
}

// Agent is the main agent struct.
type Agent struct {
	ctx         context.Context
	cfg         *config.Config
	modelClient models.Client
	env         runtimeEnvironment
}

// NewAgent creates a new agent with the given configuration and model client.
func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{
		ctx:         ctx,
		cfg:         cfg,
		modelClient: modelClient,
	}
}

// Chat processes a user message and returns a response.
func (c *Agent) Chat(ctx context.Context, msg string) (string, error) {
	if c == nil {
		return "", errors.New("agent is required")
	}
	if c.env == nil {
		return "", errors.New("environment has not been initialized")
	}
	if c.modelClient == nil {
		return "", errors.New("model client is required")
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}
    
	handCtx := c.env.Context()
	if err := handCtx.AddUserMessage(msg); err != nil {
		return "", err
	}

	instructions := handCtx.GetInstructions()
	resp, err := c.modelClient.Chat(ctx, models.GenerateRequest{
		Model:         c.cfg.Model,
		Instructions:  instructions.String(),
		Messages:      handCtx.GetMessages(),
		DebugRequests: c.cfg.DebugRequests,
	})
	if err != nil {
		return "", err
	}
	if err := handCtx.AddAssistantMessage(resp.OutputText); err != nil {
		return "", err
	}

	return resp.OutputText, nil
}

func (c *Agent) Run(context.Context) error {
	if c == nil {
		return errors.New("agent is required")
	}
	if c.cfg == nil {
		return errors.New("config is required")
	}

	c.env = newRuntimeEnvironment(c.ctx, c.cfg)

	if err := c.env.Prepare(); err != nil {
		return err
	}

	return nil
}

func (c *Agent) Conversation() handctx.Conversation {
	if c == nil {
		return handctx.NewConversation()
	}
	if c.env == nil {
		return handctx.NewConversation()
	}

	return c.env.Context().GetConversation()
}

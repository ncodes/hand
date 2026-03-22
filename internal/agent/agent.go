package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/models"
)

// Agent is the main agent struct.
type Agent struct {
	ctx         context.Context
	cfg         *config.Config
	modelClient models.Client
	env         *environment.Environment
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
	if c.env == nil {
		return "", errors.New("environment has not been initialized")
	}
	if c == nil {
		return "", errors.New("agent is required")
	}
	if c.modelClient == nil {
		return "", errors.New("model client is required")
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}

	instructions := c.env.GetInstructions()
	resp, err := c.modelClient.Chat(ctx, models.GenerateRequest{
		Model:        c.cfg.Model,
		Input:        msg,
		Instructions: instructions.String(),
	})
	if err != nil {
		return "", err
	}

	return resp.OutputText, nil
}

func (c *Agent) Run(context.Context) error {
	c.env = environment.NewEnvironment(c.ctx, c.cfg)

	if err := c.env.Prepare(); err != nil {
		return err
	}

	return nil
}

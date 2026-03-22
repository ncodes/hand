package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/agent/internal/config"
	"github.com/wandxy/agent/internal/models"
)

// Agent is the main agent struct.
type Agent struct {
	cfg         *config.Config
	modelClient models.Client
}

// NewAgent creates a new agent with the given configuration and model client.
func NewAgent(cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{
		cfg:         cfg,
		modelClient: modelClient,
	}
}

// Chat processes a user message and returns a response.
func (c *Agent) Chat(ctx context.Context, msg string) (string, error) {
	if c == nil {
		return "", errors.New("agent is required")
	}
	if c.modelClient == nil {
		return "", errors.New("model client is required")
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}

	resp, err := c.modelClient.Chat(ctx, models.GenerateRequest{
		Model: c.cfg.Model,
		Input: msg,
	})
	if err != nil {
		return "", err
	}

	return resp.OutputText, nil
}

func (c *Agent) Run(context.Context) error {
	return nil
}

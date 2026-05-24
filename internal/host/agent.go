package host

import (
	"context"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
)

func NewAgent(
	ctx context.Context,
	cfg *config.Config,
	modelClient models.Client,
	optionalSummary ...models.Client,
) *agent.Agent {
	return agent.NewAgent(ctx, cfg, modelClient, optionalSummary...)
}

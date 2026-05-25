package agent

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
)

// EnvironmentFactory creates the runtime environment for an Agent.
type EnvironmentFactory func(context.Context, *config.Config) environment.Environment

// NewEnvironment is the production environment factory used during Agent startup.
var NewEnvironment EnvironmentFactory = environment.NewEnvironment

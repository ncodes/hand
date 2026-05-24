package host

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
)

type EnvironmentFactory func(context.Context, *config.Config) environment.Environment

var NewEnvironment EnvironmentFactory = environment.NewEnvironment

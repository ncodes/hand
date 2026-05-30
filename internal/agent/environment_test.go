package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/mocks"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
)

func TestEnvironment_NewEnvironmentFactoryCreatesUsableRuntime(t *testing.T) {
	env := NewEnvironment(context.Background(), &config.Config{})
	manager, err := statemanager.NewManager(
		&stateStoreStub{session: storage.Session{ID: storage.DefaultSessionID}},
		time.Hour,
		time.Hour,
	)
	require.NoError(t, err)
	env.SetStateManager(manager)
	env.SetModelClient(&mocks.ModelClientStub{})

	require.NoError(t, env.Prepare())
	definitions, err := env.Tools().Resolve(env.ToolPolicy())
	require.NoError(t, err)
	require.Greater(t, len(definitions), 0)
	require.Empty(t, env.NewTraceSession("default").ID())
	require.Positive(t, env.NewIterationBudget().Remaining())
}

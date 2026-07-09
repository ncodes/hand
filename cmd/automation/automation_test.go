package automation

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetOutputDelegatesToCLI(t *testing.T) {
	output := &bytes.Buffer{}
	previous := SetOutput(output)
	t.Cleanup(func() { SetOutput(previous) })

	require.Same(t, output, SetOutput(io.Discard))
}

func TestNewCommandBuildsRootCommand(t *testing.T) {
	cmd := NewCommand()

	require.Equal(t, "automation", cmd.Name)
	require.NotEmpty(t, cmd.Commands)
}

func TestNewCommandShowsHelpForMissingSubcommand(t *testing.T) {
	require.NoError(t, NewCommand().Run(context.Background(), []string{"automation"}))
}

package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestModel_UpdateBlocksLocalCommandWhenShellIsDisabled(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("!ls -la")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "local commands are disabled", runModel.status.Text())
	require.Equal(t, []string{"Local command blocked: !ls -la"}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.input.Value())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Local command blocked: !ls -la")
}

func TestModel_UpdateQueuesLocalCommandWhenShellIsAllowed(t *testing.T) {
	runModel := newModel()
	runModel.allowShell = true
	runModel.input.SetValue("!pwd")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "local command execution is not connected yet", runModel.status.Text())
	require.Equal(t, []string{"Local command queued: !pwd"}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.input.Value())
}

package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestModel_UpdateHandlesChangelogCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/changelog")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.input.Value())
	require.Empty(t, runModel.messages)
	require.True(t, runModel.isCommandViewVisible())
	require.Equal(t, "✦", runModel.commandView.TitleIcon)
	require.Equal(t, "Changelog", runModel.commandView.TitleLeft)
	require.Equal(t, "See what is new", runModel.commandView.TitleSubtext)
	require.Equal(t, "esc to close", runModel.commandView.TitleRight)
	require.Empty(t, runModel.commandView.AccentColor)
	require.Equal(t, defaultTUITheme.MutedText, runModel.commandView.TitleRightColor)

	content := stripANSI(runModel.View().Content)
	require.Contains(t, content, "✦ Changelog")
	require.Contains(t, content, "Changelog")
	require.Contains(t, content, "See what is new")
	require.Contains(t, content, "Unreleased")
	require.Contains(t, content, "GitHub Copilot")
	require.NotContains(t, content, "## Unreleased")
	require.NotContains(t, content, "- Added")
	require.Contains(t, content, "esc to close")
	require.NotContains(t, content, inputPrompt+"Ask Hand")
}

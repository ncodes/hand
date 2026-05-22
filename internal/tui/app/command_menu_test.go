package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestCommandMenu_RendersAboveComposerForSlashInput(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.resize()

	content := stripANSI(runModel.View().Content)
	commandIndex := strings.Index(content, "/clear")
	inputIndex := strings.Index(content, inputPrompt+"/")

	require.NotEqual(t, -1, commandIndex)
	require.NotEqual(t, -1, inputIndex)
	require.Less(t, commandIndex, inputIndex)
	require.Contains(t, content, "Clear the transcript")
	require.Contains(t, content, "Copy the transcript")
	require.Contains(t, content, "Show supported commands")
}

func TestCommandMenu_HidesForPromptInput(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("hello")
	runModel.resize()

	content := stripANSI(runModel.View().Content)

	require.NotContains(t, content, "/clear")
	require.NotContains(t, content, "Clear the transcript")
}

func TestCommandMenu_FiltersCommandsByPrefix(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/co")
	runModel.updateCommandMenuForInput(runModel.input.Value())
	runModel.resize()

	menu := stripANSI(runModel.renderCommandMenu())

	require.Contains(t, menu, "/copy")
	require.NotContains(t, menu, "/clear")
	require.NotContains(t, menu, "/help")
	require.Equal(t, 1, runModel.getCommandMenuHeight())
	require.Zero(t, runModel.commandMenuSelected)
}

func TestCommandMenu_HidesWhenPrefixHasNoMatches(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/unknown")
	runModel.updateCommandMenuForInput(runModel.input.Value())
	runModel.resize()

	require.Empty(t, runModel.renderCommandMenu())
	require.Zero(t, runModel.getCommandMenuHeight())
	require.Zero(t, runModel.commandMenuSelected)
	require.Zero(t, runModel.commandMenuOffset)
}

func TestCommandMenu_ConstrainsHeightAndScrolls(t *testing.T) {
	original := slashCommandDefinitions
	defer func() {
		slashCommandDefinitions = original
	}()
	slashCommandDefinitions = make([]slashCommandDefinition, 0, 12)
	for index := 0; index < 12; index++ {
		slashCommandDefinitions = append(slashCommandDefinitions, slashCommandDefinition{
			Name:        fmt.Sprintf("cmd%d", index),
			Description: fmt.Sprintf("Command %d", index),
		})
	}

	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.resize()

	require.Equal(t, maxCommandMenuHeight, runModel.getCommandMenuHeight())
	require.Contains(t, stripANSI(runModel.renderCommandMenu()), "/cmd0")
	require.NotContains(t, stripANSI(runModel.renderCommandMenu()), "/cmd11")

	for range 10 {
		updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		require.Nil(t, cmd)
		runModel = updated.(model)
	}

	menu := stripANSI(runModel.renderCommandMenu())
	require.NotContains(t, menu, "/cmd0")
	require.Contains(t, menu, "/cmd10")
}

func TestCommandMenu_ReducesTranscriptHeightWhenVisible(t *testing.T) {
	runModel := newModel()
	runModel.height = 20
	runModel.input.SetValue("hello")
	runModel.resize()
	withoutMenu := runModel.transcript.Height()

	runModel.input.SetValue("/")
	runModel.resize()

	require.Equal(t, withoutMenu-runModel.getCommandMenuHeight(), runModel.transcript.Height())
}

func TestCommandMenu_RendersRowsAtFullWidth(t *testing.T) {
	row := renderCommandMenuRow(slashCommandDefinition{
		Name:        "clear",
		Description: "Clear the transcript",
	}, false, 42)

	require.Equal(t, 42, lipgloss.Width(stripANSI(row)))
}

func TestCommandMenu_AlignsRowsWithComposerPanel(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.resize()

	lines := strings.Split(stripANSI(runModel.View().Content), "\n")
	commandRow := ""
	composerRow := ""
	for _, line := range lines {
		if strings.Contains(line, "/clear") {
			commandRow = line
		}
		if strings.Contains(line, inputPrompt+"/") {
			composerRow = line
		}
	}

	require.NotEmpty(t, commandRow)
	require.NotEmpty(t, composerRow)
	require.Equal(t, commandMenuLeftPad, textColumn(commandRow, "/clear"))
	require.Equal(t, 0, textColumn(composerRow, "│"))
}

func TestCommandMenu_ArrowKeysMoveHighlight(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.updateCommandMenuForInput(runModel.input.Value())
	runModel.resize()

	require.Equal(t, 0, runModel.commandMenuSelected)

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	require.Equal(t, 1, runModel.commandMenuSelected)

	updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	require.Equal(t, 0, runModel.commandMenuSelected)
}

func TestCommandMenu_MouseHoverMovesHighlight(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.updateCommandMenuForInput(runModel.input.Value())
	runModel.resize()
	layout := runModel.getTUILayout(runModel.input.Height())

	updated, cmd := runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		X: 1,
		Y: layout.Composer.Y + 2,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	require.Equal(t, 2, runModel.commandMenuSelected)
}

func TestCommandMenu_CommandLabelIsNotBold(t *testing.T) {
	row := renderCommandMenuRow(slashCommandDefinition{
		Name:        "clear",
		Description: "Clear the transcript",
	}, false, 42)

	require.NotContains(t, row, "\x1b[1m")
}

func TestCommandMenu_SelectedCommandLabelUsesAccent(t *testing.T) {
	row := renderCommandMenuRow(slashCommandDefinition{
		Name:        "clear",
		Description: "Clear the transcript",
	}, true, 42)

	require.Contains(t, row, "38;5;39")
}

func textColumn(line string, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}

	return lipgloss.Width(line[:index])
}

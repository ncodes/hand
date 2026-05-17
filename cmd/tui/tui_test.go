package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent"
	handmsg "github.com/wandxy/hand/internal/messages"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
)

func TestMain(m *testing.M) {
	original := promptHistoryPath
	promptHistoryPath = func() string {
		return ""
	}
	code := m.Run()
	promptHistoryPath = original
	os.Exit(code)
}

type fakeProgram struct {
	model tea.Model
	err   error
	ran   bool
}

func (program *fakeProgram) Run() (tea.Model, error) {
	program.ran = true
	return program.model, program.err
}

func TestNewCommand_StartsProgram(t *testing.T) {
	originalNewProgram := newProgram
	t.Cleanup(func() {
		newProgram = originalNewProgram
	})

	program := &fakeProgram{}
	newProgram = func(model tea.Model) programRunner {
		program.model = model
		return program
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.NoError(t, err)
	require.True(t, program.ran)
	require.IsType(t, model{}, program.model)
}

func TestNewCommand_ReturnsProgramError(t *testing.T) {
	originalNewProgram := newProgram
	t.Cleanup(func() {
		newProgram = originalNewProgram
	})

	expectedErr := errors.New("terminal unavailable")
	newProgram = func(model tea.Model) programRunner {
		return &fakeProgram{model: model, err: expectedErr}
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.ErrorIs(t, err, expectedErr)
}

func TestModel_ViewRendersShellAreas(t *testing.T) {
	model := newModel()
	view := model.View()
	content := stripANSI(view.Content)

	require.True(t, view.AltScreen)
	require.Contains(t, content, "██████")
	require.Contains(t, content, "/changelogs")
	require.Contains(t, content, inputPrompt+"Ask Hand...")
	require.Contains(t, content, "Ask Hand...")
	require.Contains(t, content, "GPT 5.5")
	require.Contains(t, content, "ready")
}

func TestModel_InitFocusesInput(t *testing.T) {
	runModel := newModel()

	cmd := runModel.Init()

	require.NotNil(t, cmd)
}

func TestModel_InitSchedulesLoadedTransientStatusExpiration(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	runModel := newModel()
	runModel.status.setTransient("loaded")
	cmd := runModel.statusExpireCmd()

	require.NotNil(t, cmd)
}

func TestModel_StatusExpireCmdFallsBackToDefaultWindow(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	runModel := newModel()
	runModel.status.hideAfter = 0
	runModel.status.setTransient("loaded")
	cmd := runModel.statusExpireCmd()

	require.NotNil(t, cmd)
}

func TestModel_StatusExpireCmdReturnsExpirationMessage(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	runModel := newModel()
	runModel.status.hideAfter = time.Nanosecond
	runModel.status.setTransient("loaded")
	cmd := runModel.statusExpireCmd()

	require.NotNil(t, cmd)
	require.Equal(t, statusExpiredMsg{startedAt: now}, cmd())
}

func TestModel_ViewRendersHeaderInfoPanelWhenWide(t *testing.T) {
	runModel := newModel()
	runModel.width = 120
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "Welcome, Kennedy")
	require.Contains(t, content, "Use /changelogs to see what changed")
	require.Contains(t, content, "version: v0.1 alpha")
	require.Contains(t, content, "provider: openrouter")
	require.Contains(t, content, "model: GPT 5.5")
	require.Contains(t, content, "embedding: text-embedding-3-small")
	require.Contains(t, content, "summary: gpt-4o-mini")
	require.NotContains(t, content, "summary: openai/gpt-4o-mini")
}

func TestModel_RenderNoticeBarFillsRow(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	lines := strings.Split(stripANSI(runModel.renderNoticeBar()), "\n")

	require.Len(t, lines, noticeBarHeight)
	require.Contains(t, lines[0], "Welcome, Kennedy")
	require.Contains(t, lines[0], "Use /changelogs to see what changed")
	require.Equal(t, 80, lipgloss.Width(lines[0]))
}

func TestModel_RenderNoticeBarUsesConfiguredColors(t *testing.T) {
	content := newModel().renderNoticeBar()

	require.Contains(t, content, "48;2;41;41;41")
	require.Contains(t, renderNoticeBarLeft(), "38;2;160;160;160")
	require.Contains(t, renderNoticeBarLeft(), "38;2;255;255;255")
	require.Contains(t, renderNoticeBarRight(), "38;2;160;160;160")
	require.Contains(t, renderNoticeBarRight(), "38;2;255;255;255")
}

func TestRenderNoticeBarContent_HidesRightTextWhenTooNarrow(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", "Use /changelogs", 8))

	require.Equal(t, "Welcome", content)
}

func TestRenderNoticeBarContent_FillsWidthWithSpacer(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", "Use /changelogs", 30))

	require.Equal(t, "Welcome        Use /changelogs", content)
	require.Equal(t, 30, lipgloss.Width(content))
}

func TestModel_ViewAlignsHeaderInfoKeys(t *testing.T) {
	runModel := newModel()
	runModel.width = 120
	runModel.resize()
	lines := strings.Split(stripANSI(runModel.renderHeaderInfoPanel()), "\n")

	colonIndex := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		index := strings.Index(line, ":")
		require.NotEqual(t, -1, index)
		if colonIndex == -1 {
			colonIndex = index
			continue
		}
		require.Equal(t, colonIndex, index)
	}
}

func TestModel_ViewSizesHeaderInfoPanelToValues(t *testing.T) {
	runModel := newModel()
	runModel.width = 120
	runModel.resize()
	content := stripANSI(runModel.renderHeaderInfoPanel())

	require.Equal(t, headerInfoKeyWidth+2+lipgloss.Width("text-embedding-3-small"), lipgloss.Width(content))
}

func TestModel_ViewVerticallyCentersHeaderInfoPanel(t *testing.T) {
	panel := alignHeaderInfoPanel("one\ntwo", 4)
	lines := strings.Split(panel, "\n")

	require.Len(t, lines, 4)
	require.Equal(t, "", lines[0])
	require.Equal(t, "one", lines[1])
	require.Equal(t, "two", lines[2])
	require.Equal(t, "", lines[3])
}

func TestGetModelDisplayName_RemovesProviderPrefix(t *testing.T) {
	require.Equal(t, "gpt-4o-mini", getModelDisplayName("openai/gpt-4o-mini"))
	require.Equal(t, "GPT 5.5", getModelDisplayName(" GPT 5.5 "))
}

func TestModel_ViewKeepsBannerFullWhenInfoPanelWouldClipIt(t *testing.T) {
	runModel := newModel()
	runModel.width = lipgloss.Width(handBanner) + headerGapWidth + getHeaderInfoWidth(getHeaderInfoRows(runModel)) - 1
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "░██     ░██░░████████ ███  ░██░░██████")
	require.NotContains(t, content, "provider: openrouter")
}

func TestModel_ViewUsesCompactBannerWhenFullBannerDoesNotFit(t *testing.T) {
	runModel := newModel()
	runModel.width = lipgloss.Width(handBanner) - 1
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "|_||_\\__,_|_||_\\__,_|")
	require.NotContains(t, content, "░██")
}

func TestModel_ViewRendersInputInfoBelowComposer(t *testing.T) {
	runModel := newModel()
	content := stripANSI(runModel.View().Content)
	inputIndex := strings.Index(content, inputPrompt+"Ask Hand...")
	infoIndex := strings.LastIndex(content, "GPT 5.5")

	require.NotEqual(t, -1, inputIndex)
	require.NotEqual(t, -1, infoIndex)
	require.Greater(t, infoIndex, inputIndex)
}

func TestModel_RenderInputInfoMovesContextToRight(t *testing.T) {
	runModel := newModel()
	content := stripANSI(runModel.renderInputInfo())

	require.Contains(t, content, "GPT 5.5")
	require.Contains(t, content, "default session")
	require.Contains(t, content, "60,000 used · 65%")
	require.GreaterOrEqual(t, strings.Count(content, "  "), 1)
	require.Greater(t, strings.Index(content, "60,000 used"), strings.Index(content, "default session"))
}

func TestModel_UpdateResizesTranscriptAndInput(t *testing.T) {
	runModel := newModel()
	updated, cmd := runModel.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	require.Nil(t, cmd)

	resized := updated.(model)
	require.Equal(t, 100, resized.width)
	require.Equal(t, 30, resized.height)
	require.Equal(t, 100, resized.transcript.Width())
	require.LessOrEqual(t, resized.input.Width(), 100)
	require.GreaterOrEqual(t, resized.transcript.Height(), 1)
	require.Equal(t, 1, resized.input.Height())
}

func TestModel_HydrateSessionTimelineReplacesVisibleTranscript(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"stale cell"}
	runModel.transcript.SetContent("stale cell")

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{
		SessionID: "project-a",
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "hello"}},
			{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "hi"}},
		},
		TraceEvents: []agent.SessionTimelineTraceEvent{{
			Event: storage.TraceEvent{
				Type:    trace.EvtToolInvocationStarted,
				Payload: map[string]any{"id": "call_1", "name": "read_file"},
			},
		}},
	})

	content := runModel.transcript.View()
	require.Equal(t, "project-a · hydrated", runModel.status.Text())
	require.Equal(t, []string{"You: hello", "Hand: hi", "Tool started: read_file"}, runModel.messages)
	require.Contains(t, content, "You: hello")
	require.Contains(t, content, "Hand: hi")
	require.Contains(t, content, "Tool started: read_file")
	require.NotContains(t, content, "stale cell")
}

func TestModel_HydrateSessionTimelineShowsEmptySession(t *testing.T) {
	runModel := newModel()

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{SessionID: "empty"})

	require.Equal(t, "empty · hydrated", runModel.status.Text())
	require.Equal(t, []string{"empty has no visible timeline yet."}, runModel.messages)
	require.Contains(t, runModel.transcript.View(), "empty has no visible timeline yet.")
}

func TestModel_HydrateSessionTimelineShowsFallbackForMissingSessionID(t *testing.T) {
	runModel := newModel()

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{})

	require.Equal(t, defaultStatus, runModel.status.Text())
	require.Equal(t, []string{"session has no visible timeline yet."}, runModel.messages)
	require.Contains(t, runModel.transcript.View(), "session has no visible timeline yet.")
}

func TestModel_UpdateIgnoresEsc(t *testing.T) {
	runModel := newModel()
	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	require.Nil(t, cmd)
	require.Equal(t, runModel.status.Text(), updated.(model).status.Text())
}

func TestModel_UpdatePromptsOnFirstCtrlC(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	currentTime = func() time.Time {
		return time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	}

	runModel := newModel()
	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	require.Equal(t, "Press Ctrl-C again to exit", updated.(model).status.Text())
}

func TestModel_UpdateFirstCtrlCTimeoutReturnsExpirationMessage(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	runModel := newModel()
	_, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	msg := cmd()
	require.Equal(t, exitConfirmationExpiredMsg{startedAt: now}, msg)
}

func TestModel_RenderInputInfoShowsCtrlCNoticeOnRightOnly(t *testing.T) {
	runModel := newModel()
	runModel.exitAt = time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	runModel.status.setTransient("Press Ctrl-C again to exit")
	content := stripANSI(runModel.renderInputInfo())

	require.Contains(t, content, "Press Ctrl-C again to exit")
	require.NotContains(t, content, "GPT 5.5")
	require.NotContains(t, content, "60,000 used")
	require.Equal(t, 0, strings.Index(strings.TrimLeft(content, " "), "Press Ctrl-C again to exit"))
}

func TestModel_UpdateQuitsOnSecondQuickCtrlC(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	times := []time.Time{
		time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 16, 9, 0, 1, 0, time.UTC),
	}
	currentTime = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	runModel := newModel()
	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)

	_, cmd = updated.(model).Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, cmd())
}

func TestModel_UpdateDoesNotQuitOnSlowSecondCtrlC(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	times := []time.Time{
		time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 16, 9, 0, 3, 0, time.UTC),
	}
	currentTime = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	runModel := newModel()
	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)

	updated, cmd = updated.(model).Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)
	require.Equal(t, "Press Ctrl-C again to exit", updated.(model).status.Text())
}

func TestModel_UpdateClearsExpiredCtrlCNotice(t *testing.T) {
	startedAt := time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	runModel := newModel()
	runModel.exitAt = startedAt
	runModel.status.text = "Press Ctrl-C again to exit"
	runModel.status.startedAt = startedAt

	updated, cmd := runModel.Update(exitConfirmationExpiredMsg{startedAt: startedAt})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.exitAt.IsZero())
	require.Equal(t, defaultStatus, runModel.status.Text())
}

func TestModel_UpdateIgnoresStaleCtrlCNoticeTimeout(t *testing.T) {
	runModel := newModel()
	runModel.exitAt = time.Date(2026, 5, 16, 9, 0, 1, 0, time.UTC)
	runModel.status.text = "Press Ctrl-C again to exit"
	runModel.status.startedAt = runModel.exitAt

	updated, cmd := runModel.Update(exitConfirmationExpiredMsg{
		startedAt: time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC),
	})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.exitAt.IsZero())
	require.Equal(t, "Press Ctrl-C again to exit", runModel.status.Text())
}

func TestModel_UpdateKeepsPrintableTextInPrompt(t *testing.T) {
	runModel := newModel()
	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))

	if cmd != nil {
		_, isQuit := cmd().(tea.QuitMsg)
		require.False(t, isQuit)
	}
	require.Equal(t, "q", updated.(model).input.Value())
}

func TestModel_UpdateAppendsPromptOnEnter(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("Summarize tests")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.Nil(t, cmd)

	mainModel := updated.(model)
	require.Empty(t, mainModel.input.Value())
	require.Equal(t, []string{"You: Summarize tests"}, mainModel.messages)

	content := stripANSI(mainModel.View().Content)
	require.Contains(t, content, "██████")
	require.Contains(t, content, "You: Summarize tests")
	require.Contains(t, content, "60,000 used")
}

func TestModel_UpdateHandlesClearCommand(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"You: stale", "Hand: stale"}
	runModel.live = "Hand: live"
	runModel.stream.Add("live")
	runModel.input.SetValue("/clear")
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.messages)
	require.Empty(t, runModel.live)
	require.Empty(t, runModel.input.Value())
	require.Empty(t, runModel.stream.Render())
	require.Equal(t, "transcript cleared", runModel.status.Text())
	require.Empty(t, strings.TrimSpace(stripANSI(runModel.transcript.View())))

	updated, cmd = runModel.Update(statusExpiredMsg{startedAt: runModel.status.startedAt})
	require.Nil(t, cmd)
	require.Equal(t, defaultStatus, updated.(model).status.Text())
}

func TestModel_UpdateHandlesHelpCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/help")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"Commands: /clear, /help"}, runModel.messages)
	require.Empty(t, runModel.input.Value())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Commands: /clear, /help")
}

func TestModel_UpdateReportsUnknownCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/missing now")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.messages)
	require.Equal(t, "unknown command: /missing", runModel.status.Text())
	require.Empty(t, runModel.input.Value())
}

func TestModel_UpdateReportsEmptyCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.messages)
	require.Equal(t, "empty command", runModel.status.Text())
	require.Empty(t, runModel.input.Value())
}

func TestModel_UpdateBlocksLocalCommandWhenShellIsDisabled(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("!ls -la")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "local commands are disabled", runModel.status.Text())
	require.Equal(t, []string{"Local command blocked: !ls -la"}, runModel.messages)
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
	require.Equal(t, []string{"Local command queued: !pwd"}, runModel.messages)
	require.Empty(t, runModel.input.Value())
}

func TestModel_UpdatePastesLargeMultilineContent(t *testing.T) {
	runModel := newModel()
	paste := strings.Join([]string{
		"first",
		"second",
		strings.Repeat("x", getInputInnerWidth(runModel.width)+1),
	}, "\n")

	updated, cmd := runModel.Update(tea.PasteMsg{Content: paste})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, paste, runModel.input.Value())
	require.GreaterOrEqual(t, runModel.input.Height(), 3)
}

func TestModel_UpdateNavigatesPromptHistory(t *testing.T) {
	runModel := newModel()
	for _, prompt := range []string{"first prompt", "second prompt"} {
		runModel.input.SetValue(prompt)
		updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		require.Nil(t, cmd)
		runModel = updated.(model)
	}
	runModel.input.SetValue("draft")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "second prompt", runModel.input.Value())

	updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "first prompt", runModel.input.Value())

	updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "second prompt", runModel.input.Value())

	updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "draft", runModel.input.Value())
}

func TestModel_UpdateDeduplicatesConsecutivePromptHistory(t *testing.T) {
	runModel := newModel()
	for range 2 {
		runModel.input.SetValue("repeat")
		updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		require.Nil(t, cmd)
		runModel = updated.(model)
	}

	require.Equal(t, []string{"repeat"}, runModel.history)
	require.Equal(t, 1, runModel.historyAt)
}

func TestModel_AddPromptHistoryIgnoresBlankValues(t *testing.T) {
	runModel := newModel()

	runModel.addPromptHistory(" \n\t ")

	require.Empty(t, runModel.history)
	require.Zero(t, runModel.historyAt)
}

func TestModel_UpdateKeepsHistoryStableWhenEmpty(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("draft")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "draft", runModel.input.Value())
	require.Empty(t, runModel.history)
}

func TestModel_UpdateKeepsHistoryStableAtNewestEntry(t *testing.T) {
	runModel := newModel()
	runModel.history = []string{"first"}
	runModel.historyAt = len(runModel.history)
	runModel.input.SetValue("draft")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "draft", runModel.input.Value())
	require.Equal(t, len(runModel.history), runModel.historyAt)
}

func TestModel_UpdateLetsMultilineInputUseArrowKeys(t *testing.T) {
	runModel := newModel()
	runModel.history = []string{"previous prompt"}
	runModel.historyAt = len(runModel.history)
	runModel.input.SetValue("first\nsecond")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	if cmd != nil {
		cmd()
	}
	require.Equal(t, "first\nsecond", updated.(model).input.Value())
}

func TestModel_UpdatePreservesLiveAssistantCellDuringStreaming(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"You: hello"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "first line\npartial"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"You: hello"}, runModel.messages)
	require.Equal(t, "Hand: first line\npartial", runModel.live)
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "You: hello")
	require.Contains(t, content, "Hand: first line")
	require.Contains(t, content, "partial")
}

func TestModel_UpdateConvertsLiveAssistantCellToHistoryAtCompletion(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"You: hello"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "first line\npartial"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"You: hello", "Hand: first line\npartial"}, runModel.messages)
	require.Equal(t, "", runModel.stream.Render())
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "Hand: first line")
	require.Contains(t, content, "partial")
}

func TestModel_UpdateStreamedRenderMatchesCommittedAssistantText(t *testing.T) {
	runModel := newModel()
	deltas := []string{"# Title\n", "\n- one", "\n- two\n", "tail\n\n"}
	for _, delta := range deltas {
		updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: delta})
		require.Nil(t, cmd)
		runModel = updated.(model)
	}
	live := runModel.live

	updated, cmd := runModel.Update(assistantResponseCompletedMsg{})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{live}, runModel.messages)
	require.Empty(t, runModel.live)
}

func TestModel_UpdateUsesFinalAssistantTextAtCompletion(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"You: hello"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "draft"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{Text: "final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"You: hello", "Hand: final"}, runModel.messages)
	require.NotContains(t, stripANSI(runModel.transcript.View()), "draft")
}

func TestModel_UpdatePreservesFinalAssistantWhitespace(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "draft"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{Text: "final\n\n"})

	require.Nil(t, cmd)
	require.Equal(t, []string{"Hand: final\n\n"}, updated.(model).messages)
}

func TestModel_UpdateIgnoresEmptyAssistantDelta(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{})

	require.Nil(t, cmd)
	require.Empty(t, updated.(model).live)
	require.Empty(t, updated.(model).messages)
}

func TestModel_UpdateClearsEmptyAssistantCompletion(t *testing.T) {
	runModel := newModel()
	runModel.live = "Hand: draft"
	runModel.stream.Add("   ")

	updated, cmd := runModel.Update(assistantResponseCompletedMsg{})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Empty(t, runModel.messages)
	require.Empty(t, runModel.stream.Render())
}

func TestAssistantTranscriptCell_IgnoresBlankText(t *testing.T) {
	require.Empty(t, assistantTranscriptCell(" \n\t "))
}

func TestModel_UpdateInsertsPromptNewlineOnShiftEnter(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("first line")

	updated, cmd := runModel.Update(tea.KeyPressMsg{
		Code: tea.KeyEnter,
		Mod:  tea.ModShift,
	})

	if cmd != nil {
		cmd()
	}
	runModel = updated.(model)
	require.Equal(t, "first line\n", runModel.input.Value())
	require.Equal(t, 2, runModel.input.Height())
	require.Zero(t, runModel.input.ScrollYOffset())
	require.Contains(t, stripANSI(runModel.input.View()), "first line")
	require.Equal(t, 1, strings.Count(stripANSI(runModel.input.View()), strings.TrimSpace(inputPrompt)))
	require.Empty(t, runModel.messages)
}

func TestModel_UpdateDeletesCurrentPromptLineOnCommandDelete(t *testing.T) {
	tests := []struct {
		name string
		key  tea.Key
	}{
		{name: "command_backspace", key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModSuper}},
		{name: "command_delete", key: tea.Key{Code: tea.KeyDelete, Mod: tea.ModSuper}},
		{name: "meta_backspace", key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModMeta}},
		{name: "ctrl_backspace", key: tea.Key{Code: tea.KeyBackspace, Mod: tea.ModCtrl}},
		{name: "ctrl_u", key: tea.Key{Code: 'u', Mod: tea.ModCtrl}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runModel := newModel()
			runModel.input.SetValue("first line\nsecond line")

			updated, cmd := runModel.Update(tea.KeyPressMsg(tt.key))
			if cmd != nil {
				cmd()
			}

			runModel = updated.(model)
			require.Equal(t, "first line\n", runModel.input.Value())
			require.Empty(t, runModel.messages)
		})
	}
}

func TestModel_UpdateGrowsPromptForWrappedText(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue(strings.Repeat("a", getInputInnerWidth(runModel.width)+1))
	runModel.resize()

	require.Equal(t, 2, runModel.input.Height())
}

func TestModel_UpdateShowsAllPromptRowsWhenSpaceAllows(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue(strings.Join([]string{
		"one",
		"two",
		"three",
		"four",
		"five",
	}, "\n"))
	runModel.resize()

	require.Equal(t, 5, runModel.input.Height())
}

func TestModel_UpdateLimitsPromptRowsToAvailableHeight(t *testing.T) {
	runModel := newModel()
	runModel.height = 6
	runModel.input.SetValue(strings.Join([]string{
		"one",
		"two",
		"three",
		"four",
		"five",
	}, "\n"))
	runModel.resize()

	require.Equal(t, 1, runModel.input.Height())
	require.Equal(t, 1, runModel.transcript.Height())
}

func TestModel_UpdateIgnoresEmptyEnter(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.Nil(t, cmd)

	runModel = updated.(model)
	require.Empty(t, runModel.messages)
}

func TestModel_UpdateClampsTinyWindowSize(t *testing.T) {
	runModel := newModel()
	updated, cmd := runModel.Update(tea.WindowSizeMsg{})
	require.Nil(t, cmd)

	resized := updated.(model)
	require.Equal(t, 1, resized.width)
	require.Equal(t, 1, resized.height)
	require.Equal(t, 1, resized.transcript.Width())
	require.GreaterOrEqual(t, resized.transcript.Height(), 1)
	require.GreaterOrEqual(t, resized.input.Height(), 1)
}

func stripANSI(value string) string {
	return ansi.Strip(value)
}

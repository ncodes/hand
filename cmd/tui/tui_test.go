package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/agent"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
)

func TestMain(m *testing.M) {
	original := promptHistoryPath
	_ = original()
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
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		newProgram = originalNewProgram
		loadCommandModel = originalLoadCommandModel
	})

	program := &fakeProgram{}
	newProgram = func(model tea.Model) programRunner {
		program.model = model
		return program
	}
	loadCommandModel = func(context.Context, *cli.Command) (model, func(), error) {
		return newModel(), func() {}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.NoError(t, err)
	require.True(t, program.ran)
	require.IsType(t, model{}, program.model)
}

func TestNewCommand_ReturnsProgramError(t *testing.T) {
	originalNewProgram := newProgram
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		newProgram = originalNewProgram
		loadCommandModel = originalLoadCommandModel
	})

	expectedErr := errors.New("terminal unavailable")
	newProgram = func(model tea.Model) programRunner {
		return &fakeProgram{model: model, err: expectedErr}
	}
	loadCommandModel = func(context.Context, *cli.Command) (model, func(), error) {
		return newModel(), func() {}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.ErrorIs(t, err, expectedErr)
}

func TestNewCommand_ReturnsModelLoadError(t *testing.T) {
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		loadCommandModel = originalLoadCommandModel
	})

	expectedErr := errors.New("rpc unavailable")
	loadCommandModel = func(context.Context, *cli.Command) (model, func(), error) {
		return model{}, nil, expectedErr
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.ErrorIs(t, err, expectedErr)
}

func TestDefaultTUIFactories_ConstructProgramAndClient(t *testing.T) {
	runner := newProgram(newModel())
	require.NotNil(t, runner)

	client, err := newTUIChatClient(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 1},
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestLoadTUICommandModel_UsesConfiguredRPCClientAndCleanup(t *testing.T) {
	originalNewTUIChatClient := newTUIChatClient
	activeProfile := profile.Active()
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
		profile.SetActive(activeProfile)
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(profile.EnvName, "")
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
rpc:
  address: 127.0.0.2
  port: 45678
models:
  verify: false
tui:
  thinkingComposer: false
`), 0o600))

	client := &fakeTUIChatClient{}
	var gotRPC config.RPCConfig
	newTUIChatClient = func(_ context.Context, cfg *config.Config) (tuiClient, error) {
		gotRPC = cfg.RPC
		return client, nil
	}

	var runModel model
	var cleanup func()
	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		var err error
		runModel, cleanup, err = loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.NoError(t, err)
	require.Equal(t, config.RPCConfig{Address: "127.0.0.2", Port: 45678}, gotRPC)
	require.Same(t, client, runModel.chatClient)
	require.False(t, runModel.thinkingComposerEnabled)
	require.NotNil(t, cleanup)
	cleanup()
	require.True(t, client.closed)
}

func TestLoadTUICommandModel_ReturnsConfigLoadError(t *testing.T) {
	activeProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(activeProfile)
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(profile.EnvName, "")
	configPath := filepath.Join(home, "bad-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("{"), 0o600))

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--config", configPath})

	require.Error(t, err)
	require.ErrorContains(t, err, "yaml")
}

func TestLoadTUICommandModel_ReturnsRPCResolutionError(t *testing.T) {
	activeProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(activeProfile)
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(profile.EnvName, "")
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "runtime.json"), []byte("{"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
models:
  verify: false
`), 0o600))

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.Error(t, err)
	require.ErrorContains(t, err, "parse runtime metadata")
}

func TestLoadTUICommandModel_ReturnsClientCreationError(t *testing.T) {
	originalNewTUIChatClient := newTUIChatClient
	activeProfile := profile.Active()
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
		profile.SetActive(activeProfile)
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(profile.EnvName, "")
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
rpc:
  address: 127.0.0.2
  port: 45678
models:
  verify: false
`), 0o600))

	expectedErr := errors.New("client unavailable")
	newTUIChatClient = func(context.Context, *config.Config) (tuiClient, error) {
		return nil, expectedErr
	}

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.ErrorIs(t, err, expectedErr)
}

func TestModel_ViewRendersShellAreas(t *testing.T) {
	model := newModel()
	view := model.View()
	content := stripANSI(view.Content)

	require.True(t, view.AltScreen)
	require.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
	require.Contains(t, view.Content, "48;2;41;41;41")
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

func TestNewModelWithClientContextDefaultsNilContext(t *testing.T) {
	runModel := newModelWithClientContext(nil, nil)

	require.NotNil(t, runModel.chatCtx)
}

func TestModel_InitLoadsExistingSessionTimeline(t *testing.T) {
	client := &fakeTUIChatClient{
		timeline: rpcclient.SessionTimeline{
			SessionID: "default",
			Messages: []agent.SessionTimelineMessage{{
				Message: handmsg.Message{Role: handmsg.RoleUser, Content: "older prompt"},
			}},
		},
	}
	runModel := newModelWithClient(client)

	cmd := runModel.Init()

	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)

	var loaded sessionTimelineLoadedMsg
	for _, child := range batch {
		if msg, ok := child().(sessionTimelineLoadedMsg); ok {
			loaded = msg
			break
		}
	}

	require.Equal(t, "default", loaded.Timeline.SessionID)
	require.Len(t, loaded.Timeline.Messages, 1)
	require.Equal(t, 1, client.timelineCalls)
}

func TestLoadSessionTimelineCmdReturnsLoadFailure(t *testing.T) {
	expectedErr := errors.New("timeline unavailable")
	client := &fakeTUIChatClient{timelineErr: expectedErr}

	cmd := loadSessionTimelineCmd(context.Background(), client)

	require.NotNil(t, cmd)
	require.Equal(t, sessionTimelineLoadFailedMsg{Err: expectedErr}, cmd())
}

func TestModel_UpdateHydratesLoadedSessionTimeline(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(sessionTimelineLoadedMsg{
		Timeline: rpcclient.SessionTimeline{
			SessionID: "default",
			Messages: []agent.SessionTimelineMessage{{
				Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "older answer"},
			}},
		},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"Hand: older answer"}, runModel.messages)
	require.Contains(t, stripANSI(runModel.transcript.View()), "older answer")
	require.Equal(t, "default · hydrated", runModel.status.Text())
}

func TestModel_UpdateReportsTimelineLoadFailure(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(sessionTimelineLoadFailedMsg{Err: errors.New("timeline unavailable")})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "session timeline unavailable", runModel.status.Text())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Welcome to Hand TUI")
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

func TestRenderNoticeBarContent_HidesRightTextWhenMissing(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", " ", 80))

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

func TestGetHandBannerColor_UsesLastColorForOutOfRangeIndex(t *testing.T) {
	require.Equal(t, handBannerColors[len(handBannerColors)-1], getHandBannerColor(-1))
	require.Equal(t, handBannerColors[len(handBannerColors)-1], getHandBannerColor(len(handBannerColors)))
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

func TestModel_ViewRendersBottomStatusPanelBelowComposer(t *testing.T) {
	runModel := newModel()
	content := stripANSI(runModel.View().Content)
	inputIndex := strings.Index(content, inputPrompt+"Ask Hand...")
	infoIndex := strings.LastIndex(content, "GPT 5.5")

	require.NotEqual(t, -1, inputIndex)
	require.NotEqual(t, -1, infoIndex)
	require.Greater(t, infoIndex, inputIndex)
}

func TestModel_RenderInputUsesCompleteComposerFrame(t *testing.T) {
	runModel := newModel()
	runModel.width = 40
	runModel.resize()

	lines := strings.Split(stripANSI(runModel.renderInput()), "\n")

	require.GreaterOrEqual(t, len(lines), 3)
	require.True(t, strings.HasPrefix(lines[0], "╭"))
	require.True(t, strings.HasSuffix(strings.TrimRight(lines[0], " "), "╮"))
	require.True(t, strings.HasPrefix(lines[1], "│"))
	require.True(t, strings.HasSuffix(strings.TrimRight(lines[1], " "), "│"))
	require.Contains(t, lines[1], inputPrompt+"Ask Hand...")
	require.True(t, strings.HasPrefix(lines[2], "╰"))
	require.True(t, strings.HasSuffix(strings.TrimRight(lines[2], " "), "╯"))
}

func TestModel_RenderBottomStatusPanelMovesContextToRight(t *testing.T) {
	runModel := newModel()
	content := stripANSI(runModel.renderBottomStatusPanel())

	require.True(t, strings.HasPrefix(content, " "))
	require.Equal(t, runModel.width, lipgloss.Width(content))
	require.Contains(t, content, "GPT 5.5")
	require.Contains(t, content, "default session")
	require.Contains(t, content, "60,000 used · 65%")
	require.GreaterOrEqual(t, strings.Count(content, "  "), 1)
	require.Greater(t, strings.Index(content, "60,000 used"), strings.Index(content, "default session"))
}

func TestModel_RenderBottomStatusPanelShowsThinkingBeforeModel(t *testing.T) {
	runModel := newModel()
	runModel.responding = true

	content := stripANSI(runModel.renderBottomStatusPanel())

	require.Contains(t, content, "Thinking")
	require.Contains(t, content, "GPT 5.5")
	require.Less(t, strings.Index(content, "Thinking"), strings.Index(content, "GPT 5.5"))
}

func TestModel_RenderBottomStatusPanelHidesThinkingWhenNotThinking(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.live = "Hand: hello"

	content := stripANSI(runModel.renderBottomStatusPanel())

	require.NotContains(t, content, "Thinking")
}

func TestModel_RenderBottomStatusPanelShowsThinkingWhenComposerAnimationDisabled(t *testing.T) {
	disabled := false
	runModel := newModelWithClientContextAndConfig(
		context.Background(),
		nil,
		&config.Config{TUI: config.TUIConfig{ThinkingComposer: &disabled}},
	)
	runModel.responding = true

	content := stripANSI(runModel.renderBottomStatusPanel())

	require.False(t, runModel.isThinkingComposerVisible())
	require.True(t, runModel.isModelThinking())
	require.Contains(t, content, "Thinking")
}

func TestModel_RenderBottomStatusPanelAnimatesThinkingStatus(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.thinkingComposerFrame = 0
	first := runModel.renderBottomStatusPanel()

	runModel.thinkingComposerFrame = 1
	second := runModel.renderBottomStatusPanel()

	require.Contains(t, stripANSI(first), "Thinking")
	require.Contains(t, stripANSI(second), "Thinking")
	require.NotEqual(t, first, second)
}

func TestGetThinkingStatusColor_UsesGrayBaseAndThreeCharacterShimmer(t *testing.T) {
	require.Equal(t, thinkingStatusShimmerColor, getThinkingStatusColor(0, 0, len("Thinking")))
	require.Equal(t, thinkingStatusEdgeColor, getThinkingStatusColor(1, 0, len("Thinking")))
	require.Equal(t, thinkingStatusEdgeColor, getThinkingStatusColor(len("Thinking")-1, 0, len("Thinking")))
	require.Equal(t, thinkingStatusBaseColor, getThinkingStatusColor(2, 0, len("Thinking")))
	require.Equal(t, thinkingStatusShimmerColor, getThinkingStatusColor(1, 1, len("Thinking")))
	require.Equal(t, thinkingStatusShimmerColor, getThinkingStatusColor(len("Thinking")-1, -1, len("Thinking")))
	require.Equal(t, thinkingStatusBaseColor, getThinkingStatusColor(0, 0, 0))
}

func TestModel_RenderBottomStatusPanelKeepsMutedCellsWhenThinking(t *testing.T) {
	runModel := newModel()
	runModel.responding = true

	content := runModel.renderBottomStatusPanel()

	require.Contains(t, content, renderBottomStatusMutedCell("GPT 5.5"))
	require.Contains(t, content, renderBottomStatusMutedCell(defaultStatus))
}

func TestGetPanelHorizontalPadding_DisablesPaddingWhenNarrow(t *testing.T) {
	require.Equal(t, 0, getPanelHorizontalPadding(2))
	require.Equal(t, panelHorizontalPadding, getPanelHorizontalPadding(3))
}

func TestJoinBottomStatusPanelSegments_HandlesEmptySingleAndNarrowValues(t *testing.T) {
	require.Empty(t, joinBottomStatusPanelSegments([]string{" ", ""}, 20))
	require.Equal(t, "ready", joinBottomStatusPanelSegments([]string{"ready"}, 20))
	require.Equal(t, "model · status", joinBottomStatusPanelSegments([]string{"model", "status"}, 5))
}

func TestSpaceBetweenBottomStatusPanel_HandlesMissingAndNarrowSides(t *testing.T) {
	require.Equal(t, "right", spaceBetweenBottomStatusPanel("", "right", 20))
	require.Equal(t, "left · right", stripANSI(spaceBetweenBottomStatusPanel("left", "right", 1)))
}

func TestCompactTranscriptSelectionBlankLines_CollapsesVisualPaddingRuns(t *testing.T) {
	require.Equal(t,
		"❯ first\n\nHand: second",
		compactTranscriptSelectionBlankLines("❯ first\n\n\nHand: second"),
	)
	require.Equal(t,
		"❯ first\n\nHand: second",
		compactTranscriptSelectionBlankLines("❯ first\n"+strings.Repeat("▄", 40)+"\n"+strings.Repeat("▀", 40)+"\n\nHand: second"),
	)
}

func TestModel_UpdateResizesTranscriptAndInput(t *testing.T) {
	runModel := newModel()
	updated, cmd := runModel.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	require.Nil(t, cmd)

	resized := updated.(model)
	require.Equal(t, 100, resized.width)
	require.Equal(t, 30, resized.height)
	require.Equal(t, getPanelContentWidth(100), resized.transcript.Width())
	require.LessOrEqual(t, resized.input.Width(), 100)
	require.GreaterOrEqual(t, resized.transcript.Height(), 1)
	require.Equal(t, 1, resized.input.Height())
}

func TestModel_UpdateScrollsTranscriptWithPagingKeys(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	require.Greater(t, bottomOffset, 0)

	updated, cmd := runModel.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Less(t, runModel.transcript.YOffset(), bottomOffset)

	updated, cmd = runModel.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Greater(t, runModel.transcript.YOffset(), 0)

	updated, cmd = runModel.Update(tea.KeyPressMsg{Code: tea.KeyHome})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Zero(t, runModel.transcript.YOffset())

	updated, cmd = runModel.Update(tea.KeyPressMsg{Code: tea.KeyEnd})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, bottomOffset, runModel.transcript.YOffset())
}

func TestModel_UpdateScrollsHeaderWithTranscript(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	require.Contains(t, stripANSI(runModel.transcript.View()), "Welcome, Kennedy")

	updated, cmd := runModel.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Welcome, Kennedy")
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "Welcome, Kennedy")
}

func TestModel_UpdateScrollsTranscriptWithMouseWheel(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	require.Greater(t, bottomOffset, 0)

	updated, cmd := runModel.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Less(t, runModel.transcript.YOffset(), bottomOffset)
}

func TestModel_HydrateSessionTimelineReplacesVisibleTranscript(t *testing.T) {
	runModel := newModel()
	runModel.height = 14
	runModel.resize()
	runModel.messages = []string{"stale cell"}
	runModel.transcript.SetContent("stale cell")

	messages := make([]agent.SessionTimelineMessage, 0, 20)
	for index := 0; index < 18; index++ {
		messages = append(messages, agent.SessionTimelineMessage{
			Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: fmt.Sprintf("older %02d", index)},
		})
	}
	messages = append(messages,
		agent.SessionTimelineMessage{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "hello"}},
		agent.SessionTimelineMessage{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "hi"}},
	)

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{
		SessionID: "project-a",
		Messages:  messages,
		TraceEvents: []agent.SessionTimelineTraceEvent{{
			Event: storage.TraceEvent{
				Type:    trace.EvtToolInvocationStarted,
				Payload: map[string]any{"id": "call_1", "name": "read_file"},
			},
		}},
	})

	content := stripANSI(runModel.transcript.View())
	require.Equal(t, "project-a · hydrated", runModel.status.Text())
	require.Equal(t, "You: hello", runModel.messages[len(runModel.messages)-3])
	require.Equal(t, "Hand: hi", runModel.messages[len(runModel.messages)-2])
	require.Equal(t, toolOperationTranscriptCell("call_1", "read_file", ""), runModel.messages[len(runModel.messages)-1])
	require.Contains(t, content, "❯ hello")
	require.Contains(t, content, "Hand: hi")
	require.Contains(t, content, "● Read")
	require.Contains(t, content, "└ read_file")
	require.NotContains(t, content, "older 00")
	require.NotContains(t, content, "stale cell")
	require.True(t, runModel.transcript.AtBottom())
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

func TestModel_RenderBottomStatusPanelShowsCtrlCNoticeOnRightOnly(t *testing.T) {
	runModel := newModel()
	runModel.exitAt = time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	runModel.status.setTransient("Press Ctrl-C again to exit")
	content := stripANSI(runModel.renderBottomStatusPanel())

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
	require.Contains(t, content, "❯ Summarize tests")
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
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "Welcome, Kennedy")
	require.NotContains(t, content, "You: stale")
	require.NotContains(t, content, "Hand: live")

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
	require.Equal(t, []string{"Commands: /clear, /copy, /help"}, runModel.messages)
	require.Empty(t, runModel.input.Value())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Commands: /clear, /copy, /help")
}

func TestModel_UpdateCopiesTranscriptToClipboard(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.messages = []string{"You: hello", "Hand: hi"}
	runModel.setTranscriptContent()
	runModel.input.SetValue("/copy")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "You: hello\n\nHand: hi", copied)
	require.Equal(t, "transcript copied", runModel.status.Text())
	require.Empty(t, runModel.input.Value())
}

func TestModel_UpdateCopiesTranscriptWithShortcut(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.messages = []string{"Hand: shortcut"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "Hand: shortcut", copied)
	require.Equal(t, "transcript copied", runModel.status.Text())
}

func TestModel_CopyTranscriptReportsEmptyTranscript(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent(" \n\t ")

	cmd := runModel.copyTranscript()

	require.NotNil(t, cmd)
	require.Equal(t, "transcript is empty", runModel.status.Text())
}

func TestModel_TranscriptTextIncludesLiveAssistantCell(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"You: hello"}
	runModel.live = "Hand: streaming"

	require.Equal(t, "You: hello\n\nHand: streaming", runModel.transcriptText())
}

func TestModel_TranscriptTextFallsBackToViewportContent(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent("  saved viewport  ")

	require.Equal(t, "saved viewport", runModel.transcriptText())
}

func TestModel_UpdateReportsClipboardFailure(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	writeClipboard = func(string) error {
		return errors.New("clipboard unavailable")
	}
	runModel := newModel()
	runModel.messages = []string{"Hand: hi"}
	runModel.setTranscriptContent()
	runModel.input.SetValue("/copy")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	require.Equal(t, "copy failed", updated.(model).status.Text())
}

func TestModel_UpdateSelectsTranscriptTextWithMouseAndCopiesOnRelease(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.height = 40
	runModel.resize()
	runModel.messages = []string{"You: first", "Hand: second", toolOperationTranscriptCell("", "read_file", "")}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	firstRow := getTranscriptContentRow(t, runModel, "❯ first")
	secondRow := getTranscriptContentRow(t, runModel, "Hand: second")
	require.GreaterOrEqual(t, runModel.transcript.Height(), 3)

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      firstRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("Hand: second"),
		Y:      secondRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.transcript.View(), "48;2;21;21;21")
	require.Contains(t, runModel.transcript.View(), "38;5;83")

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("Hand: second"),
		Y:      secondRow,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.selection.dragging)
	require.True(t, runModel.selection.active)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.transcript.View(), "48;2;21;21;21")
	require.Contains(t, runModel.transcript.View(), "38;5;83")
	require.Equal(t, strings.Join([]string{
		"❯ first",
		"",
		"Hand: second",
	}, "\n"), trimTrailingLineSpaces(copied))
	require.Equal(t, defaultStatus, runModel.status.Text())
}

func TestModel_UpdateSelectsTranscriptTextCharacterByCharacter(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	var copied string
	writeClipboard = func(text string) error {
		copied = text
		return nil
	}
	runModel := newModel()
	runModel.height = 40
	runModel.resize()
	runModel.messages = []string{"Hand: second"}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	row := getTranscriptContentRow(t, runModel, "Hand: second")

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("Hand: "),
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("Hand: sec"),
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("Hand: sec"),
		Y:      row,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.selection.dragging)
	require.True(t, runModel.selection.active)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Equal(t, "sec", runModel.selectedTranscriptText())
	require.Equal(t, "sec", copied)
	require.Equal(t, defaultStatus, runModel.status.Text())
}

func TestModel_UpdateIgnoresNonLeftMouseSelectionStart(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"Hand: first"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseRight,
		Y:      runModel.getTranscriptTop(),
	}))

	require.Nil(t, cmd)
	require.False(t, updated.(model).selection.active)
}

func TestModel_UpdateIgnoresSelectionMotionAndReleaseWithoutDrag(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"Hand: first"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      runModel.getTranscriptTop(),
	}))

	require.Nil(t, cmd)
	require.False(t, updated.(model).selection.active)

	updated, cmd = updated.(model).Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      runModel.getTranscriptTop(),
	}))

	require.Nil(t, cmd)
	require.False(t, updated.(model).selection.active)
}

func TestModel_UpdateKeepsSelectionWhenDraggingOutsideTranscript(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"Hand: first"}
	runModel.setTranscriptContent()
	top := runModel.getTranscriptTop()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      len("Hand"),
		Y:      top,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	start := runModel.selection.start

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      len("Hand: first"),
		Y:      top + runModel.transcript.Height(),
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)
	require.Equal(t, start, runModel.selection.end)
}

func TestModel_UpdateDoesNotCopyBlankMouseSelection(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	writeClipboard = func(string) error {
		t.Fatal("clipboard should not be called for blank selections")
		return nil
	}
	runModel := newModel()
	runModel.messages = []string{"   "}
	runModel.transcript.SetContent("   ")
	top := runModel.getTranscriptTop()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      top,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      3,
		Y:      top,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.selection.dragging)
	require.False(t, runModel.selection.active)
	require.NotContains(t, runModel.transcript.View(), "\x1b[7m")
}

func TestModel_UpdateReportsMouseSelectionCopyFailure(t *testing.T) {
	originalWriteClipboard := writeClipboard
	t.Cleanup(func() {
		writeClipboard = originalWriteClipboard
	})
	writeClipboard = func(string) error {
		return errors.New("clipboard unavailable")
	}
	runModel := newModel()
	runModel.messages = []string{"Hand: first"}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	row := getTranscriptContentRow(t, runModel, "Hand: first")

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      len("Hand"),
		Y:      row,
	}))

	require.NotNil(t, cmd)
	require.Equal(t, "copy failed", updated.(model).status.Text())
}

func TestModel_UpdateIgnoresMouseSelectionOutsideTranscript(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"Hand: first"}
	runModel.setTranscriptContent()
	runModel.resize()
	belowTranscript := runModel.getTranscriptTop() + runModel.transcript.Height()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      belowTranscript,
	}))

	require.Nil(t, cmd)
	require.False(t, updated.(model).selection.active)
}

func TestModel_TranscriptSelectionPointFromVisualLineHandlesPlainLines(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SoftWrap = false
	runModel.transcript.SetContent("one\ntwo")

	point, ok := runModel.transcriptSelectionPointFromVisualLine(1, 2)

	require.True(t, ok)
	require.Equal(t, transcriptSelectionPoint{line: 1, offset: len("one\n") + len("tw")}, point)

	_, ok = runModel.transcriptSelectionPointFromVisualLine(2, 0)
	require.False(t, ok)
}

func TestModel_TranscriptSelectionPointFromVisualLineRejectsInvalidRows(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent("one")

	_, ok := runModel.transcriptSelectionPointFromVisualLine(-1, 0)
	require.False(t, ok)

	_, ok = runModel.transcriptSelectionPointFromVisualLine(10, 0)
	require.False(t, ok)
}

func TestModel_TranscriptSelectionPointFromMouseMapsWrappedVisualRowsToContentLine(t *testing.T) {
	runModel := newModel()
	runModel.width = 24
	runModel.height = 40
	runModel.resize()
	first := "Hand: " + strings.Repeat("wrapped ", 6)
	runModel.transcript.SetContent(first + "\nYou: next")
	runModel.transcript.GotoTop()

	point, ok := runModel.transcriptSelectionPointFromMouse(tea.Mouse{
		X: 3,
		Y: runModel.getTranscriptTop() + 1,
	})

	require.True(t, ok)
	require.Equal(t, 0, point.line)
	require.Greater(t, point.offset, 0)
}

func TestModel_TranscriptSelectionPointFromMouseUsesWrappedVisualViewportOffset(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetWidth(10)
	runModel.transcript.SetHeight(1)
	firstLine := "abcdefghijklmno"
	runModel.transcript.SetContent(firstLine + "\ntarget line")
	width := max(runModel.transcript.Width()-runModel.transcript.Style.GetHorizontalFrameSize(), 1)
	runModel.transcript.SetYOffset(getWrappedTranscriptLineHeight(firstLine, width))

	point, ok := runModel.transcriptSelectionPointFromMouse(tea.Mouse{
		X: getPanelHorizontalPadding(runModel.width) + len("target"),
		Y: runModel.getTranscriptTop(),
	})

	require.True(t, ok)
	require.Equal(
		t,
		transcriptSelectionPoint{line: 1, offset: len("abcdefghijklmno\n") + len("target")},
		point,
	)
}

func TestModel_SelectedTranscriptTextHandlesOutOfRangeOffsets(t *testing.T) {
	runModel := newModel()
	runModel.transcript.SetContent("abc")
	runModel.selection = transcriptSelection{
		active: true,
		start:  transcriptSelectionPoint{offset: 2},
		end:    transcriptSelectionPoint{offset: 20},
	}

	require.Equal(t, "c", runModel.selectedTranscriptText())

	runModel.selection = transcriptSelection{
		active: true,
		start:  transcriptSelectionPoint{offset: 10},
		end:    transcriptSelectionPoint{offset: 10},
	}
	require.Empty(t, runModel.selectedTranscriptText())
}

func TestTranscriptSelectionOffsetBoundsOrdersReverseSelection(t *testing.T) {
	selection := transcriptSelection{
		start: transcriptSelectionPoint{offset: 8},
		end:   transcriptSelectionPoint{offset: 3},
	}

	start, end := selection.offsetBounds()

	require.Equal(t, 3, start)
	require.Equal(t, 8, end)
}

func TestGetTranscriptSelectionPointRejectsInvalidLineIndex(t *testing.T) {
	require.Equal(t, transcriptSelectionPoint{}, getTranscriptSelectionPoint([]string{"one"}, 2, 0, 0))
	require.Equal(t, transcriptSelectionPoint{}, getTranscriptSelectionPoint([]string{"one"}, -1, 0, 0))
}

func TestGetTranscriptLineOffsetReturnsEndOffsetForPastEndIndex(t *testing.T) {
	require.Equal(t, len("one\ntwo"), getTranscriptLineOffset([]string{"one", "two"}, 10))
}

func TestGetByteOffsetForDisplayColumnSkipsANSISequences(t *testing.T) {
	line := renderTranscriptCell("Hand: hello")

	offset := getByteOffsetForDisplayColumn(line, len("Hand: hel"))

	require.Equal(t, strings.Index(line, "lo"), offset)
}

func TestHighlightTranscriptSelectionUsesDisplayColumnsForWideCharacters(t *testing.T) {
	line := renderTranscriptCell("Hand: 👋 anything")
	plain := stripANSI(line)
	start := strings.Index(plain, "anything")
	end := start + len("anything")

	highlighted := highlightTranscriptSelection(
		line,
		start,
		end,
		lipgloss.NewStyle().Reverse(true),
	)

	require.Contains(t, highlighted, "\x1b[7manything")
	require.NotContains(t, highlighted, "\x1b[7mything")
}

func TestGetDisplayColumnForByteOffsetHandlesWideCharacters(t *testing.T) {
	line := "Hand: 👋 anything"

	column := getDisplayColumnForByteOffset(line, strings.Index(line, "anything"))

	require.Equal(t, len("Hand: ")+2+1, column)
}

func TestModel_SetTranscriptContentClearsMouseSelection(t *testing.T) {
	runModel := newModel()
	runModel.messages = []string{"Hand: stale"}
	runModel.setTranscriptContent()
	runModel.selection = transcriptSelection{
		active: true,
		start:  transcriptSelectionPoint{offset: 0},
		end:    transcriptSelectionPoint{offset: len("Hand")},
	}
	runModel.applyTranscriptSelectionStyle()
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")

	runModel.messages = []string{"Hand: refreshed"}
	runModel.setTranscriptContent()

	require.False(t, runModel.selection.active)
	require.Empty(t, runModel.selectedTranscriptText())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Hand: refreshed")
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

func TestModel_SubmitPromptStartsRPCResponse(t *testing.T) {
	client := &fakeTUIChatClient{reply: "hello back"}
	runModel := newModelWithClient(client)
	runModel.input.SetValue("hello")

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.True(t, runModel.responding)
	require.True(t, runModel.thinkingComposerActive)
	require.Equal(t, []string{"You: hello"}, runModel.messages)
	require.Empty(t, runModel.input.Value())
	require.Equal(t, []string{"hello"}, runModel.history)
	require.Zero(t, client.calls)
}

func TestModel_SubmitPromptScrollsTranscriptToBottom(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	runModel.input.SetValue("hello")

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "❯ hello")
}

func TestModel_SubmitPromptStartsResponseFollowFromSettledBottom(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	runModel.input.SetValue(strings.Join([]string{
		"first line",
		"second line",
		"third line",
		"fourth line",
	}, "\n"))

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.True(t, runModel.responding)
	require.True(t, runModel.responseTranscriptFollow)
	require.False(t, runModel.responseTranscriptScrolled)
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd := runModel.Update(responseCompletedMsg{ResponseID: runModel.responseID, Text: "final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Hand: final")
}

func TestRespondToPromptCmd_StreamsDeltasTraceEventsAndCompletion(t *testing.T) {
	client := &fakeTUIChatClient{
		reply: "hello world",
		events: []rpcclient.Event{
			{Kind: agent.EventKindTextDelta},
			{Kind: agent.EventKindTextDelta, Text: "hello "},
			{
				Kind: agent.EventKindTrace,
				TraceEvent: &trace.Event{
					Type:    trace.EvtToolInvocationStarted,
					Payload: map[string]any{"id": "call_1", "name": "read_file"},
				},
			},
			{
				Kind: agent.EventKindTrace,
				TraceEvent: &trace.Event{
					Type:    trace.EvtFinalAssistantResponse,
					Payload: map[string]any{"message": "hello world"},
				},
			},
		},
	}
	events := make(chan tea.Msg, 8)

	msg := respondToPromptCmd(client, 7, context.Background(), "hello", events)()

	require.Equal(t, responseCompletedMsg{ResponseID: 7, Text: "hello world"}, msg)
	require.Equal(t, "hello", client.message)
	require.True(t, client.stream)
	require.Equal(t, assistantTextDeltaMsg{Channel: "assistant", Text: "hello "}, <-events)
	require.Equal(t, toolInvocationStartedMsg{ID: "call_1", Name: "read_file"}, <-events)
	_, ok := <-events
	require.False(t, ok)
}

func TestRespondToPromptCmd_ReturnsErrorCompletion(t *testing.T) {
	expectedErr := errors.New("daemon unavailable")
	client := &fakeTUIChatClient{err: expectedErr}
	events := make(chan tea.Msg, 1)

	msg := respondToPromptCmd(client, 3, nil, "hello", events)()

	require.Equal(t, responseCompletedMsg{ResponseID: 3, Err: expectedErr}, msg)
	require.Equal(t, "hello", client.message)
	_, ok := <-events
	require.False(t, ok)
}

func TestModel_UpdateAppliesResponseEventsAndCompletion(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "hello"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "Hand: hello", runModel.live)

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4, Text: "hello final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Nil(t, runModel.events)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"Hand: hello final"}, runModel.messages)
}

func TestModel_UpdatePreservesTranscriptScrollDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	require.Greater(t, bottomOffset, 0)
	runModel.transcript.GotoTop()
	offsetBefore := runModel.transcript.YOffset()
	runModel.responding = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, offsetBefore, runModel.transcript.YOffset())
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "Hand: streamed")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateFollowsBottomDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "Hand: streamed")
	require.Contains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateKeepsFollowingBottomWhenResponseCompletesAfterStream(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateScrollsToBottomWhenResponseCompletesWhileViewportIsAtBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseCompletedMsg{ResponseID: 4, Text: "final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Hand: final")
}

func TestModel_UpdateDoesNotScrollToBottomWhenResponseCompletesAfterManualScroll(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responseTranscriptScrolled)
	offsetBefore := runModel.transcript.YOffset()

	updated, cmd = runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})
	require.NotNil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, offsetBefore, runModel.transcript.YOffset())
	require.False(t, runModel.transcript.AtBottom())
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateDisablesFollowModeOnWheelDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(tea.MouseWheelMsg(tea.Mouse{
		Button: tea.MouseWheelUp,
		X:      getPanelHorizontalPadding(runModel.width),
		Y:      runModel.transcript.Height() - 1,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responseTranscriptScrolled)
	require.False(t, runModel.responseTranscriptFollow)
	offsetBefore := runModel.transcript.YOffset()

	updated, cmd = runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, offsetBefore, runModel.transcript.YOffset())
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateDoesNotScrollToBottomWhenResponseArrivesAwayFromBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, fmt.Sprintf("Message %02d", index))
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	offsetBefore := runModel.transcript.YOffset()
	runModel.responding = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})
	require.NotNil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, offsetBefore, runModel.transcript.YOffset())
	require.False(t, runModel.transcript.AtBottom())
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateSurfacesRPCErrorInStatusAndTranscript(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 2
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseCompletedMsg{
		ResponseID: 2,
		Err:        errors.New("daemon unavailable"),
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Nil(t, runModel.events)
	require.Equal(t, "response failed", runModel.status.Text())
	require.Equal(t, []string{"Error: daemon unavailable"}, runModel.messages)
}

func TestModel_UpdateAppliesSessionErrorMessage(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(sessionErrorMsg{Message: "daemon unavailable"})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "response failed", runModel.status.Text())
	require.Equal(t, []string{"Error: daemon unavailable"}, runModel.messages)
}

func TestModel_UpdateIgnoresStaleResponseEvents(t *testing.T) {
	runModel := newModel()
	runModel.responding = false
	runModel.responseID = 3
	runModel.messages = []string{"Hand: final"}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 3,
		Message:    assistantTextDeltaMsg{Text: "late delta"},
	})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"Hand: final"}, runModel.messages)
	require.NotContains(t, stripANSI(runModel.transcript.View()), "late delta")
}

func TestModel_UpdateHandlesResponseEventsClosed(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 3

	updated, cmd := runModel.Update(responseEventsClosedMsg{ResponseID: 3})

	require.Nil(t, cmd)
	require.True(t, updated.(model).responding)

	updated, cmd = runModel.Update(responseEventsClosedMsg{ResponseID: 2})

	require.Nil(t, cmd)
	require.True(t, updated.(model).responding)
}

func TestModel_UpdateIgnoresStaleResponseCompletion(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 5

	updated, cmd := runModel.Update(responseCompletedMsg{ResponseID: 4, Text: "stale"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responding)
	require.Empty(t, runModel.messages)
}

func TestWaitForResponseEventReturnsQueuedAndClosedMessages(t *testing.T) {
	events := make(chan tea.Msg, 1)
	events <- sessionErrorMsg{Message: "failed"}

	msg := waitForResponseEvent(9, events)()

	require.Equal(t, responseEventMsg{
		ResponseID: 9,
		Message:    sessionErrorMsg{Message: "failed"},
	}, msg)

	close(events)
	msg = waitForResponseEvent(9, events)()

	require.Equal(t, responseEventsClosedMsg{ResponseID: 9}, msg)
}

func TestModel_UpdateAddsTraceMessagesToTranscript(t *testing.T) {
	runModel := newModel()

	for index, msg := range []tea.Msg{
		toolInvocationStartedMsg{Name: "read_file"},
		toolInvocationCompletedMsg{Name: "read_file"},
		safetyEventMsg{Action: "blocked", FindingIDs: []string{"prompt_exfiltration"}},
	} {
		updated, cmd := runModel.Update(msg)
		if index == 0 {
			require.NotNil(t, cmd)
		} else {
			require.Nil(t, cmd)
		}
		runModel = updated.(model)
	}

	require.Equal(t, []string{
		toolOperationTranscriptCell("", "read_file", ""),
		toolOperationTranscriptCell("", "read_file", "", true),
		"Safety: blocked: prompt_exfiltration",
	}, runModel.messages)
}

func TestModel_UpdateAnimatesRunningToolTranscriptDot(t *testing.T) {
	originalInterval := toolAnimationInterval
	t.Cleanup(func() {
		toolAnimationInterval = originalInterval
	})
	toolAnimationInterval = time.Nanosecond
	runModel := newModel()

	updated, cmd := runModel.Update(toolInvocationStartedMsg{ID: "call_1", Name: "web_search"})
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.toolAnimationActive)
	require.Contains(t, stripANSI(runModel.transcript.View()), "● Web Search")
	require.Equal(t, toolAnimationTickMsg{}, cmd())

	updated, cmd = runModel.Update(toolAnimationTickMsg{})
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.toolAnimationFrame)
	require.Contains(t, stripANSI(runModel.transcript.View()), "◖ Web Search")

	updated, cmd = runModel.Update(toolInvocationCompletedMsg{ID: "call_1", Name: "web_search"})
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Contains(t, stripANSI(runModel.transcript.View()), "● Searched")

	updated, cmd = runModel.Update(toolAnimationTickMsg{})
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.toolAnimationActive)
	require.Contains(t, stripANSI(runModel.transcript.View()), "● Searched")
}

func TestModel_UpdateAnimatesThinkingComposerBorder(t *testing.T) {
	originalInterval := thinkingComposerInterval
	t.Cleanup(func() {
		thinkingComposerInterval = originalInterval
	})
	thinkingComposerInterval = time.Nanosecond
	runModel := newModel()
	runModel.responding = true

	cmd := runModel.startThinkingComposer()
	require.NotNil(t, cmd)
	require.True(t, runModel.thinkingComposerActive)
	require.Equal(t, getThinkingComposerBorderColor(0), runModel.getInputFrameBorderColor())
	require.Equal(t, thinkingComposerTickMsg{}, cmd())

	updated, cmd := runModel.Update(thinkingComposerTickMsg{})
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.thinkingComposerFrame)
	require.Equal(t, getThinkingComposerBorderColor(1), runModel.getInputFrameBorderColor())

	runModel.live = "Hand: hello"
	updated, cmd = runModel.Update(thinkingComposerTickMsg{})
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.thinkingComposerActive)
	require.Equal(t, "8", runModel.getInputFrameBorderColor())
}

func TestModel_ThinkingComposerBorderWaitsForRunningTool(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.messages = []string{toolOperationTranscriptCell("call_1", "web_search", "")}

	require.False(t, runModel.isThinkingComposerVisible())
	require.Equal(t, "8", runModel.getInputFrameBorderColor())

	runModel.messages = []string{toolOperationTranscriptCell("call_1", "web_search", "", true)}
	require.True(t, runModel.isThinkingComposerVisible())
	require.Equal(t, getThinkingComposerBorderColor(0), runModel.getInputFrameBorderColor())
}

func TestModel_ThinkingComposerBorderCanBeDisabled(t *testing.T) {
	disabled := false
	runModel := newModelWithClientContextAndConfig(
		context.Background(),
		nil,
		&config.Config{TUI: config.TUIConfig{ThinkingComposer: &disabled}},
	)
	runModel.responding = true

	require.False(t, runModel.thinkingComposerEnabled)
	require.False(t, runModel.isThinkingComposerVisible())
	require.NotNil(t, runModel.startThinkingComposer())
	require.True(t, runModel.thinkingComposerActive)
	require.Equal(t, "8", runModel.getInputFrameBorderColor())
}

func TestModel_UpdatePreventsOverlappingPromptSubmission(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.responding = true
	runModel.input.SetValue("second prompt")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "response already in progress", runModel.status.Text())
	require.Equal(t, "second prompt", runModel.input.Value())
	require.Empty(t, runModel.messages)
	require.Empty(t, runModel.history)
}

func TestModel_UpdateKeepsCommandsLocalDuringActiveResponse(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.responding = true
	runModel.input.SetValue("/help")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responding)
	require.Equal(t, []string{"Commands: /clear, /copy, /help"}, runModel.messages)
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

func TestModel_UpdateTrimsTrailingPasteLineBreaks(t *testing.T) {
	runModel := newModel()
	paste := "first\nsecond\n\n"

	updated, cmd := runModel.Update(tea.PasteMsg{Content: paste})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "first\nsecond", runModel.input.Value())
	require.Equal(t, 2, runModel.input.Height())
	require.Contains(t, stripANSI(runModel.input.View()), "second")
}

func TestModel_UpdateSizesPasteUsingTextareaWidth(t *testing.T) {
	runModel := newModel()
	runModel.width = 180
	runModel.height = 20
	runModel.resize()
	paste := strings.Join([]string{
		`office.\n[...]\nOn Monday Iran said it had responded to the latest US proposal and that exchanges with Washington were continuing through Pakistani mediators.`,
		`\n[...]\nTrump's message echoed his threat that a \"whole civilisation\" would die unless Iran agreed to a deal to end the war.`,
		`\n[...]\nIsraeli and US forces began massive air strikes on Iran on 28 February. The ceasefire meant to facilitate`,
	}, "")

	updated, cmd := runModel.Update(tea.PasteMsg{Content: paste})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Greater(t, runModel.input.Height(), 1)
	require.Zero(t, runModel.input.ScrollYOffset())
	require.Contains(t, stripANSI(runModel.input.View()), "office.")
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
	require.Contains(t, content, "❯ hello")
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

	require.Equal(t, 2, runModel.input.Height())
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

func getTranscriptContentRow(t *testing.T, runModel model, needle string) int {
	t.Helper()

	lines := strings.Split(stripANSI(runModel.transcript.GetContent()), "\n")
	for index, line := range lines {
		if strings.Contains(line, needle) {
			return index
		}
	}

	t.Fatalf("transcript row containing %q not found in %q", needle, runModel.transcript.GetContent())
	return 0
}

func trimTrailingLineSpaces(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " ")
	}

	return strings.Join(lines, "\n")
}

func newTUITestRootCommand(action func(context.Context, *cli.Command) error) *cli.Command {
	envFile := ".env"
	configFile := "config.yaml"

	return &cli.Command{
		Flags: handcli.RootFlags(&envFile, &configFile),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return action(ctx, cmd)
		},
	}
}

type fakeTUIChatClient struct {
	events        []rpcclient.Event
	reply         string
	err           error
	timeline      rpcclient.SessionTimeline
	timelineErr   error
	message       string
	stream        bool
	calls         int
	timelineCalls int
	closed        bool
}

func (c *fakeTUIChatClient) Respond(
	_ context.Context,
	message string,
	opts rpcclient.RespondOptions,
) (string, error) {
	c.calls++
	c.message = message
	if opts.Stream != nil {
		c.stream = *opts.Stream
	}
	for _, event := range c.events {
		if opts.OnEvent != nil {
			opts.OnEvent(event)
		}
	}

	return c.reply, c.err
}

func (c *fakeTUIChatClient) GetSessionTimeline(
	_ context.Context,
	_ rpcclient.SessionTimelineOptions,
) (rpcclient.SessionTimeline, error) {
	c.timelineCalls++
	return c.timeline, c.timelineErr
}

func (c *fakeTUIChatClient) Close() error {
	c.closed = true
	return nil
}

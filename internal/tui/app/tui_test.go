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

	agentapi "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/internal/tui/render"
	agent "github.com/wandxy/hand/pkg/agent"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
)

func TestMain(m *testing.M) {
	original := promptHistoryPath
	originalTheme := defaultTUITheme
	originalProfile := profile.Active()
	testProfileHome, _ := os.MkdirTemp("", "hand-tui-profile-*")
	_ = original()
	promptHistoryPath = func() string {
		return ""
	}
	if testProfileHome != "" {
		_ = os.WriteFile(
			filepath.Join(testProfileHome, userNameFilename),
			[]byte("{\"name\":\"Kennedy\"}\n"),
			0o600,
		)
		profile.SetActive(profile.Profile{Name: profile.DefaultName, HomeDir: testProfileHome})
	} else {
		profile.SetActive(profile.Profile{})
	}
	defaultTUITheme = render.DefaultTheme
	code := m.Run()
	promptHistoryPath = original
	defaultTUITheme = originalTheme
	profile.SetActive(originalProfile)
	if testProfileHome != "" {
		_ = os.RemoveAll(testProfileHome)
	}
	os.Exit(code)
}

func TestModel_ViewRendersShellAreas(t *testing.T) {
	model := newModel()
	view := model.View()
	content := stripANSI(view.Content)

	require.True(t, view.AltScreen)
	require.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
	require.Contains(t, view.Content, "48;5;235")
	require.Contains(t, content, "██████")
	require.Contains(t, content, "/changelog")
	require.Contains(t, content, "Hi, Kennedy")
	require.Contains(t, content, emptyUserPromptQuestion)
	require.Contains(t, content, inputPrompt+"Ask Hand...")
	require.Contains(t, content, "Ask Hand...")
	require.Contains(t, content, "minimax-m2.7")
	require.Contains(t, content, "enter to send")
}

func TestModel_ViewShowsCancelHintDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.responding = true

	content := stripANSI(runModel.View().Content)

	require.Contains(t, content, "esc to stop")
	require.NotContains(t, content, "enter to send")
}

func TestModel_InitFocusesInput(t *testing.T) {
	runModel := newModel()

	cmd := runModel.Init()

	require.NotNil(t, cmd)
}

func TestNewModelWithClientContextDefaultsNilContext(t *testing.T) {
	var ctx context.Context
	runModel := newModelWithClientContext(ctx, nil)

	require.NotNil(t, runModel.chatCtx)
}

func TestNewModel_ShowsNamePromptForEmptyProfile(t *testing.T) {
	home := t.TempDir()
	setActiveTestProfile(t, home)

	runModel := newModel()
	content := stripANSI(runModel.View().Content)

	require.True(t, runModel.shouldShowNamePrompt())
	require.Contains(t, content, "████████")
	require.Contains(t, content, namePromptTitle)
	require.Contains(t, content, namePromptPlaceholder)
	require.Contains(t, content, namePromptSubmitHint)
	require.NotContains(t, content, inputPrompt+"Ask Hand")
	require.NotContains(t, content, "Welcome to Hand TUI")
}

func TestNewModel_LoadsSavedProfileName(t *testing.T) {
	home := t.TempDir()
	setActiveTestProfile(t, home)
	require.NoError(t, os.WriteFile(
		filepath.Join(home, userNameFilename),
		[]byte("{\"name\":\"Nedy\"}\n"),
		0o600,
	))

	runModel := newModel()

	require.False(t, runModel.shouldShowNamePrompt())
	require.Equal(t, "Nedy", runModel.userName)
	require.Contains(t, stripANSI(runModel.renderHeader()), "Welcome, Nedy")
}

func TestNewModel_ShowsEmptyPromptForSavedProfileName(t *testing.T) {
	home := t.TempDir()
	setActiveTestProfile(t, home)
	require.NoError(t, os.WriteFile(
		filepath.Join(home, userNameFilename),
		[]byte("{\"name\":\"Nedy\"}\n"),
		0o600,
	))

	runModel := newModel()
	content := stripANSI(runModel.View().Content)

	require.True(t, runModel.shouldShowEmptyUserPrompt())
	require.Contains(t, content, "██████")
	require.Contains(t, content, "/changelog")
	require.Contains(t, content, "Hi, Nedy")
	require.Contains(t, content, emptyUserPromptQuestion)
	require.Contains(t, content, inputPrompt+"Ask Hand")
	require.NotContains(t, content, "Welcome to Hand TUI")
}

func TestModel_SubmitsNamePrompt(t *testing.T) {
	home := t.TempDir()
	setActiveTestProfile(t, home)
	runModel := newModel()
	runModel.nameInput.SetValue("  Nedy-Okpala  ")

	updated, cmd := runModel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	runModel = updated.(model)
	data, err := os.ReadFile(filepath.Join(home, userNameFilename))
	require.NoError(t, err)
	require.False(t, runModel.shouldShowNamePrompt())
	require.Equal(t, "Nedy-Okpala", runModel.userName)
	require.JSONEq(t, `{"name":"Nedy-Okpala"}`, string(data))
	require.Contains(t, stripANSI(runModel.renderHeader()), "Welcome, Nedy-Okpala")
}

func TestModel_SubmitNamePromptRejectsInvalidName(t *testing.T) {
	now := time.Date(2026, 5, 28, 20, 0, 0, 0, time.UTC)
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	currentTime = func() time.Time {
		return now
	}
	home := t.TempDir()
	setActiveTestProfile(t, home)
	runModel := newModel()
	runModel.nameInput.SetValue("Nedy Okpala!")

	updated, cmd := runModel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	runModel = updated.(model)
	_, err := os.Stat(filepath.Join(home, userNameFilename))
	require.True(t, os.IsNotExist(err))
	require.True(t, runModel.shouldShowNamePrompt())
	require.Empty(t, runModel.userName)
	require.Equal(t, defaultStatus, runModel.status.Text())
	require.Contains(t, stripANSI(runModel.View().Content), namePromptInvalidHint)

	require.Equal(t, namePromptErrorExpiredMsg{startedAt: now}, cmd())
	updated, cmd = runModel.Update(namePromptErrorExpiredMsg{startedAt: now})
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.namePromptError)
	require.Contains(t, stripANSI(runModel.View().Content), namePromptSubmitHint)
	require.NotContains(t, stripANSI(runModel.View().Content), namePromptInvalidHint)
}

func TestModel_NamePromptAllowsCtrlCExit(t *testing.T) {
	home := t.TempDir()
	setActiveTestProfile(t, home)
	runModel := newModel()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)

	runModel = updated.(model)
	require.Equal(t, "Press Ctrl-C again to exit", runModel.status.Text())
	require.True(t, runModel.shouldShowNamePrompt())
}

func TestModel_InitLoadsExistingSessionTimeline(t *testing.T) {
	client := &fakeTUIChatClient{
		timeline: rpcclient.SessionTimeline{
			SessionID: "default",
			Messages: []agentapi.SessionTimelineMessage{{
				Message: handmsg.Message{Role: handmsg.RoleUser, Content: "older prompt"},
			}},
		},
	}
	runModel := newModelWithClient(client)

	cmd := runModel.Init()

	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 3)

	loaded, ok := batch[1]().(sessionTimelineLoadedMsg)
	require.True(t, ok)

	require.Equal(t, "default", loaded.Timeline.SessionID)
	require.Len(t, loaded.Timeline.Messages, 1)
	require.Equal(t, 1, client.timelineCalls)
}

func TestModel_InitLoadsSessionContextUsage(t *testing.T) {
	client := &fakeTUIChatClient{
		contextStatus: rpcclient.ContextStatus{
			SessionID: "default",
			Length:    128000,
			Used:      64000,
			UsedPct:   0.5,
		},
	}
	runModel := newModelWithClient(client)

	cmd := runModel.Init()

	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 3)

	loaded, ok := batch[2]().(sessionContextLoadedMsg)
	require.True(t, ok)

	require.Equal(t, "default", client.contextSessionID)
	require.Equal(t, 1, client.contextCalls)
	require.Equal(t, 64000, loaded.Status.Used)
}

func TestLoadSessionTimelineCmdReturnsLoadFailure(t *testing.T) {
	expectedErr := errors.New("timeline unavailable")
	client := &fakeTUIChatClient{timelineErr: expectedErr}

	cmd := loadSessionTimelineCmd(context.Background(), client)

	require.NotNil(t, cmd)
	require.Equal(t, sessionTimelineLoadFailedMsg{Err: expectedErr}, cmd())
}

func TestFormatSessionContextUsageUsesStatusValues(t *testing.T) {
	status := rpcclient.ContextStatus{
		Length:  128000,
		Used:    64000,
		UsedPct: 0.5,
	}

	require.Equal(t, "64,000 used · 50%", formatSessionContextUsage(status))
}

func TestFormatSessionContextUsageComputesMissingPercent(t *testing.T) {
	status := rpcclient.ContextStatus{
		Length: 200000,
		Used:   130000,
	}

	require.Equal(t, "130,000 used · 65%", formatSessionContextUsage(status))
}

func TestLoadSessionTitleCmdReturnsLoadedTitle(t *testing.T) {
	client := &fakeTUIChatClient{
		currentSession: storage.Session{
			ID:    "default",
			Title: "Daily Planning",
		},
	}

	cmd := loadSessionTitleCmd(context.Background(), client)

	require.NotNil(t, cmd)
	require.Equal(t, sessionTitleLoadedMsg{Session: client.currentSession}, cmd())
	require.Equal(t, 1, client.currentSessionCalls)
}

func TestModel_UpdateHydratesLoadedSessionTimeline(t *testing.T) {
	runModel := newModel()
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	originalTime := currentTime
	currentTime = func() time.Time { return now }
	t.Cleanup(func() { currentTime = originalTime })

	updated, cmd := runModel.Update(sessionTimelineLoadedMsg{
		Timeline: rpcclient.SessionTimeline{
			SessionID: "default",
			Title:     "Daily Planning",
			Messages: []agentapi.SessionTimelineMessage{{
				Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "older answer"},
			}},
			TraceEvents: []agentapi.SessionTimelineTraceEvent{{
				Event: agentsession.TraceEvent{
					Type:      trace.EvtContextCompactionSucceeded,
					Timestamp: now,
					Payload: trace.CompactionEventPayload{
						SessionID: "default",
						Status:    string(storage.CompactionStatusSucceeded),
						Auto:      true,
					},
				},
			}},
		},
	})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"Automatic compaction completed", "Hand: older answer"}, transcriptCellPlainTexts(runModel.messages))
	require.Contains(t, stripANSI(runModel.transcript.View()), "older answer")
	require.Equal(t, defaultSessionID, runModel.sessionID)
	require.Equal(t, "Daily Planning (default)", runModel.sessionTitle)
	require.Contains(t, transcriptCellPlainTexts(runModel.messages), "Automatic compaction completed")
	require.Contains(t, stripANSI(runModel.View().Content), "Automatic compaction completed")
	require.Equal(t, defaultStatus, runModel.status.Text())
}

func TestModel_ApplyTUIMessageRendersLiveAutoCompactionTrace(t *testing.T) {
	runModel := newModel()

	cmd := runModel.applyTUIMessage(manualCompactionMsg{
		State: manualCompactionState{Status: "succeeded", Label: autoCompactionLabel},
	})

	require.Nil(t, cmd)
	require.Equal(t, []string{"Automatic compaction completed"}, transcriptCellPlainTexts(runModel.messages))
	require.Contains(t, stripANSI(runModel.View().Content), "Automatic compaction completed")
}

func TestModel_UpdateReportsTimelineLoadFailure(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(sessionTimelineLoadFailedMsg{Err: errors.New("timeline unavailable")})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "session timeline unavailable", runModel.status.Text())
	require.Contains(t, stripANSI(runModel.View().Content), emptyUserPromptQuestion)
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
	setStatusTransient(&runModel.status, "loaded")
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
	runModel.status.SetHideAfter(0)
	setStatusTransient(&runModel.status, "loaded")
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
	runModel.status.SetHideAfter(time.Nanosecond)
	setStatusTransient(&runModel.status, "loaded")
	cmd := runModel.statusExpireCmd()

	require.NotNil(t, cmd)
	require.Equal(t, statusExpiredMsg{startedAt: now}, cmd())
}

func TestModel_ViewRendersHeaderInfoPanelWhenWide(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.Models.Main.Provider = "openai"
	cfg.Models.Main.Name = "openai/gpt-4o-mini"
	cfg.Models.Summary.Provider = "openrouter"
	cfg.Models.Summary.Name = "openai/gpt-4o"
	cfg.Models.Embedding.Provider = "openai"
	cfg.Models.Embedding.Name = "openai/text-embedding-3-large"
	cfg.Storage.Backend = "memory"
	runModel := newModelWithClientContextAndConfig(context.Background(), nil, cfg)
	runModel.runtimeInfo.Profile = "work"
	runModel.width = 180
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "Welcome, Kennedy")
	require.Contains(t, content, "Use /changelog to see what changed")
	require.Contains(t, content, "version: dev")
	require.Contains(t, content, "commit: unknown")
	require.Contains(t, content, "profile: work")
	require.Contains(t, content, "session: default")
	require.Contains(t, content, "provider: openai")
	require.Contains(t, content, "model: gpt-4o-mini")
	require.Contains(t, content, "summary: gpt-4o")
	require.Contains(t, content, "embedding: text-embedding-3-large")
	require.Contains(t, content, "storage: memory")
	require.Contains(t, content, "streaming: on")
	require.NotContains(t, content, "summary: openai/gpt-4o")
}

func TestModel_RenderNoticeBarFillsRow(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	lines := strings.Split(stripANSI(runModel.renderNoticeBar()), "\n")

	require.Len(t, lines, noticeBarHeight)
	require.Contains(t, lines[0], "Welcome, Kennedy")
	require.Contains(t, lines[0], "Use /changelog to see what changed")
	require.Equal(t, 80, lipgloss.Width(lines[0]))
}

func TestModel_RenderNoticeBarUsesConfiguredColors(t *testing.T) {
	content := newModel().renderNoticeBar()

	require.Contains(t, content, "48;5;235")
	require.Contains(t, renderNoticeBarLeft(), "38;5;246")
	require.Contains(t, renderNoticeBarLeft(), "97")
	require.Contains(t, renderNoticeBarRight(), "38;5;246")
	require.Contains(t, renderNoticeBarRight(), "97")
}

func TestRenderNoticeBarContent_HidesRightTextWhenTooNarrow(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", "Use /changelog", 8))

	require.Equal(t, "Welcome", content)
}

func TestRenderNoticeBarContent_HidesRightTextWhenMissing(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", " ", 80))

	require.Equal(t, "Welcome", content)
}

func TestRenderNoticeBarContent_FillsWidthWithSpacer(t *testing.T) {
	content := stripANSI(renderNoticeBarContent("Welcome", "Use /changelog", 30))

	require.Equal(t, "Welcome         Use /changelog", content)
	require.Equal(t, 30, lipgloss.Width(content))
}

func TestModel_ViewAlignsHeaderInfoKeys(t *testing.T) {
	runModel := newModel()
	runModel.width = 180
	runModel.resize()
	lines := strings.Split(stripANSI(runModel.renderHeaderInfoPanel()), "\n")
	rows := getHeaderInfoRows(runModel)
	columnWidth := getHeaderInfoColumnWidth(rows)
	leftColonIndex := headerInfoKeyWidth
	rightColonIndex := columnWidth + headerInfoColumnGap + headerInfoKeyWidth

	require.Len(t, lines, (len(rows)+1)/2)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		require.Equal(t, leftColonIndex, strings.Index(line, ":"))
		if strings.Count(line, ":") > 1 {
			require.Equal(t, rightColonIndex, strings.LastIndex(line, ":"))
		}
	}
}

func TestRenderHeaderInfoPanel_UsesOneColorForBothColumns(t *testing.T) {
	panel := getHeaderPanel(newModel(), 180)
	content := renderHeaderInfoPanel(panel)
	modelCell := renderBottomStatusMutedCell("model")

	require.Equal(t, lipgloss.Height(content), strings.Count(content, "\x1b[90m"))
	require.Contains(t, modelCell, "\x1b[90m")
	require.NotContains(t, content, "38;5;"+defaultTUITheme.ToolDetail)
	require.Contains(t, stripANSI(content), "version:")
	require.Contains(t, stripANSI(content), "model:")
}

func TestModel_ViewSizesHeaderInfoPanelToValues(t *testing.T) {
	runModel := newModel()
	runModel.width = 180
	runModel.resize()
	content := stripANSI(runModel.renderHeaderInfoPanel())
	columnWidth := headerInfoKeyWidth + 2 + lipgloss.Width("text-embedding-3-small")

	require.Equal(t, columnWidth*2+headerInfoColumnGap, lipgloss.Width(content))
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

func TestModel_ViewShowsHeaderMarkNextToFullBanner(t *testing.T) {
	runModel := newModel()
	runModel.width = lipgloss.Width(handHeaderMark) + headerGapWidth + lipgloss.Width(handBanner) + headerBodyPadding*2
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "░████████  ░██")
	require.Contains(t, content, "░█░█░█▀")
	require.Contains(t, content, "   █ █ █")
	require.Contains(t, content, "   ▀▀▀▀▀  ")
}

func TestModel_ViewHidesHeaderMarkWhenFullBannerWouldClip(t *testing.T) {
	runModel := newModel()
	runModel.width = lipgloss.Width(handHeaderMark) + headerGapWidth + lipgloss.Width(handBanner) + headerBodyPadding*2 - 1
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "░██     ░██")
	require.NotContains(t, content, "░█░█░█▀")
}

func TestModel_ViewUsesCompactBannerWhenFullBannerDoesNotFit(t *testing.T) {
	runModel := newModel()
	runModel.width = lipgloss.Width(handBanner) - 1
	runModel.resize()
	content := stripANSI(runModel.renderHeader())

	require.Contains(t, content, "|_||_\\__,_|_||_\\__,_|")
	require.NotContains(t, content, "░██")
}

func TestRenderHeaderBody_FillsAvailableWidthWhenInfoIsVisible(t *testing.T) {
	runModel := newModel()
	panel := getHeaderPanel(runModel, 120)
	content := stripANSI(renderHeaderBody(panel))

	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		require.Equal(t, panel.Width, lipgloss.Width(line))
	}
}

func TestRenderHeaderBody_InsetsBannerAndInfo(t *testing.T) {
	runModel := newModel()
	panel := getHeaderPanel(runModel, 120)
	content := stripANSI(renderHeaderBody(panel))

	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		require.True(t, strings.HasPrefix(line, " "))
		require.True(t, strings.HasSuffix(line, " "))
	}
}

func TestModel_ViewRendersBottomStatusPanelBelowComposer(t *testing.T) {
	runModel := newModel()
	content := stripANSI(runModel.View().Content)
	inputIndex := strings.Index(content, inputPrompt+"Ask Hand...")
	infoIndex := strings.LastIndex(content, "minimax-m2.7")

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

func TestRenderComposerInputPrompt_HasNoBackgroundColor(t *testing.T) {
	prompt := renderComposerInputPrompt()

	require.Contains(t, stripANSI(prompt), inputPrompt)
	require.NotContains(t, prompt, "[48;")
}

func TestModel_RenderBottomStatusPanelMovesContextToRight(t *testing.T) {
	runModel := newModel()
	runModel.context = "64,000 used · 50%"
	content := stripANSI(runModel.renderBottomStatusPanel())

	require.True(t, strings.HasPrefix(content, " "))
	require.Equal(t, runModel.width, lipgloss.Width(content))
	require.Contains(t, content, "minimax-m2.7")
	require.Contains(t, content, "default session")
	require.Contains(t, content, "64,000")
	require.Contains(t, content, "used · 50%")
	require.GreaterOrEqual(t, strings.Count(content, "  "), 1)
	require.Greater(t, strings.Index(content, "64,000"), strings.Index(content, "default session"))
}

func TestModel_RenderBottomStatusPanelShowsThinkingBeforeModel(t *testing.T) {
	runModel := newModel()
	runModel.responding = true

	content := stripANSI(runModel.renderBottomStatusPanel())

	require.Contains(t, content, "Thinking")
	require.Contains(t, content, "minimax-m2.7")
	require.Less(t, strings.Index(content, "Thinking"), strings.Index(content, "minimax-m2.7"))
}

func TestModel_RenderBottomStatusPanelHidesThinkingWhenNotThinking(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.live = assistantTranscriptCell{text: "hello"}

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

	require.Contains(t, content, renderBottomStatusMutedCell("minimax-m2.7"))
	require.Contains(t, content, renderBottomStatusMutedCell(statusCancelSuffix))
	require.Contains(t, content, renderBottomStatusMutedCell(defaultSessionTitle))
}

func TestSpaceAroundBottomStatusPanel_CentersTitle(t *testing.T) {
	content := stripANSI(spaceAroundBottomStatusPanel("model", "Project Planning", "context", 60))

	require.Equal(t, 22, strings.Index(content, "Project Planning"))
	require.True(t, strings.HasPrefix(content, "model"))
	require.True(t, strings.HasSuffix(content, "context"))
}

func TestGetPanelHorizontalPadding_DisablesPaddingWhenNarrow(t *testing.T) {
	require.Equal(t, 0, getPanelHorizontalPadding(2))
	require.Equal(t, panelHorizontalPadding, getPanelHorizontalPadding(3))
}

func TestJoinBottomStatusPanelSegments_HandlesEmptySingleAndNarrowValues(t *testing.T) {
	require.Empty(t, joinBottomStatusPanelSegments([]string{" ", ""}, 20))
	require.Equal(t, "enter to send · ctrl+c to quit", joinBottomStatusPanelSegments([]string{"enter to send · ctrl+c to quit"}, 40))
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
	mainWidth := resized.getMainPaneWidth()
	require.Equal(t, 100, resized.width)
	require.Equal(t, 30, resized.height)
	require.Equal(t, mainWidth, resized.transcript.Width())
	require.LessOrEqual(t, resized.input.Width(), mainWidth)
	require.GreaterOrEqual(t, resized.transcript.Height(), 1)
	require.Equal(t, 1, resized.input.Height())
	lines := strings.Split(stripANSI(resized.transcript.GetContent()), "\n")
	require.NotEmpty(t, lines)
	require.Equal(t, mainWidth, lipgloss.Width(lines[0]))
	require.Contains(t, stripANSI(resized.View().Content), emptyUserPromptQuestion)
}

func TestModel_UpdateScrollsTranscriptWithPagingKeys(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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

func TestModel_RenderTranscriptContentPreservesMainPaneHeader(t *testing.T) {
	runModel := newModel()
	runModel.width = 120
	runModel.resize()
	runModel.messages = []transcriptCell{systemTranscriptCell{text: "ready"}}
	runModel.setTranscriptContent()
	lines := strings.Split(stripANSI(runModel.transcript.GetContent()), "\n")
	viewLines := strings.Split(stripANSI(runModel.View().Content), "\n")
	mainWidth := runModel.getMainPaneWidth()

	require.NotEmpty(t, lines)
	require.Equal(t, mainWidth, lipgloss.Width(lines[0]))
	require.True(t, strings.HasPrefix(lines[0], " Welcome, Kennedy"))
	require.NotEmpty(t, viewLines)
	require.Equal(t, runModel.width, lipgloss.Width(viewLines[0]))
	require.True(t, strings.HasPrefix(viewLines[0], " Welcome, Kennedy"))
}

func TestModel_RenderTranscriptContentKeepsFirstPromptCloseToHeader(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.resize()
	runModel.setTranscriptContent()

	lines := strings.Split(stripANSI(runModel.transcript.GetContent()), "\n")
	firstPromptRow := -1
	for index, line := range lines {
		if strings.Contains(line, "❯ hello") {
			firstPromptRow = index
			break
		}
	}

	require.Greater(t, firstPromptRow, 2)
	require.NotEmpty(t, strings.TrimSpace(lines[firstPromptRow-1]))
	require.Contains(t, lines[firstPromptRow-1], "▄")
}

func TestModel_UpdateScrollsTranscriptWithMouseWheel(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	require.Greater(t, bottomOffset, 0)

	updated, cmd := runModel.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Less(t, runModel.transcript.YOffset(), bottomOffset)
}

func TestModel_UpdateOpensTranscriptLinkWithClick(t *testing.T) {
	originalOpenExternalLink := openExternalLink
	t.Cleanup(func() {
		openExternalLink = originalOpenExternalLink
	})

	opened := ""
	openExternalLink = func(raw string) error {
		opened = raw
		return nil
	}

	runModel := newModel()
	runModel.width = 100
	runModel.height = 20
	runModel.resize()
	runModel.messages = []transcriptCell{
		assistantTranscriptCell{text: "Read [docs](https://example.com/docs) for details."},
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()

	lines := strings.Split(stripANSI(runModel.transcript.View()), "\n")
	row := indexLineContaining(lines, "docs")
	require.NotEqual(t, -1, row)
	column := strings.Index(lines[row], "docs")
	require.NotEqual(t, -1, column)

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      column,
		Y:      runModel.getTranscriptTop() + row,
	}))

	require.Nil(t, cmd)
	require.Equal(t, "https://example.com/docs", opened)
	require.False(t, updated.(model).selection.active)
}

func TestModel_UpdateDoesNotOpenTranscriptLinkWithRightClick(t *testing.T) {
	originalOpenExternalLink := openExternalLink
	t.Cleanup(func() {
		openExternalLink = originalOpenExternalLink
	})

	openExternalLink = func(string) error {
		t.Fatal("right click should not open external link")
		return nil
	}

	runModel := newModel()
	runModel.width = 100
	runModel.height = 20
	runModel.resize()
	runModel.messages = []transcriptCell{
		assistantTranscriptCell{text: "Read [docs](https://example.com/docs) for details."},
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()

	lines := strings.Split(stripANSI(runModel.transcript.View()), "\n")
	row := indexLineContaining(lines, "docs")
	require.NotEqual(t, -1, row)
	column := strings.Index(lines[row], "docs")
	require.NotEqual(t, -1, column)

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseRight,
		X:      column,
		Y:      runModel.getTranscriptTop() + row,
	}))

	require.Nil(t, cmd)
	require.False(t, updated.(model).selection.active)
}

func TestModel_UpdateDoesNotScrollTranscriptWhenTypingComposerText(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "k", runModel.input.Value())
	require.Equal(t, bottomOffset, runModel.transcript.YOffset())
	require.True(t, runModel.transcript.AtBottom())
}

func TestModel_ViewShowsJumpToBottomWhenTranscriptIsNotAtBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	require.NotContains(t, stripANSI(runModel.View().Content), jumpToBottomLabel)

	runModel.transcript.GotoTop()

	require.False(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.View().Content), jumpToBottomLabel)
}

func TestModel_UpdateJumpsTranscriptToBottomFromIndicatorAndShortcut(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	bottomOffset := runModel.transcript.YOffset()
	runModel.transcript.GotoTop()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      runModel.width / 2,
		Y:      runModel.getJumpToBottomIndicatorRow(),
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, bottomOffset, runModel.transcript.YOffset())
	require.True(t, runModel.transcript.AtBottom())

	runModel.transcript.GotoTop()
	updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd, Mod: tea.ModCtrl}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, bottomOffset, runModel.transcript.YOffset())
	require.True(t, runModel.transcript.AtBottom())
}

func TestModel_HydrateSessionTimelineReplacesVisibleTranscript(t *testing.T) {
	runModel := newModel()
	runModel.height = 14
	runModel.resize()
	runModel.messages = []transcriptCell{systemTranscriptCell{text: "stale cell"}}
	runModel.transcript.SetContent("stale cell")

	messages := make([]agentapi.SessionTimelineMessage, 0, 20)
	for index := 0; index < 18; index++ {
		messages = append(messages, agentapi.SessionTimelineMessage{
			Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: fmt.Sprintf("older %02d", index)},
		})
	}
	messages = append(messages,
		agentapi.SessionTimelineMessage{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "hello"}},
		agentapi.SessionTimelineMessage{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "hi"}},
	)

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{
		SessionID: "project-a",
		Title:     "Project Planning",
		Messages:  messages,
		TraceEvents: []agentapi.SessionTimelineTraceEvent{{
			Event: agentsession.TraceEvent{
				Type:    trace.EvtToolInvocationStarted,
				Payload: map[string]any{"id": "call_1", "name": "read_file"},
			},
		}},
	})

	content := stripANSI(runModel.transcript.View())
	require.Equal(t, "Project Planning", runModel.sessionTitle)
	require.Equal(t, defaultStatus, runModel.status.Text())
	require.Equal(t, "You: hello", transcriptCellPlainText(runModel.messages[len(runModel.messages)-3]))
	require.Equal(t, "Hand: hi", transcriptCellPlainText(runModel.messages[len(runModel.messages)-2]))
	require.Equal(t, transcriptCellPlainText(toolTranscriptTestCell("call_1", "read_file", "")), transcriptCellPlainText(runModel.messages[len(runModel.messages)-1]))
	require.Contains(t, content, "❯ hello")
	require.Contains(t, content, "hi")
	require.NotContains(t, content, "Hand: hi")
	require.Contains(t, content, "● Read")
	require.Contains(t, content, "└ read_file")
	require.NotContains(t, content, "older 00")
	require.NotContains(t, content, "stale cell")
	require.True(t, runModel.transcript.AtBottom())
}

func TestModel_HydrateSessionTimelineShowsEmptySession(t *testing.T) {
	runModel := newModel()

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{SessionID: "empty"})

	require.Equal(t, "empty", runModel.sessionTitle)
	require.Equal(t, defaultStatus, runModel.status.Text())
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.NotContains(t, runModel.transcript.View(), "empty has no visible timeline yet.")
}

func TestModel_HydrateSessionTimelineShowsFallbackForMissingSessionID(t *testing.T) {
	runModel := newModel()

	runModel.hydrateSessionTimeline(rpcclient.SessionTimeline{})

	require.Equal(t, "session", runModel.sessionTitle)
	require.Equal(t, defaultStatus, runModel.status.Text())
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.NotContains(t, runModel.transcript.View(), "session has no visible timeline yet.")
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
	setStatusTransient(&runModel.status, "Press Ctrl-C again to exit")
	content := stripANSI(runModel.renderBottomStatusPanel())

	require.Contains(t, content, "Press Ctrl-C again to exit")
	require.NotContains(t, content, "minimax-m2.7")
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
		if len(times) == 0 {
			return time.Date(2026, 5, 16, 9, 0, 1, 0, time.UTC)
		}
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
		if len(times) == 0 {
			return time.Date(2026, 5, 16, 9, 0, 3, 0, time.UTC)
		}
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
	runModel.status.SetTransient("Press Ctrl-C again to exit", startedAt)

	updated, cmd := runModel.Update(exitConfirmationExpiredMsg{startedAt: startedAt})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.exitAt.IsZero())
	require.Equal(t, defaultStatus, runModel.status.Text())
}

func TestModel_UpdateIgnoresStaleCtrlCNoticeTimeout(t *testing.T) {
	runModel := newModel()
	runModel.exitAt = time.Date(2026, 5, 16, 9, 0, 1, 0, time.UTC)
	runModel.status.SetTransient("Press Ctrl-C again to exit", runModel.exitAt)

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
	runModel.context = "64,000 used · 50%"
	runModel.input.SetValue("Summarize tests")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.Nil(t, cmd)

	mainModel := updated.(model)
	require.Empty(t, mainModel.input.Value())
	require.Equal(t, []string{"You: Summarize tests"}, transcriptCellPlainTexts(mainModel.messages))

	content := stripANSI(mainModel.View().Content)
	require.Contains(t, content, "██████")
	require.Contains(t, content, "❯ Summarize tests")
	require.Contains(t, content, "64,000")
	require.Contains(t, content, "used · 50%")
}

func TestModel_UpdateHandlesClearCommand(t *testing.T) {
	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "stale"}, assistantTranscriptCell{text: "stale"}}
	runModel.live = assistantTranscriptCell{text: "live"}
	runModel.stream.Add("live")
	runModel.input.SetValue("/clear")
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.live)
	require.Empty(t, runModel.input.Value())
	require.Empty(t, runModel.stream.Render())
	require.Equal(t, "transcript cleared", runModel.status.Text())
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, stripANSI(runModel.View().Content), emptyUserPromptQuestion)
	require.Contains(t, content, "Welcome, Kennedy")
	require.NotContains(t, content, "You: stale")
	require.NotContains(t, content, "Hand: live")

	updated, cmd = runModel.Update(statusExpiredMsg{startedAt: runModel.status.StartedAt()})
	require.Nil(t, cmd)
	require.Equal(t, defaultStatus, updated.(model).status.Text())
}

func TestModel_UpdateHandlesHelpCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/help")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"Commands: /changelog, /clear, /compact, /copy, /help, /new-chat"}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.input.Value())
	require.Contains(t, stripANSI(runModel.transcript.View()), "Commands: /changelog, /clear, /compact, /copy, /help, /new-chat")
}

func TestModel_UpdateSubmitsDefaultCommandMenuItemForBareSlash(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/")
	runModel.updateCommandMenuForInput("/")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.input.Value())
	require.Zero(t, runModel.commandMenuSelected)
	require.Zero(t, runModel.commandMenuOffset)
	require.Empty(t, runModel.commandMenuPrefix)
	require.True(t, runModel.isCommandViewVisible())
	require.Equal(t, "Changelog", runModel.commandView.TitleLeft)
}

func TestModel_UpdateEscapeClosesCommandView(t *testing.T) {
	runModel := newModel()
	runModel.showCommandView(commandViewPayload{
		TitleLeft:  "Changelog",
		TitleRight: "esc to close",
		Content:    "latest updates",
	})

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.isCommandViewVisible())
	require.Contains(t, stripANSI(runModel.View().Content), inputPrompt+"Ask Hand")
}

func TestCommandViewFrame_UsesProvidedTitleColorsAndContent(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.showCommandView(commandViewPayload{
		TitleIcon:       "◉",
		TitleLeft:       "Release Notes",
		TitleSubtext:    "New things",
		TitleRight:      "esc",
		AccentColor:     "203",
		TitleRightColor: "83",
		Content:         "latest update",
	})

	content := runModel.renderCommandView()
	plain := stripANSI(content)

	require.Contains(t, plain, "◉ Release Notes")
	require.Contains(t, plain, "Release Notes")
	require.Contains(t, plain, " - New things")
	require.Contains(t, plain, "esc")
	require.Contains(t, plain, "latest update")
	require.Contains(t, content, "38;5;203")
	require.Contains(t, content, "38;5;83")
}

func TestCommandViewFrame_UsesDefaultTitleAndMutedSubtitleColors(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.showCommandView(commandViewPayload{
		TitleIcon:    "◉",
		TitleLeft:    "Release Notes",
		TitleSubtext: "New things",
		Content:      "latest update",
	})

	frame := runModel.getCommandViewFrame()
	title := lipgloss.NewStyle().
		Inline(true).
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Render("◉ Release Notes")
	mutedSubtitle := lipgloss.NewStyle().
		Inline(true).
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Render(" - New things")

	require.Equal(t, defaultTUITheme.NoticeForeground, frame.AccentColor)
	require.Contains(t, frame.Title, title)
	require.Contains(t, frame.Title, mutedSubtitle)
}

func TestCommandViewFrame_AddsGapBetweenTitleAndContent(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Release Notes",
		Content:   "latest update",
	})

	lines := strings.Split(stripANSI(runModel.renderCommandView()), "\n")

	require.GreaterOrEqual(t, len(lines), 4)
	require.Contains(t, lines[1], "Release Notes")
	require.NotContains(t, lines[2], "Release Notes")
	require.NotContains(t, lines[2], "latest update")
	require.Contains(t, lines[3], "latest update")
}

func TestCommandViewFrame_UsesComposerBorderColor(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.showCommandView(commandViewPayload{
		TitleLeft:   "Release Notes",
		AccentColor: "203",
		Content:     "latest update",
	})

	frame := runModel.getCommandViewFrame()

	require.Equal(t, defaultTUITheme.InputFrameBorder, frame.BorderColor)
}

func TestCommandViewFrame_ScrollsContent(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.height = 18
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Long Output",
		Content:   strings.Join([]string{"line 1", "line 2", "line 3", "line 4", "line 5", "line 6"}, "\n"),
	})

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.commandViewOffset)
	require.NotContains(t, stripANSI(runModel.renderCommandView()), "line 1")
	require.Contains(t, stripANSI(runModel.renderCommandView()), "line 2")
}

func TestModel_UpdateCopiesCommandViewContent(t *testing.T) {
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
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Changelog",
		Content:   "latest update",
	})

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "latest update", copied)
	require.Equal(t, "command view copied", runModel.status.Text())
}

func TestModel_UpdateCopiesRenderedCommandViewMarkdown(t *testing.T) {
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
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Notes",
		Content:   "## Latest\n\n- Added markdown rendering",
	})

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Mod: tea.ModCtrl}))

	require.NotNil(t, cmd)
	_ = updated.(model)
	require.Contains(t, copied, "Latest")
	require.Contains(t, copied, "Added markdown rendering")
	require.NotContains(t, copied, "## Latest")
	require.NotContains(t, copied, "- Added")
}

func TestModel_UpdateSelectsCommandViewTextWithMouseAndCopiesOnRelease(t *testing.T) {
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
	runModel.width = 80
	runModel.height = 24
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Changelog",
		Content:   "alpha\nbeta",
	})
	row := runModel.getCommandViewContentTop()
	x := runModel.getCommandViewContentLeft()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      x,
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.commandViewSelection.dragging)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      x + len("alpha"),
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Contains(t, runModel.renderCommandView(), "\x1b[7m")

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      x + len("alpha"),
		Y:      row,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.commandViewSelection.dragging)
	require.True(t, runModel.commandViewSelection.active)
	require.Contains(t, runModel.renderCommandView(), "\x1b[7m")
	require.Equal(t, "alpha", copied)
	require.Equal(t, "alpha", runModel.selectedCommandViewText())
}

func TestModel_UpdateAutoScrollsCommandViewSelection(t *testing.T) {
	runModel := newModel()
	runModel.width = 80
	runModel.height = 18
	runModel.showCommandView(commandViewPayload{
		TitleLeft: "Long Output",
		Content: strings.Join([]string{
			"line 1",
			"line 2",
			"line 3",
			"line 4",
			"line 5",
			"line 6",
		}, "\n"),
	})
	top := runModel.getCommandViewContentTop()
	x := runModel.getCommandViewContentLeft()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      x,
		Y:      top,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      x + len("line 6"),
		Y:      top + runModel.getCommandViewContentHeight(),
	}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.commandViewOffset)
	require.Contains(t, runModel.selectedCommandViewText(), "line 4")

	updated, cmd = runModel.Update(commandViewSelectionAutoScrollTickMsg{})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 2, runModel.commandViewOffset)
	require.Contains(t, runModel.selectedCommandViewText(), "line 5")
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
	runModel.messages = []transcriptCell{
		userTranscriptCell{text: "first"},
		assistantTranscriptCell{text: "second"},
		toolTranscriptTestCell("", "read_file", ""),
	}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	firstRow := getTranscriptContentRow(t, runModel, "❯ first")
	secondRow := getTranscriptContentRow(t, runModel, "second")
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
		X:      getPanelHorizontalPadding(runModel.width) + len("second"),
		Y:      secondRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.transcript.View(), "48;5;235")

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("second"),
		Y:      secondRow,
	}))

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.selection.dragging)
	require.True(t, runModel.selection.active)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.transcript.View(), "48;5;235")
	require.Equal(t, strings.Join([]string{
		"❯ first",
		"",
		"second",
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "second"}}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	row := getTranscriptContentRow(t, runModel, "second")

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width),
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("sec"),
		Y:      row,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseReleaseMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("sec"),
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "first"}}
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "first"}}
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "first"}}
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

func TestModel_UpdateKeepsSelectionDragDuringResponseUpdate(t *testing.T) {
	runModel := newModel()
	runModel.height = 40
	runModel.resize()
	runModel.messages = []transcriptCell{
		userTranscriptCell{text: "first"},
		assistantTranscriptCell{text: "second"},
	}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	firstRow := getTranscriptContentRow(t, runModel, "❯ first")
	secondRow := getTranscriptContentRow(t, runModel, "second")

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      firstRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("second"),
		Y:      secondRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")

	runModel.responding = true
	runModel.responseID = 4
	updated, cmd = runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "new response text"},
	})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.selectedTranscriptText(), "first")
	require.Contains(t, runModel.selectedTranscriptText(), "second")
}

func TestModel_UpdateKeepsSelectionDragDuringToolUpdate(t *testing.T) {
	runModel := newModel()
	runModel.height = 40
	runModel.resize()
	runModel.messages = []transcriptCell{
		userTranscriptCell{text: "first"},
		assistantTranscriptCell{text: "second"},
	}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	firstRow := getTranscriptContentRow(t, runModel, "❯ first")
	secondRow := getTranscriptContentRow(t, runModel, "second")

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		Y:      firstRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("second"),
		Y:      secondRow,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)

	updated, cmd = runModel.Update(toolInvocationCompletedMsg{Name: "read_file"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.selection.dragging)
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")
	require.Contains(t, runModel.selectedTranscriptText(), "first")
	require.Contains(t, runModel.selectedTranscriptText(), "second")
}

func TestModel_UpdateAutoScrollsTranscriptSelectionAtBottomEdge(t *testing.T) {
	runModel := newModel()
	runModel.width = 40
	runModel.height = 12
	runModel.resize()
	runModel.transcript.SetWidth(20)
	runModel.transcript.SetHeight(3)
	runModel.transcript.SetContent(strings.Join([]string{
		"line 00",
		"line 01",
		"line 02",
		"line 03",
		"line 04",
		"line 05",
	}, "\n"))
	runModel.transcript.GotoTop()
	top := runModel.getTranscriptTop()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width),
		Y:      top,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width) + len("line 03"),
		Y:      top + runModel.transcript.Height(),
	}))
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.transcript.YOffset())
	require.Contains(t, runModel.selectedTranscriptText(), "line 03")

	updated, cmd = runModel.Update(transcriptSelectionAutoScrollTickMsg{})
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 2, runModel.transcript.YOffset())
	require.Contains(t, runModel.selectedTranscriptText(), "line 04")
}

func TestModel_UpdateAutoScrollsTranscriptSelectionAtTopEdge(t *testing.T) {
	runModel := newModel()
	runModel.width = 40
	runModel.height = 12
	runModel.resize()
	runModel.transcript.SetWidth(20)
	runModel.transcript.SetHeight(3)
	runModel.transcript.SetContent(strings.Join([]string{
		"line 00",
		"line 01",
		"line 02",
		"line 03",
		"line 04",
		"line 05",
	}, "\n"))
	runModel.transcript.SetYOffset(3)
	top := runModel.getTranscriptTop()

	updated, cmd := runModel.Update(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width),
		Y:      top + runModel.transcript.Height() - 1,
	}))
	require.Nil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      getPanelHorizontalPadding(runModel.width),
		Y:      top - 1,
	}))
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 2, runModel.transcript.YOffset())
	require.Contains(t, runModel.selectedTranscriptText(), "line 02")

	updated, cmd = runModel.Update(transcriptSelectionAutoScrollTickMsg{})
	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, 1, runModel.transcript.YOffset())
	require.Contains(t, runModel.selectedTranscriptText(), "line 01")
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
	runModel.messages = []transcriptCell{systemTranscriptCell{text: "   "}}
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "first"}}
	runModel.setTranscriptContent()
	runModel.resize()
	runModel.transcript.GotoTop()
	row := getTranscriptContentRow(t, runModel, "first")

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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "first"}}
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
	line := renderTranscriptTestCell(assistantTranscriptCell{text: "hello"})

	offset := getByteOffsetForDisplayColumn(line, len("hel"))

	require.Equal(t, strings.Index(line, "lo"), offset)
}

func TestHighlightTranscriptSelectionUsesDisplayColumnsForWideCharacters(t *testing.T) {
	line := renderTranscriptTestCell(assistantTranscriptCell{text: "👋 anything"})
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
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "stale"}}
	runModel.setTranscriptContent()
	runModel.selection = transcriptSelection{
		active: true,
		start:  transcriptSelectionPoint{offset: 0},
		end:    transcriptSelectionPoint{offset: len("Hand")},
	}
	runModel.applyTranscriptSelectionStyle()
	require.Contains(t, runModel.transcript.View(), "\x1b[7m")

	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "refreshed"}}
	runModel.setTranscriptContent()

	require.False(t, runModel.selection.active)
	require.Empty(t, runModel.selectedTranscriptText())
	require.Contains(t, stripANSI(runModel.transcript.View()), "refreshed")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: refreshed")
}

func TestModel_UpdateReportsUnknownCommand(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue("/missing now")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.Equal(t, "unknown command: /missing", runModel.status.Text())
	require.Empty(t, runModel.input.Value())
}

func TestModel_HandleSlashCommandReportsEmptyCommand(t *testing.T) {
	runModel := newModel()

	cmd := runModel.handleSlashCommand(composerInput{Kind: composerInputCommand})

	require.NotNil(t, cmd)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.Equal(t, "empty command", runModel.status.Text())
}

func TestModel_SubmitPromptStartsRPCResponse(t *testing.T) {
	client := &fakeTUIChatClient{reply: "hello back"}
	runModel := newModelWithClient(client)
	runModel.input.SetValue("hello")

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.True(t, runModel.responding)
	require.True(t, runModel.thinkingComposerActive)
	require.Equal(t, []string{"You: hello"}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.input.Value())
	require.Equal(t, []string{"hello"}, runModel.history)
	require.Zero(t, client.calls)
	require.NotNil(t, runModel.responseCancel)
}

func TestModel_UpdateEnterStartsThinkingResponse(t *testing.T) {
	client := &fakeTUIChatClient{reply: "hello back"}
	runModel := newModelWithClient(client)
	runModel.input.SetValue("hello")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responding)
	require.True(t, runModel.thinkingComposerActive)
	require.True(t, runModel.isModelThinking())
	require.Contains(t, stripANSI(runModel.renderBottomStatusPanel()), "Thinking")
	require.Equal(t, []string{"You: hello"}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.input.Value())
	require.Zero(t, client.calls)
	require.NotNil(t, runModel.responseCancel)
}

func TestModel_UpdateEscapeCancelsActiveResponse(t *testing.T) {
	responseCtx, cancel := context.WithCancel(context.Background())
	runModel := newModelWithClientContext(responseCtx, &fakeTUIChatClient{})
	runModel.responding = true
	runModel.responseID = 4
	runModel.responseCancel = cancel
	runModel.responseTranscriptFollow = true
	runModel.thinkingComposerActive = true
	runModel.toolAnimationActive = true
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.False(t, runModel.responseTranscriptFollow)
	require.False(t, runModel.thinkingComposerActive)
	require.False(t, runModel.toolAnimationActive)
	require.Nil(t, runModel.responseCancel)
	require.Nil(t, runModel.events)
	require.Equal(t, "response cancelled", runModel.status.Text())
	require.ErrorIs(t, responseCtx.Err(), context.Canceled)
}

func TestModel_UpdateEscapeIgnoresStaleCancelledCompletion(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 4
	runModel.responseCancel = func() {}
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	require.NotNil(t, cmd)
	runModel = updated.(model)

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4, Err: context.Canceled})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.Equal(t, "response cancelled", runModel.status.Text())
}

func TestModel_SubmitPromptPreservesTranscriptOffsetWhenAwayFromBottom(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	offsetBefore := runModel.transcript.YOffset()
	runModel.input.SetValue("hello")

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.Equal(t, offsetBefore, runModel.transcript.YOffset())
	require.False(t, runModel.transcript.AtBottom())
	require.False(t, runModel.responseTranscriptFollow)
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "❯ hello")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "❯ hello")
}

func TestModel_SubmitPromptStartsResponseFollowFromSettledBottom(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
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

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "final")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: final")
}

func TestModel_SubmitPromptFollowsResponseAfterUserScrollsBackToBottom(t *testing.T) {
	runModel := newModelWithClient(&fakeTUIChatClient{})
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	runModel.transcript.GotoTop()
	require.False(t, runModel.transcript.AtBottom())
	runModel.transcript.GotoBottom()
	require.True(t, runModel.transcript.AtBottom())
	runModel.input.SetValue("hello")

	cmd := runModel.submitPrompt()

	require.NotNil(t, cmd)
	require.True(t, runModel.responseTranscriptFollow)
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: runModel.responseID,
		Message:    assistantTextDeltaMsg{Text: strings.Repeat("streamed ", 40)},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateRefreshesSessionTitleAfterResponseCompletes(t *testing.T) {
	client := &fakeTUIChatClient{
		currentSession: storage.Session{
			ID:    "default",
			Title: "Daily Planning",
		},
	}
	runModel := newModelWithClient(client)
	runModel.sessionTitle = defaultSessionTitle
	runModel.responding = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseCompletedMsg{ResponseID: 4, Text: "final"})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, defaultSessionTitle, runModel.sessionTitle)

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)

	var msg tea.Msg
	for _, child := range batch {
		if childMsg, ok := child().(sessionTitleLoadedMsg); ok {
			msg = childMsg
			break
		}
	}
	require.Equal(t, 1, client.currentSessionCalls)
	require.NotNil(t, msg)

	updated, cmd = runModel.Update(msg)

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, defaultSessionID, runModel.sessionID)
	require.Equal(t, "Daily Planning (default)", runModel.sessionTitle)
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
	require.False(t, client.streamSet)
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
	require.Equal(t, "Hand: hello", transcriptCellPlainText(runModel.live))

	updated, cmd = runModel.Update(responseCompletedMsg{ResponseID: 4, Text: "hello final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Nil(t, runModel.events)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"Hand: hello final"}, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdatePreservesTranscriptScrollDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "streamed")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateFollowsBottomDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.Contains(t, stripANSI(runModel.transcript.GetContent()), "streamed")
	require.Contains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateFollowsBottomWhenToolCallGrowsTranscript(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message: toolInvocationStartedMsg{
			ID:     "call_1",
			Name:   "run_command",
			Detail: "printf " + strings.Repeat("long-output ", 40),
		},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "long-output")
}

func TestModel_UpdateKeepsFollowingBottomWhenResponseCompletesAfterStream(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.Contains(t, stripANSI(runModel.transcript.View()), "streamed")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: streamed")
}

func TestModel_UpdateScrollsToBottomWhenResponseCompletesWhileViewportIsAtBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.Contains(t, stripANSI(runModel.transcript.View()), "final")
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Hand: final")
}

func TestModel_UpdateDoesNotScrollToBottomWhenResponseCompletesAfterManualScroll(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.NotContains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateDisablesFollowModeOnWheelDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.NotContains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateReenablesFollowModeWhenUserScrollsBackToBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())
	runModel.responding = true
	runModel.responseTranscriptFollow = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.transcript.AtBottom())
	require.True(t, runModel.responseTranscriptScrolled)
	require.False(t, runModel.responseTranscriptFollow)

	for !runModel.transcript.AtBottom() {
		updated, cmd = runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		require.Nil(t, cmd)
		runModel = updated.(model)
	}
	require.True(t, runModel.responseTranscriptFollow)
	require.False(t, runModel.responseTranscriptScrolled)

	updated, cmd = runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    assistantTextDeltaMsg{Text: "streamed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.transcript.AtBottom())
	require.Contains(t, stripANSI(runModel.transcript.View()), "streamed")
}

func TestModel_UpdateDoesNotScrollToBottomWhenResponseArrivesAwayFromBottom(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	runModel.messages = make([]transcriptCell, 0, 30)
	for index := 0; index < 30; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
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
	require.Equal(t, []string{"Error: daemon unavailable"}, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdateSurfacesProviderErrorAsFriendlyMessage(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 2
	runModel.events = make(chan tea.Msg)

	updated, cmd := runModel.Update(responseCompletedMsg{
		ResponseID: 2,
		Err: errors.New(
			`POST "https://api.anthropic.com/v1/messages": 400 Bad Request ` +
				`{"type":"error","error":{"type":"invalid_request_error","message":"tools.1.custom is not supported"}}`,
		),
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Equal(
		t,
		[]string{"Error: Model provider rejected the request: tools.1.custom is not supported"},
		transcriptCellPlainTexts(runModel.messages),
	)
}

func TestModel_UpdateSuppressesLiveTraceErrorDuringActiveResponse(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseID = 4
	runModel.events = make(chan tea.Msg, 1)

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 4,
		Message:    sessionErrorMsg{Message: "provider failed"},
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responding)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))

	updated, cmd = runModel.Update(responseCompletedMsg{
		ResponseID: 4,
		Err:        errors.New("provider failed"),
	})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.responding)
	require.Equal(t, []string{"Error: provider failed"}, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdateAppliesSessionErrorMessage(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(sessionErrorMsg{Message: "daemon unavailable"})

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, "response failed", runModel.status.Text())
	require.Equal(t, []string{"Error: daemon unavailable"}, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdateIgnoresStaleResponseEvents(t *testing.T) {
	runModel := newModel()
	runModel.responding = false
	runModel.responseID = 3
	runModel.messages = []transcriptCell{assistantTranscriptCell{text: "final"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(responseEventMsg{
		ResponseID: 3,
		Message:    assistantTextDeltaMsg{Text: "late delta"},
	})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"Hand: final"}, transcriptCellPlainTexts(runModel.messages))
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
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
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
		transcriptCellPlainText(toolTranscriptTestCell("", "read_file", "")),
		transcriptCellPlainText(toolTranscriptTestCell("", "read_file", "", true)),
		"Safety: blocked: prompt_exfiltration",
	}, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdateMergesCompletedToolAfterInterleavedSafetyEvent(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseStartMessageIndex = len(runModel.messages)

	for index, msg := range []tea.Msg{
		toolInvocationStartedMsg{ID: "call_1", Name: "web_extract"},
		safetyEventMsg{Action: "blocked", FindingIDs: []string{"invisible_unicode"}},
		toolInvocationCompletedMsg{ID: "call_1", Name: "web_extract"},
	} {
		updated, cmd := runModel.Update(msg)
		if index == 0 {
			require.NotNil(t, cmd)
		} else if index == 2 {
			require.NotNil(t, cmd)
		} else {
			require.Nil(t, cmd)
		}
		runModel = updated.(model)
	}

	require.Equal(t, []string{
		transcriptCellPlainText(toolTranscriptTestCell("call_1", "web_extract", "", true)),
		"Safety: blocked: invisible_unicode",
	}, transcriptCellPlainTexts(runModel.messages))
	require.NotContains(t, stripANSI(runModel.transcript.View()), "Extracting from web")
	require.Contains(t, stripANSI(runModel.transcript.View()), "Extraction finished")
}

func TestModel_UpdateDoesNotMergeCompletedToolBeforeCurrentResponse(t *testing.T) {
	runModel := newModel()
	runModel.messages = []transcriptCell{
		userTranscriptCell{text: "first"},
		toolTranscriptTestCell("call_1", "web_extract", ""),
		assistantTranscriptCell{text: "first done"},
		userTranscriptCell{text: "second"},
	}
	runModel.responding = true
	runModel.responseStartMessageIndex = len(runModel.messages)
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(toolInvocationCompletedMsg{ID: "call_1", Name: "web_extract"})
	require.NotNil(t, cmd)

	runModel = updated.(model)
	require.Equal(t, []string{
		"You: first",
		transcriptCellPlainText(toolTranscriptTestCell("call_1", "web_extract", "")),
		"Hand: first done",
		"You: second",
		transcriptCellPlainText(toolTranscriptTestCell("call_1", "web_extract", "", true)),
	}, transcriptCellPlainTexts(runModel.messages))
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

	runModel.live = assistantTranscriptCell{text: "hello"}
	updated, cmd = runModel.Update(thinkingComposerTickMsg{})
	require.Nil(t, cmd)
	runModel = updated.(model)
	require.False(t, runModel.thinkingComposerActive)
	require.Equal(t, "8", runModel.getInputFrameBorderColor())
}

func TestModel_ThinkingComposerBorderWaitsForRunningTool(t *testing.T) {
	runModel := newModel()
	runModel.responding = true
	runModel.responseRunningToolCount = 1
	runModel.messages = []transcriptCell{toolTranscriptTestCell("call_1", "web_search", "")}

	require.False(t, runModel.isThinkingComposerVisible())
	require.Equal(t, "8", runModel.getInputFrameBorderColor())

	runModel.responseRunningToolCount = 0
	runModel.messages = []transcriptCell{toolTranscriptTestCell("call_1", "web_search", "", true)}
	require.True(t, runModel.isThinkingComposerVisible())
	require.Equal(t, getThinkingComposerBorderColor(0), runModel.getInputFrameBorderColor())
}

func TestModel_ThinkingComposerIgnoresStaleRunningToolCells(t *testing.T) {
	client := &fakeTUIChatClient{reply: "hello back"}
	runModel := newModelWithClient(client)
	runModel.messages = []transcriptCell{toolTranscriptTestCell("old_call", "web_search", "")}
	runModel.setTranscriptContent()
	runModel.input.SetValue("hello")

	updated, cmd := runModel.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, cmd)
	runModel = updated.(model)
	require.True(t, runModel.responding)
	require.True(t, runModel.isModelThinking())
	require.True(t, runModel.isThinkingComposerVisible())
	require.Contains(t, stripANSI(runModel.renderBottomStatusPanel()), "Thinking")
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
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
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
	require.Equal(t, []string{"Commands: /changelog, /clear, /compact, /copy, /help, /new-chat"}, transcriptCellPlainTexts(runModel.messages))
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
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "first line\npartial"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Equal(t, []string{"You: hello"}, transcriptCellPlainTexts(runModel.messages))
	require.Equal(t, "Hand: first line\npartial", transcriptCellPlainText(runModel.live))
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "❯ hello")
	require.Contains(t, content, "first line")
	require.NotContains(t, content, "Hand: first line")
	require.Contains(t, content, "partial")
}

func TestModel_UpdateConvertsLiveAssistantCellToHistoryAtCompletion(t *testing.T) {
	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "first line\npartial"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"You: hello", "Hand: first line\npartial"}, transcriptCellPlainTexts(runModel.messages))
	require.Equal(t, "", runModel.stream.Render())
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "first line")
	require.NotContains(t, content, "Hand: first line")
	require.Contains(t, content, "partial")
}

func TestModel_UpdateRendersReasoningDeltasOutsideAssistantStream(t *testing.T) {
	now := time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)
	originalCurrentTime := currentTime
	t.Cleanup(func() { currentTime = originalCurrentTime })
	currentTime = func() time.Time { return now }

	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Channel: "reasoning", Text: "first "})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantTextDeltaMsg{Channel: "reasoning", Text: "token"})
	require.Nil(t, cmd)
	now = now.Add(3 * time.Second)
	updated, cmd = updated.(model).Update(assistantTextDeltaMsg{Text: "answer"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{})
	require.Nil(t, cmd)

	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{
		"You: hello",
		"Thought: 3s",
		"Hand: answer",
	}, transcriptCellPlainTexts(runModel.messages))
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "Thought for 3s")
	require.Contains(t, content, "answer")
	require.NotContains(t, content, "Reasoning:")
	require.NotContains(t, content, "first token")
}

func TestModel_UpdateReasoningCompletedCollapsesEarlierThinkingCell(t *testing.T) {
	now := time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)
	originalCurrentTime := currentTime
	t.Cleanup(func() { currentTime = originalCurrentTime })
	currentTime = func() time.Time { return now }

	runModel := newModel()
	runModel.responding = true
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Channel: "reasoning", Text: "checking messages"})
	require.Nil(t, cmd)
	now = now.Add(5 * time.Second)
	updated, cmd = updated.(model).Update(toolInvocationStartedMsg{
		ID:   "call_1",
		Name: "session_messages",
	})
	require.NotNil(t, cmd)
	updated, cmd = updated.(model).Update(toolInvocationCompletedMsg{
		ID:   "call_1",
		Name: "session_messages",
	})
	require.NotNil(t, cmd)
	updated, cmd = updated.(model).Update(assistantTextDeltaMsg{Channel: "reasoning", Text: "checking again"})
	require.Nil(t, cmd)
	now = now.Add(17 * time.Second)
	updated, cmd = updated.(model).Update(reasoningCompletedMsg{Duration: 17 * time.Second})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{Text: "done"})
	require.Nil(t, cmd)

	runModel = updated.(model)
	require.Equal(t, []string{
		"You: hello",
		"Thought: 5s",
		transcriptCellPlainText(toolTranscriptTestCell("call_1", "session_messages", "", true)),
		"Thought: 17s",
		"Hand: done",
	}, transcriptCellPlainTexts(runModel.messages))
	content := stripANSI(runModel.transcript.View())
	require.Contains(t, content, "Thought for 5s")
	require.Contains(t, content, "Thought for 17s")
	require.Contains(t, content, "Fetched Session Messages")
	require.NotContains(t, content, "Thinking")
	require.NotContains(t, content, "checking messages")
	require.NotContains(t, content, "checking again")
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
	require.Equal(t, []string{transcriptCellPlainText(live)}, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.live)
}

func TestModel_UpdateUsesFinalAssistantTextAtCompletion(t *testing.T) {
	runModel := newModel()
	runModel.messages = []transcriptCell{userTranscriptCell{text: "hello"}}
	runModel.setTranscriptContent()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "draft"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{Text: "final"})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Equal(t, []string{"You: hello", "Hand: final"}, transcriptCellPlainTexts(runModel.messages))
	require.NotContains(t, stripANSI(runModel.transcript.View()), "draft")
}

func TestModel_UpdatePreservesFinalAssistantWhitespace(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{Text: "draft"})
	require.Nil(t, cmd)
	updated, cmd = updated.(model).Update(assistantResponseCompletedMsg{Text: "final\n\n"})

	require.Nil(t, cmd)
	require.Equal(t, []string{"Hand: final\n\n"}, transcriptCellPlainTexts(updated.(model).messages))
}

func TestModel_UpdateIgnoresEmptyAssistantDelta(t *testing.T) {
	runModel := newModel()

	updated, cmd := runModel.Update(assistantTextDeltaMsg{})

	require.Nil(t, cmd)
	require.Empty(t, updated.(model).live)
	require.Empty(t, transcriptCellPlainTexts(updated.(model).messages))
}

func TestModel_UpdateClearsEmptyAssistantCompletion(t *testing.T) {
	runModel := newModel()
	runModel.live = assistantTranscriptCell{text: "draft"}
	runModel.stream.Add("   ")

	updated, cmd := runModel.Update(assistantResponseCompletedMsg{})

	require.Nil(t, cmd)
	runModel = updated.(model)
	require.Empty(t, runModel.live)
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
	require.Empty(t, runModel.stream.Render())
}

func TestAssistantTranscriptCell_IgnoresBlankText(t *testing.T) {
	require.True(t, assistantTranscriptCell{text: " \n\t "}.IsEmpty())
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
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
}

func TestModel_UpdateInsertsPromptNewlineOnTerminalModifiedEnterFallbacks(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{
			name: "alt_enter",
			key: tea.KeyPressMsg{
				Code: tea.KeyEnter,
				Mod:  tea.ModAlt,
			},
		},
		{
			name: "ctrl_j",
			key: tea.KeyPressMsg{
				Code: 'j',
				Mod:  tea.ModCtrl,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runModel := newModel()
			runModel.input.SetValue("first line")

			updated, cmd := runModel.Update(tt.key)

			if cmd != nil {
				cmd()
			}
			runModel = updated.(model)
			require.Equal(t, "first line\n", runModel.input.Value())
			require.Equal(t, 2, runModel.input.Height())
			require.Empty(t, transcriptCellPlainTexts(runModel.messages))
		})
	}
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
			require.Empty(t, transcriptCellPlainTexts(runModel.messages))
		})
	}
}

func TestModel_UpdateGrowsPromptForWrappedText(t *testing.T) {
	runModel := newModel()
	runModel.input.SetValue(strings.Repeat("a", getInputInnerWidth(runModel.width)+1))
	runModel.resize()

	require.Equal(t, 2, runModel.input.Height())
}

func TestModel_UpdateKeepsTranscriptAtBottomWhenPromptWrapGrowsComposer(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	for index := 0; index < 12; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd := runModel.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.NotNil(t, cmd)
	runModel = updated.(model)

	for i := 0; i < getInputInnerWidth(runModel.width)+2; i++ {
		updated, cmd = runModel.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		require.NotNil(t, cmd)
		runModel = updated.(model)
	}

	require.Greater(t, runModel.input.Height(), 1)
	require.True(t, runModel.transcript.AtBottom())
	require.NotContains(t, stripANSI(runModel.View().Content), jumpToBottomLabel)
}

func TestModel_UpdateKeepsTranscriptAtBottomWhenNewlineGrowsComposer(t *testing.T) {
	runModel := newModel()
	runModel.height = 10
	runModel.resize()
	for index := 0; index < 12; index++ {
		runModel.messages = append(runModel.messages, systemTranscriptCell{text: fmt.Sprintf("Message %02d", index)})
	}
	runModel.setTranscriptContent()
	runModel.input.SetValue("first line")
	require.True(t, runModel.transcript.AtBottom())

	updated, cmd := runModel.Update(tea.KeyPressMsg{
		Code: tea.KeyEnter,
		Mod:  tea.ModShift,
	})
	if cmd != nil {
		cmd()
	}
	runModel = updated.(model)

	require.Equal(t, 2, runModel.input.Height())
	require.True(t, runModel.transcript.AtBottom())
	require.NotContains(t, stripANSI(runModel.View().Content), jumpToBottomLabel)
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
	require.Empty(t, transcriptCellPlainTexts(runModel.messages))
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

func setActiveTestProfile(t *testing.T, home string) {
	t.Helper()

	original := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(original)
	})

	profile.SetActive(profile.Profile{Name: profile.DefaultName, HomeDir: home})
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

type fakeTUIChatClient struct {
	events              []rpcclient.Event
	reply               string
	err                 error
	compactResult       rpcclient.CompactSessionResult
	compactErr          error
	compactID           string
	createdSession      storage.Session
	createSessionErr    error
	createSessionID     string
	timeline            rpcclient.SessionTimeline
	timelineErr         error
	currentSession      storage.Session
	currentSessionErr   error
	contextStatus       rpcclient.ContextStatus
	contextErr          error
	contextSessionID    string
	message             string
	stream              bool
	streamSet           bool
	calls               int
	compactCalls        int
	createSessionCalls  int
	timelineCalls       int
	currentSessionCalls int
	contextCalls        int
	closed              bool
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
		c.streamSet = true
	}
	for _, event := range c.events {
		if opts.OnEvent != nil {
			opts.OnEvent(event)
		}
	}

	return c.reply, c.err
}

func (c *fakeTUIChatClient) CompactSession(_ context.Context, id string) (rpcclient.CompactSessionResult, error) {
	c.compactCalls++
	c.compactID = id
	return c.compactResult, c.compactErr
}

func (c *fakeTUIChatClient) CreateSession(_ context.Context, id string) (storage.Session, error) {
	c.createSessionCalls++
	c.createSessionID = id
	return c.createdSession, c.createSessionErr
}

func (c *fakeTUIChatClient) GetSessionTimeline(
	_ context.Context,
	_ rpcclient.SessionTimelineOptions,
) (rpcclient.SessionTimeline, error) {
	c.timelineCalls++
	return c.timeline, c.timelineErr
}

func (c *fakeTUIChatClient) CurrentSession(context.Context) (storage.Session, error) {
	c.currentSessionCalls++
	return c.currentSession, c.currentSessionErr
}

func (c *fakeTUIChatClient) GetSession(_ context.Context, id string) (rpcclient.ContextStatus, error) {
	c.contextCalls++
	c.contextSessionID = id
	return c.contextStatus, c.contextErr
}

func (c *fakeTUIChatClient) Close() error {
	c.closed = true
	return nil
}

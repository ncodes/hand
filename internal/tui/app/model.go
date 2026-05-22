package tui

import (
	"context"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	transcriptComposerGapHeight = 1
	baseInputChromeHeight       = inputFrameChromeHeight + bottomStatusPanelHeight + transcriptComposerGapHeight
)

// model is the root Bubble Tea application state for the interactive shell.
type model struct {
	transcript viewport.Model
	input      textarea.Model
	tuiState
	chatClient rpcclient.ChatAPI
	timeline   sessionTimelineLoader
	title      sessionTitleLoader
	chatCtx    context.Context
	events     <-chan tea.Msg
}

// newModel builds the initial TUI state and sizes child components.
func newModel() model {
	return newModelWithClientContext(context.Background(), nil)
}

func newModelWithClient(client rpcclient.ChatAPI) model {
	return newModelWithClientContext(context.Background(), client)
}

func newModelWithClientContext(ctx context.Context, client rpcclient.ChatAPI) model {
	return newModelWithClientContextAndConfig(ctx, client, nil)
}

func newModelWithClientContextAndConfig(ctx context.Context, client rpcclient.ChatAPI, cfg *config.Config) model {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		cfg = config.NewDefaultConfig()
	}

	history, err := loadPromptHistory()
	appModel := model{
		transcript: newTranscript(),
		input:      newInputComposer(),
		tuiState:   newTUIState(history, cfg.TUIThinkingComposerEnabled()),
		chatClient: client,
		chatCtx:    ctx,
	}
	if timeline, ok := client.(sessionTimelineLoader); ok {
		appModel.timeline = timeline
	}
	if title, ok := client.(sessionTitleLoader); ok {
		appModel.title = title
	}
	if err != nil {
		setStatusTransient(&appModel.status, "prompt history unavailable")
	}
	appModel.resize()
	appModel.setTranscriptContent()

	return appModel
}

func NewModelWithClientContextAndConfig(
	ctx context.Context,
	client rpcclient.ChatAPI,
	cfg *config.Config,
) tea.Model {
	return newModelWithClientContextAndConfig(ctx, client, cfg)
}

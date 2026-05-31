package tui

import (
	"context"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
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
	nameInput  textinput.Model
	renameInput textinput.Model
	tuiState
	chatClient    rpcclient.ChatAPI
	sessionClient rpcclient.SessionAPI
	timeline      sessionTimelineLoader
	title         sessionTitleLoader
	contextLoader sessionContextLoader
	chatCtx       context.Context
	events        <-chan tea.Msg
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
	userName, userNameSet, namePromptEnabled, userNameErr := loadProfileUserName()
	runtimeInfo := runtimeInfoFromConfig(cfg)
	appModel := model{
		transcript: newTranscript(),
		input:      newInputComposer(),
		nameInput:  newNameInput(),
		renameInput: newChatRenameInput(),
		tuiState:   newTUIState(history, cfg.TUIThinkingComposerEnabled()),
		chatClient: client,
		chatCtx:    ctx,
	}
	if userNameSet {
		appModel.userName = userName
	}
	appModel.namePromptEnabled = namePromptEnabled
	appModel.runtimeInfo = runtimeInfo
	appModel.modelName = getModelDisplayName(runtimeInfo.Model)
	if sessions, ok := client.(rpcclient.SessionAPI); ok {
		appModel.sessionClient = sessions
	}
	if provider, ok := client.(interface{ SessionAPI() rpcclient.SessionAPI }); ok {
		appModel.sessionClient = provider.SessionAPI()
	}
	if timeline, ok := appModel.sessionClient.(sessionTimelineLoader); ok {
		appModel.timeline = timeline
	}
	if title, ok := appModel.sessionClient.(sessionTitleLoader); ok {
		appModel.title = title
	}
	if contextLoader, ok := appModel.sessionClient.(sessionContextLoader); ok {
		appModel.contextLoader = contextLoader
	}
	if err != nil {
		setStatusTransient(&appModel.status, "prompt history unavailable")
	}
	if userNameErr != nil {
		setStatusTransient(&appModel.status, "user profile unavailable")
	}
	appModel.resize()
	appModel.setTranscriptContent()

	return appModel
}

// NewModelWithClientContextAndConfig returns a model with client context and config.
func NewModelWithClientContextAndConfig(
	ctx context.Context,
	client rpcclient.ChatAPI,
	cfg *config.Config,
) tea.Model {
	return newModelWithClientContextAndConfig(ctx, client, cfg)
}

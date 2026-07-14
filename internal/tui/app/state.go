package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/morph/internal/config"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	storage "github.com/wandxy/morph/internal/state/core"
)

type tuiState struct {
	width                      int
	height                     int
	status                     statusModel
	sessionID                  string
	sessionTitle               string
	userName                   string
	namePromptEnabled          bool
	setupNamePromptActive      bool
	setupDismissible           bool
	setupOAuthPending          bool
	setupOAuthProvider         string
	setupOAuthCancel           context.CancelFunc
	setupPullCancel            context.CancelFunc
	setupPullEvents            <-chan tea.Msg
	namePromptError            string
	namePromptErrorStartedAt   time.Time
	modelName                  string
	runtimeInfo                runtimeInfo
	context                    string
	messages                   []transcriptCell
	live                       transcriptCell
	showIntro                  bool
	stream                     markdownStreamCollector
	reasoningStartedAt         time.Time
	reasoningMessageIndex      int
	reasoningMessageIndices    []int
	history                    []string
	historyAt                  int
	draft                      string
	responding                 bool
	responseID                 int
	responseCancel             context.CancelFunc
	responseTranscriptFollow   bool
	responseTranscriptScrolled bool
	responseStartMessageIndex  int
	responseStartedAt          time.Time
	responseRunningToolCount   int
	responseEventStreamActive  bool
	pendingResponseCompletion  *responseCompletedMsg
	toolAnimationFrame         int
	toolAnimationActive        bool
	thinkingComposerFrame      int
	thinkingComposerActive     bool
	thinkingComposerEnabled    bool
	manualCompactionActive     bool
	manualCompactionIndex      int
	chatSwitching              bool
	commandMenuOffset          int
	commandMenuSelected        int
	commandMenuPrefix          string
	commandView                commandViewState
	commandViewOffset          int
	commandViewSelection       commandViewSelection
	commandViewItemSelected    int
	setupModelStep             string
	setupAuthMethod            string
	setupProviders             []rpcclient.ProviderOption
	setupModels                []rpcclient.ModelOption
	setupModelProvider         string
	setupModelBaseURL          string
	setupProviderAPIKey        string
	setupPendingModelID        string
	setupNoticeTitle           string
	setupNoticeMessage         string
	setupNoticeHint            string
	setupNoticeAction          string
	setupItemSelected          int
	setupOffset                int
	configEnvPath              string
	configPath                 string
	setupSavedConfig           *config.Config
	chatsArchiveConfirm        bool
	chatsRenaming              bool
	chatsRenameSessionID       string
	exitAt                     time.Time
	allowShell                 bool
	selection                  transcriptSelection
	pendingApprovalID          string
	pendingApprovalAlways      bool
	pendingApprovalOrder       []string
	pendingApprovalMessages    map[string]permissionApprovalMsg
	approvalMessageIndices     map[string]int
}

type commandViewState struct {
	Visible         bool
	Kind            string
	TitleIcon       string
	TitleLeft       string
	TitleSubtext    string
	TitleRight      string
	AccentColor     string
	TitleRightColor string
	Content         string
	Height          int
	Chats           []storage.Session
	Models          []rpcclient.ModelOption
	Providers       []rpcclient.ProviderOption
	ModelProvider   string
	ModelAuthType   string
	PendingModelID  string
}

type commandViewSelection struct {
	active   bool
	dragging bool
	content  string
	start    transcriptSelectionPoint
	end      transcriptSelectionPoint
	mouse    tea.Mouse
	scroll   int
	ticking  bool
}

func newTUIState(history []string, thinkingComposerEnabled bool) tuiState {
	return tuiState{
		width:                    defaultWidth,
		height:                   defaultHeight,
		status:                   newStatusModel(),
		sessionID:                defaultSessionID,
		sessionTitle:             defaultSessionTitle,
		runtimeInfo:              defaultRuntimeInfo(),
		showIntro:                true,
		reasoningMessageIndex:    -1,
		manualCompactionIndex:    -1,
		history:                  history,
		historyAt:                len(history),
		thinkingComposerEnabled:  thinkingComposerEnabled,
		responseTranscriptFollow: false,
		approvalMessageIndices:   make(map[string]int),
		pendingApprovalMessages:  make(map[string]permissionApprovalMsg),
	}
}

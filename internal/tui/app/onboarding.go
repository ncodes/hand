package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"
	"gopkg.in/yaml.v3"

	clisetup "github.com/wandxy/morph/internal/cli/setup"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	appcredential "github.com/wandxy/morph/internal/credential"
	modelcatalog "github.com/wandxy/morph/internal/model"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	_ "github.com/wandxy/morph/internal/model/provider_anthropic"
	_ "github.com/wandxy/morph/internal/model/provider_copilot"
	_ "github.com/wandxy/morph/internal/model/provider_openai"
	"github.com/wandxy/morph/internal/profile"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
)

const (
	userNameFilename        = "user.json"
	namePromptTitle         = "Hi, there ☺"
	namePromptPlaceholder   = "What can I call you?"
	namePromptSubmitHint    = "Enter to send →"
	namePromptInvalidHint   = "Use letters, numbers, and hyphen only"
	emptyUserPromptQuestion = "What can I do for you?"
	namePromptMaxWidth      = 52
	namePromptInputMinWidth = 28
	namePromptErrorWindow   = 2 * time.Second

	setupModelStepAuthMethod = "auth-method"
	setupModelStepProvider   = "provider"
	setupModelStepModel      = "model"
	setupModelStepAPIKey     = "api-key"
	setupModelStepNotice     = "notice"

	setupAuthMethodSubscription = "subscription"
	setupAuthMethodAPIKey       = "api-key"

	setupModelMaxWidth      = 72
	setupModelMinWidth      = 34
	setupModelLoginMinWidth = 52
	setupModelMaxListHeight = 8
	setupModelFilterWidth   = 18
	setupCloseHint          = "ctrl+x to close"
)

var validUserName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

var (
	getSetupSubscriptionProvider = appcredential.GetSubscriptionProvider
	newSetupCredentialStore      = func() setupCredentialStore {
		return appcredential.NewFileStore("")
	}
)

var setupAuthMethodOptions = []setupAuthMethodOption{
	{
		ID:          setupAuthMethodSubscription,
		Label:       "Use a subscription",
		Description: "Connect with your Anthropic, OpenAI or Github account",
	},
	{
		ID:          setupAuthMethodAPIKey,
		Label:       "Use an API Key",
		Description: "Connect with your own Anthropic, OpenAI, OpenRouter API Keys (BYOK)",
	},
}

type namePromptErrorExpiredMsg struct {
	startedAt time.Time
}

type setupAuthMethodOption struct {
	ID          string
	Label       string
	Description string
}

type profileUser struct {
	Name string `json:"name"`
}

type setupCredentialStore interface {
	Set(string, appcredential.StoredCredential) error
}

type setupOAuthOutputMsg struct {
	provider string
	reader   *io.PipeReader
	line     string
	err      error
}

type setupOAuthCompletedMsg struct {
	provider string
	output   string
	err      error
}

type setupModelOptionsLoadedMsg struct {
	provider        string
	baseURL         string
	selectedModelID string
	models          []rpcclient.ModelOption
	err             error
}

func newNameInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = namePromptPlaceholder
	input.CharLimit = 80
	input.SetWidth(namePromptMaxWidth - 4)
	input.Focus()

	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		UnsetBackground()
	styles.Focused.Placeholder = styles.Focused.Placeholder.
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		UnsetBackground()
	styles.Focused.Prompt = styles.Focused.Prompt.
		UnsetBackground()
	input.SetStyles(styles)

	return input
}

func loadProfileUserName() (string, bool, bool, error) {
	path := profileUserPath()
	if path == "" {
		return noticeBarName, true, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, true, nil
		}

		return "", false, false, fmt.Errorf("read user profile: %w", err)
	}

	var user profileUser
	if err := json.Unmarshal(data, &user); err != nil {
		return "", false, false, fmt.Errorf("parse user profile: %w", err)
	}

	name := strings.TrimSpace(user.Name)
	return name, name != "", false, nil
}

func saveProfileUserName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	path := profileUserPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create profile metadata dir: %w", err)
	}

	data, err := json.MarshalIndent(profileUser{Name: name}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func profileUserPath() string {
	active := profile.WithMetadataPaths(profile.Active())
	home := strings.TrimSpace(active.HomeDir)
	if home == "" {
		return ""
	}

	return filepath.Join(home, userNameFilename)
}

func (m model) shouldShowNamePrompt() bool {
	if !m.namePromptEnabled {
		return false
	}
	if m.setupNamePromptActive {
		return true
	}

	return strings.TrimSpace(m.userName) == "" &&
		len(m.messages) == 0 &&
		(m.live == nil || m.live.IsEmpty())
}

func (m model) shouldShowEmptyUserPrompt() bool {
	return !m.shouldShowNamePrompt() &&
		!m.shouldShowProfileModelSetup() &&
		strings.TrimSpace(m.userDisplayName()) != "" &&
		len(m.messages) == 0 &&
		(m.live == nil || m.live.IsEmpty())
}

func (m model) userDisplayName() string {
	if name := strings.TrimSpace(m.userName); name != "" {
		return name
	}

	return noticeBarName
}

func (m model) renderNamePrompt() string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	height := max(m.transcript.Height(), 1)
	boxWidth := min(max(width/2, namePromptInputMinWidth), min(namePromptMaxWidth, width))
	inputWidth := max(boxWidth-4, 1)
	input := m.nameInput
	input.SetWidth(inputWidth)

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render(namePromptTitle)
	mark := renderMorphBanner(morphHeaderMark)
	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Padding(0, 1).
		Width(boxWidth).
		Render(input.View())
	hintText := namePromptSubmitHint
	hintColor := defaultTUITheme.MutedText
	if errorText := strings.TrimSpace(m.namePromptError); errorText != "" {
		hintText = errorText
		hintColor = defaultTUITheme.ToolDeletion
	} else if m.setupNamePromptActive {
		hintText = "Enter to continue"
	}
	hint := m.renderNamePromptHint(hintText, hintColor, lipgloss.Width(inputBox))
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		mark,
		"",
		title,
		"",
		inputBox,
		hint,
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m model) renderEmptyUserPrompt() string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	header := strings.Trim(m.renderHeaderWithWidth(width), "\n")
	headerHeight := lipgloss.Height(header)
	height := max(m.transcript.Height()-headerHeight, 1)
	name := m.userDisplayName()
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render("Hi, " + name + " ☺")
	question := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Render(emptyUserPromptQuestion)
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		question,
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m model) renderNamePromptHint(hintText string, hintColor string, width int) string {
	width = max(width, 1)
	hintText = strings.TrimSpace(hintText)
	if m.setupDismissible {
		return renderProfileModelSetupSplitHint(hintText, setupCloseHint, width)
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(hintColor)).
		Width(width).
		Render(renderProfileModelSetupPaddedLabel(hintText, width))
}

func (m model) renderEmptyUserPromptContent() string {
	width := m.transcript.Width()
	if width <= 0 {
		width = m.getMainPaneWidth()
	}
	header := strings.Trim(m.renderHeaderWithWidth(width), "\n")

	return strings.TrimRight(lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.renderEmptyUserPrompt(),
	), "\n")
}

func (m model) handleNamePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.setupDismissible && isSetupCloseKey(msg) {
		return m.closeProfileSetup()
	}
	if msg.Key().Code == tea.KeyEsc && m.setupDismissible {
		return m.closeProfileSetup()
	}
	if msg.Key().Code == tea.KeyEnter {
		return m.submitNamePrompt()
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)

	return m, cmd
}

func (m model) handleNamePromptPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	m.nameInput.SetValue(m.nameInput.Value() + normalizeComposerPaste(msg.Content))

	return m, nil
}

func (m model) submitNamePrompt() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return m.setNamePromptError("name is required")
	}
	if !validUserName.MatchString(name) {
		return m.setNamePromptError(namePromptInvalidHint)
	}
	if err := saveProfileUserName(name); err != nil {
		return m, m.setStatus("name unavailable")
	}

	m.userName = name
	m.namePromptEnabled = false
	startSetup := m.setupNamePromptActive || m.profileModelSetupMissing()
	m.setupNamePromptActive = false
	m.nameInput.SetValue("")
	if startSetup {
		return m, m.startProfileModelSetup()
	}

	m.resize()
	m.setTranscriptContent()

	return m, m.setStatus("name saved")
}

func (m model) setNamePromptError(text string) (tea.Model, tea.Cmd) {
	m.namePromptError = strings.TrimSpace(text)
	m.namePromptErrorStartedAt = currentTime()
	startedAt := m.namePromptErrorStartedAt

	return m, tea.Tick(namePromptErrorWindow, func(time.Time) tea.Msg {
		return namePromptErrorExpiredMsg{startedAt: startedAt}
	})
}

func (m model) expireNamePromptError(msg namePromptErrorExpiredMsg) tea.Model {
	if m.namePromptErrorStartedAt.IsZero() || !m.namePromptErrorStartedAt.Equal(msg.startedAt) {
		return m
	}

	m.namePromptError = ""
	m.namePromptErrorStartedAt = time.Time{}
	return m
}

func (m *model) startProfileModelSetup() tea.Cmd {
	m.setupNamePromptActive = false
	m.setupModelStep = setupModelStepAuthMethod
	m.setupAuthMethod = ""
	m.setupProviders = nil
	m.setupModels = nil
	m.setupModelProvider = ""
	m.setupModelBaseURL = ""
	m.setupProviderAPIKey = ""
	m.setupPendingModelID = ""
	m.setupItemSelected = 0
	m.setupOffset = 0
	m.resize()

	return m.setStatus("choose an auth method")
}

func (m *model) startProfileSetup(dismissible bool) tea.Cmd {
	name := strings.TrimSpace(m.userName)
	m.clearProfileModelSetup()
	m.namePromptEnabled = true
	m.setupNamePromptActive = true
	m.setupDismissible = dismissible
	m.namePromptError = ""
	m.namePromptErrorStartedAt = time.Time{}
	m.nameInput = newNameInput()
	m.nameInput.SetValue(name)
	m.nameInput.CursorEnd()
	m.resize()

	return m.setStatus("enter your name")
}

func (m *model) selectCurrentSetupAuthMethodOption() (tea.Model, tea.Cmd) {
	if len(setupAuthMethodOptions) == 0 {
		return *m, nil
	}

	selected := min(max(m.setupItemSelected, 0), len(setupAuthMethodOptions)-1)
	m.setupAuthMethod = setupAuthMethodOptions[selected].ID
	providers := modelcatalog.ListProviders(modelcatalog.ProviderQuery{
		Current:    m.loadRawProfileMainProvider(),
		OAuthOnly:  m.setupAuthMethod == setupAuthMethodSubscription,
		APIKeyOnly: m.setupAuthMethod == setupAuthMethodAPIKey,
	})
	providers = filterSetupProvidersForAuthMethod(providers, m.setupAuthMethod)
	if len(providers) == 0 {
		m.setupAuthMethod = ""
		return *m, m.setStatus("model setup unavailable")
	}

	m.setupModelStep = setupModelStepProvider
	m.setupProviders = providers
	m.setupModels = nil
	m.setupModelProvider = ""
	m.setupProviderAPIKey = ""
	m.setupPendingModelID = ""
	m.setupItemSelected = 0
	m.setupOffset = 0
	m.resize()

	return *m, m.setStatus("choose a model provider")
}

func filterSetupProvidersForAuthMethod(
	providers []modelcatalog.ProviderOption,
	authMethod string,
) []modelcatalog.ProviderOption {
	if authMethod != setupAuthMethodAPIKey {
		return providers
	}

	filtered := make([]modelcatalog.ProviderOption, 0, len(providers))
	for _, provider := range providers {
		switch strings.TrimSpace(strings.ToLower(provider.ID)) {
		case constants.ModelProviderOpenAICodex, constants.ModelProviderGitHubCopilot:
			continue
		default:
			filtered = append(filtered, provider)
		}
	}

	return filtered
}

func (m *model) selectCurrentSetupProviderOption() (tea.Model, tea.Cmd) {
	if len(m.setupProviders) == 0 {
		return *m, nil
	}

	provider := m.setupProviders[m.setupItemSelected]
	providerID := strings.TrimSpace(provider.ID)
	if providerID == "" {
		return *m, m.setStatus("provider selection unavailable")
	}

	rawConfig := m.loadRawProfileConfig()
	opts := clisetup.ModelOptions{
		Provider:  providerID,
		Current:   strings.TrimSpace(rawConfig.Models.Main.Name),
		OAuthOnly: m.setupAuthMethod == setupAuthMethodSubscription,
		Config:    rawConfig,
	}
	baseURL := clisetup.ResolveModelOptionsBaseURL(opts)
	opts.BaseURL = baseURL
	models, _, err := clisetup.ListModelOptions(m.chatCtx, opts)
	if err != nil {
		return *m, m.setStatus("models unavailable")
	}
	if len(models) == 0 {
		return *m, m.setStatus("models unavailable")
	}

	m.setupModels = models
	m.setupModelProvider = providerID
	m.setupModelBaseURL = baseURL
	m.modelFilterInput = newModelFilterInput()
	m.setupItemSelected = 0
	m.setupOffset = 0

	if m.setupAuthMethod == setupAuthMethodAPIKey {
		return m.showSetupProviderAPIKeyPromptForProvider(providerID)
	}
	if m.setupAuthMethod == setupAuthMethodSubscription {
		if err := m.checkSetupModelAuth(models[0]); err == nil {
			return m.showSetupModelSelection()
		} else if !isMissingModelCredentialError(err) {
			return m.showSetupNotice("Authentication unavailable", err.Error(), "enter to continue")
		}

		return m.startSetupOAuthLogin(providerID)
	}

	return m.showSetupModelSelection()
}

func (m *model) selectCurrentSetupModelOption() (tea.Model, tea.Cmd) {
	models := m.filteredSetupModels()
	if len(models) == 0 {
		return *m, nil
	}

	option := models[min(max(m.setupItemSelected, 0), len(models)-1)]
	apiKey := strings.TrimSpace(m.setupProviderAPIKey)
	if apiKey == "" {
		if err := m.checkSetupModelAuth(option); err != nil {
			if isMissingModelCredentialError(err) {
				if option.SupportsOAuth {
					return m.startSetupOAuthLogin(getSetupModelProvider(m.setupModelProvider, option))
				}

				return m.showSetupProviderAPIKeyPrompt(option)
			}

			return m.showSetupNotice("Model setup unavailable", err.Error(), "enter to continue")
		}
	}

	err := m.persistSetupModelSelection(option, apiKey)
	if err == nil {
		return m.completeSetupModelSelection(option)
	}
	if option.SupportsOAuth && isMissingModelCredentialError(err) {
		return m.startSetupOAuthLogin(getSetupModelProvider(m.setupModelProvider, option))
	}
	if isEmbeddingSetupError(err) {
		return m.showSetupNotice("Embedding setup required", getEmbeddingSetupInstruction(), "enter to continue")
	}
	if isMissingModelCredentialError(err) {
		return m.showSetupProviderAPIKeyPrompt(option)
	}

	return m.showSetupNotice("Model setup unavailable", err.Error(), "enter to continue")
}

func (m *model) submitSetupProviderAPIKey() (tea.Model, tea.Cmd) {
	provider := strings.TrimSpace(m.setupModelProvider)
	modelID := strings.TrimSpace(m.setupPendingModelID)
	apiKey := strings.TrimSpace(m.apiKeyInput.Value())
	if provider == "" {
		return *m, m.setStatus("provider API key unavailable")
	}
	if apiKey == "" {
		return *m, m.setStatus("provider API key required")
	}
	if modelID == "" {
		m.setupProviderAPIKey = apiKey

		return m.showSetupModelSelection()
	}

	option := rpcclient.ModelOption{ID: modelID, Provider: provider}
	if model, ok := modelprovider.DefaultRegistry().GetModel(provider, modelID); ok {
		option.Name = model.Name
		option.API = model.API
		option.ContextWindow = model.ContextWindow
		option.MaxTokens = model.MaxTokens
		option.Reasoning = model.Reasoning
		option.SupportsOAuth = model.SupportsOAuth
	}
	if err := m.persistSetupModelSelection(option, apiKey); err != nil {
		if isEmbeddingSetupError(err) {
			return m.showSetupNotice("Embedding setup required", getEmbeddingSetupInstruction(), "enter to continue")
		}

		return m.showSetupNotice("Provider API key unavailable", err.Error(), "enter to continue")
	}

	return m.completeSetupModelSelection(option)
}

func (m *model) showSetupNotice(title string, message string, hint string) (tea.Model, tea.Cmd) {
	m.cancelSetupOAuthLogin()
	m.setupModelStep = setupModelStepNotice
	m.setupPendingModelID = strings.TrimSpace(title)
	m.setupNoticeMessage = strings.TrimSpace(message)
	m.setupNoticeHint = strings.TrimSpace(hint)
	m.setupOAuthPending = false
	m.setupOAuthProvider = ""
	m.resize()

	return *m, m.setStatus(strings.ToLower(strings.TrimSpace(title)))
}

func (m *model) startSetupOAuthLogin(provider string) (tea.Model, tea.Cmd) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return *m, m.setStatus("provider selection unavailable")
	}
	subscriptionProvider, ok := getSetupSubscriptionProvider(provider)
	if !ok {
		return m.showSetupNotice(
			"Authentication unavailable",
			"Subscription login is not available for "+getProviderDisplayName(provider)+".",
			"enter to continue",
		)
	}

	m.cancelSetupOAuthLogin()
	ctx, cancel := context.WithCancel(m.chatCtx)
	reader, writer := io.Pipe()
	m.setupModelStep = setupModelStepNotice
	m.setupModelProvider = provider
	m.setupPendingModelID = "Connect " + getProviderDisplayName(provider)
	m.setupNoticeMessage = strings.Join([]string{
		"Opening browser to connect " + getProviderDisplayName(provider) + ".",
		"Complete login in your browser, then return here.",
	}, "\n")
	m.setupNoticeHint = "esc to go back"
	m.setupOAuthPending = true
	m.setupOAuthProvider = provider
	m.setupOAuthCancel = cancel
	m.resize()

	return *m, tea.Batch(
		runSetupOAuthLoginCommand(ctx, provider, subscriptionProvider, writer),
		readSetupOAuthOutputCommand(provider, reader),
		m.setStatus("waiting for oauth login"),
	)
}

func runSetupOAuthLoginCommand(
	ctx context.Context,
	provider string,
	subscriptionProvider appcredential.SubscriptionProvider,
	writer *io.PipeWriter,
) tea.Cmd {
	return func() tea.Msg {
		var output bytes.Buffer
		multiWriter := io.MultiWriter(writer, &output)
		credential, err := subscriptionProvider.Login(ctx, appcredential.LoginOptions{
			Provider: provider,
			Output:   multiWriter,
		})
		if err == nil {
			err = newSetupCredentialStore().Set(provider, credential)
		}
		if closeErr := writer.Close(); err == nil && closeErr != nil {
			err = closeErr
		}

		return setupOAuthCompletedMsg{
			provider: provider,
			output:   strings.TrimSpace(output.String()),
			err:      err,
		}
	}
}

func readSetupOAuthOutputCommand(provider string, reader *io.PipeReader) tea.Cmd {
	return func() tea.Msg {
		buffer := make([]byte, 1024)
		n, err := reader.Read(buffer)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return setupOAuthOutputMsg{provider: provider, reader: reader}
			}

			return setupOAuthOutputMsg{provider: provider, reader: reader, err: err}
		}
		if n == 0 {
			return setupOAuthOutputMsg{provider: provider, reader: reader}
		}

		return setupOAuthOutputMsg{
			provider: provider,
			reader:   reader,
			line:     strings.TrimRight(string(buffer[:n]), "\r\n"),
		}
	}
}

func (m model) updateSetupOAuthOutput(msg setupOAuthOutputMsg) (tea.Model, tea.Cmd) {
	if !m.isCurrentSetupOAuthLogin(msg.provider) {
		return m, nil
	}
	if msg.err != nil {
		next, cmd := m.showSetupNotice("Authentication unavailable", msg.err.Error(), "enter to continue")
		return next, cmd
	}
	output := shortenSetupOAuthOutput(msg.line)
	if output == "" {
		return m, nil
	}

	m.setupNoticeMessage = strings.TrimSpace(m.setupNoticeMessage + "\n" + output)
	m.resize()

	return m, readSetupOAuthOutputCommand(msg.provider, msg.reader)
}

func shortenSetupOAuthOutput(output string) string {
	lines := strings.Split(output, "\n")
	shortened := make([]string, 0, len(lines))
	for _, line := range lines {
		line = shortenSetupOAuthOutputLine(line)
		if line != "" {
			shortened = append(shortened, line)
		}
	}

	return strings.Join(shortened, "\n")
}

func shortenSetupOAuthOutputLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	parsed, err := url.Parse(line)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return line
	}

	shortURL := parsed.Scheme + "://" + parsed.Host
	if parsed.EscapedPath() != "" {
		shortURL += parsed.EscapedPath()
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		shortURL += "..."
	}

	return "URL: " + shortURL
}

func (m model) completeSetupOAuthLogin(msg setupOAuthCompletedMsg) (tea.Model, tea.Cmd) {
	if !m.isCurrentSetupOAuthLogin(msg.provider) {
		return m, nil
	}
	if msg.err != nil {
		m.cancelSetupOAuthLogin()
		message := strings.TrimSpace(msg.output)
		if message != "" {
			message += "\n"
		}
		message += msg.err.Error()
		next, cmd := m.showSetupNotice("Authentication failed", message, "enter to retry")
		nextModel := next.(model)
		nextModel.setupOAuthProvider = msg.provider

		return nextModel, cmd
	}

	m.cancelSetupOAuthLogin()
	m.setupOAuthPending = false
	m.setupOAuthProvider = ""
	m.setupNoticeMessage = ""
	m.setupNoticeHint = ""
	m.resize()

	return m.showSetupModelSelection()
}

func loadSetupModelOptionsCommand(
	ctx context.Context,
	provider string,
	selectedModelID string,
	opts clisetup.ModelOptions,
) tea.Cmd {
	return func() tea.Msg {
		baseURL := clisetup.ResolveModelOptionsBaseURL(opts)
		opts.BaseURL = baseURL
		models, _, err := clisetup.ListModelOptions(ctx, opts)

		return setupModelOptionsLoadedMsg{
			provider:        strings.TrimSpace(provider),
			baseURL:         baseURL,
			selectedModelID: strings.TrimSpace(selectedModelID),
			models:          models,
			err:             err,
		}
	}
}

func (m model) completeSetupModelOptionsRefresh(msg setupModelOptionsLoadedMsg) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(msg.provider) != strings.TrimSpace(m.setupModelProvider) {
		return m, nil
	}
	if msg.err != nil {
		return m, m.setStatus("model discovery failed")
	}
	if len(msg.models) == 0 {
		return m, m.setStatus("models unavailable")
	}

	m.setupModels = msg.models
	m.setupModelBaseURL = strings.TrimSpace(msg.baseURL)
	m.setProfileModelSetupModelSelection(msg.selectedModelID)
	m.resize()

	return m, m.setStatus("models refreshed")
}

func (m *model) refreshSetupModelOptions() (tea.Model, tea.Cmd) {
	provider := strings.TrimSpace(m.setupModelProvider)
	if provider == "" {
		return *m, m.setStatus("provider selection unavailable")
	}

	rawConfig := m.loadRawProfileConfig()
	selectedModelID := m.currentSetupModelID()
	opts := clisetup.ModelOptions{
		Provider:  provider,
		Current:   strings.TrimSpace(rawConfig.Models.Main.Name),
		BaseURL:   strings.TrimSpace(m.setupModelBaseURL),
		OAuthOnly: m.setupAuthMethod == setupAuthMethodSubscription,
		Config:    rawConfig,
		Refresh:   true,
	}

	return *m, tea.Batch(
		loadSetupModelOptionsCommand(m.chatCtx, provider, selectedModelID, opts),
		m.setStatus("refreshing models"),
	)
}

func (m *model) cancelSetupOAuthLogin() {
	if m.setupOAuthCancel != nil {
		m.setupOAuthCancel()
		m.setupOAuthCancel = nil
	}
}

func (m model) isCurrentSetupOAuthLogin(provider string) bool {
	return m.setupOAuthPending &&
		m.setupModelStep == setupModelStepNotice &&
		strings.TrimSpace(m.setupOAuthProvider) == strings.TrimSpace(provider)
}

func (m *model) showSetupProviderAPIKeyPrompt(option rpcclient.ModelOption) (tea.Model, tea.Cmd) {
	provider := getSetupModelProvider(m.setupModelProvider, option)
	if provider == "" {
		return *m, m.setStatus("model selection unavailable")
	}

	m.setupModelStep = setupModelStepAPIKey
	m.setupModelProvider = provider
	m.setupPendingModelID = strings.TrimSpace(option.ID)
	m.apiKeyInput = newProviderAPIKeyInput("API key for " + getProviderDisplayName(provider))
	m.prefillSetupProviderAPIKeyInput(provider)
	m.resize()

	return *m, m.setStatus("provider API key required")
}

func (m *model) showSetupProviderAPIKeyPromptForProvider(provider string) (tea.Model, tea.Cmd) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return *m, m.setStatus("provider selection unavailable")
	}

	m.setupModelStep = setupModelStepAPIKey
	m.setupModelProvider = provider
	m.setupPendingModelID = ""
	m.apiKeyInput = newProviderAPIKeyInput("API key for " + getProviderDisplayName(provider))
	m.prefillSetupProviderAPIKeyInput(provider)
	m.resize()

	return *m, m.setStatus("provider API key required")
}

func (m *model) prefillSetupProviderAPIKeyInput(provider string) {
	apiKey := m.loadRawProviderAPIKey(provider)
	if apiKey == "" {
		return
	}

	m.apiKeyInput.SetValue(apiKey)
	m.apiKeyInput.CursorEnd()
}

func (m *model) completeSetupModelSelection(option rpcclient.ModelOption) (tea.Model, tea.Cmd) {
	m.clearProfileModelSetup()
	m.applySetupModelSelectionToRuntime(option)
	m.resize()
	m.setTranscriptContent()

	return *m, m.setStatus("model setup saved")
}

func (m *model) persistSetupModelSelection(option rpcclient.ModelOption, apiKey string) error {
	provider := getSetupModelProvider(m.setupModelProvider, option)
	modelID := strings.TrimSpace(option.ID)
	if provider == "" || modelID == "" {
		return errors.New("model selection unavailable")
	}
	if strings.TrimSpace(m.configPath) == "" {
		return errors.New("config path unavailable")
	}

	api := getSetupModelOptionAPI(provider, option)
	baseURL := getSetupModelOptionBaseURL(provider, m.setupModelBaseURL, option)
	updates := []config.ConfigUpdate{
		{Path: "models.main.provider", Value: provider},
		{Path: "models.main.name", Value: modelID},
		{Path: "models.main.api", Value: api},
		{Path: "models.main.baseURL", Value: baseURL},
		{Path: "models.summary.provider", Value: provider},
		{Path: "models.summary.name", Value: modelID},
		{Path: "models.summary.api", Value: api},
		{Path: "models.summary.baseURL", Value: baseURL},
	}
	updates = append(updates, config.ModelSetupEmbeddingUpdates(provider)...)
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		updates = append(updates, config.ConfigUpdate{
			Path:  fmt.Sprintf("models.providers.%s.apiKey", provider),
			Value: apiKey,
		})
	}
	if _, err := config.SetConfigValuesRelaxed(m.configEnvPath, m.configPath, updates); err != nil {
		return err
	}

	cfg, err := config.Load(m.configEnvPath, m.configPath)
	if err == nil {
		config.Set(cfg)
		m.setupSavedConfig = cfg
	}

	return nil
}

func getSetupModelOptionAPI(provider string, option rpcclient.ModelOption) string {
	if api := strings.TrimSpace(option.API); api != "" {
		return api
	}

	providerDef, ok := modelprovider.DefaultRegistry().GetProvider(strings.TrimSpace(strings.ToLower(provider)))
	if !ok {
		return ""
	}

	return strings.TrimSpace(providerDef.DefaultAPI)
}

func getSetupModelOptionBaseURL(provider string, setupBaseURL string, option rpcclient.ModelOption) string {
	if baseURL := strings.TrimSpace(option.BaseURL); baseURL != "" {
		return baseURL
	}
	if isLocalSetupProvider(provider) {
		return strings.TrimSpace(setupBaseURL)
	}

	return ""
}

func isLocalSetupProvider(provider string) bool {
	providerDef, ok := modelprovider.DefaultRegistry().GetProvider(strings.TrimSpace(strings.ToLower(provider)))

	return ok && providerDef.Local != nil
}

func (m *model) applySetupModelSelectionToRuntime(option rpcclient.ModelOption) {
	m.applySelectedModelToRuntime(option)
	if m.setupSavedConfig == nil {
		return
	}

	info := runtimeInfoFromConfig(m.setupSavedConfig)
	m.runtimeInfo.Provider = info.Provider
	m.runtimeInfo.Model = info.Model
	m.runtimeInfo.SummaryProvider = info.SummaryProvider
	m.runtimeInfo.SummaryModel = info.SummaryModel
	m.runtimeInfo.EmbeddingProvider = info.EmbeddingProvider
	m.runtimeInfo.EmbeddingModel = info.EmbeddingModel
	m.runtimeInfo.Storage = info.Storage
	m.runtimeInfo.Streaming = info.Streaming
	m.modelName = getModelDisplayName(info.Model)
}

func (m model) checkSetupModelAuth(option rpcclient.ModelOption) error {
	provider := getSetupModelProvider(m.setupModelProvider, option)
	modelID := strings.TrimSpace(option.ID)
	if provider == "" || modelID == "" {
		return errors.New("model selection unavailable")
	}

	cfg, err := config.Load(m.configEnvPath, m.configPath)
	if err != nil {
		return err
	}
	cfg.Models.Main.Provider = provider
	cfg.Models.Main.Name = modelID
	cfg.Models.Summary.Provider = provider
	cfg.Models.Summary.Name = modelID
	cfg.Search.Vector.Enabled = false
	if option.API != "" {
		cfg.Models.Main.API = option.API
		cfg.Models.Summary.API = option.API
	}
	_, err = cfg.ResolveModelAuth()

	return err
}

func (m *model) handleProfileModelSetupKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.setupDismissible && isSetupCloseKey(msg) {
		return m.closeProfileSetup()
	}

	switch m.setupModelStep {
	case setupModelStepAuthMethod:
		if msg.Key().Code == tea.KeyEsc {
			return *m, m.startProfileSetup(m.setupDismissible)
		}

		return m.handleProfileModelSetupListKey(msg, len(setupAuthMethodOptions), m.selectCurrentSetupAuthMethodOption)
	case setupModelStepProvider:
		switch msg.Key().Code {
		case tea.KeyEsc, tea.KeyLeft, tea.KeyBackspace:
			return m.showSetupAuthMethodSelection()
		}

		return m.handleProfileModelSetupListKey(msg, len(m.setupProviders), m.selectCurrentSetupProviderOption)
	case setupModelStepModel:
		if isSetupModelRefreshKey(msg) {
			return m.refreshSetupModelOptions()
		}
		if isModelFilterKey(msg) {
			var cmd tea.Cmd
			m.modelFilterInput, cmd = m.modelFilterInput.Update(msg)
			m.setupItemSelected = 0
			m.setupOffset = 0
			m.resize()
			return *m, cmd
		}

		switch msg.Key().Code {
		case tea.KeyEsc, tea.KeyLeft:
			return m.showSetupProviderSelection()
		}

		return m.handleProfileModelSetupListKey(msg, len(m.filteredSetupModels()), m.selectCurrentSetupModelOption)
	case setupModelStepAPIKey:
		switch msg.Key().Code {
		case tea.KeyEsc:
			if strings.TrimSpace(m.setupPendingModelID) == "" {
				return m.showSetupProviderSelection()
			}

			return m.showSetupModelSelection()
		case tea.KeyEnter:
			return m.submitSetupProviderAPIKey()
		}

		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		m.resize()
		return *m, cmd
	case setupModelStepNotice:
		switch msg.Key().Code {
		case tea.KeyEnter:
			if m.setupOAuthPending {
				return *m, nil
			}
			if strings.TrimSpace(m.setupOAuthProvider) != "" {
				return m.startSetupOAuthLogin(m.setupOAuthProvider)
			}

			return m.showSetupModelSelection()
		case tea.KeyEsc, tea.KeyLeft, tea.KeyBackspace:
			return m.showSetupProviderSelection()
		}

		return *m, nil
	default:
		return *m, nil
	}
}

func (m *model) showSetupAuthMethodSelection() (tea.Model, tea.Cmd) {
	m.cancelSetupOAuthLogin()
	authMethod := strings.TrimSpace(m.setupAuthMethod)
	m.setupModelStep = setupModelStepAuthMethod
	m.setupAuthMethod = ""
	m.setupProviders = nil
	m.setupModels = nil
	m.setupModelProvider = ""
	m.setupModelBaseURL = ""
	m.setupProviderAPIKey = ""
	m.setupPendingModelID = ""
	m.setupNoticeMessage = ""
	m.setupNoticeHint = ""
	m.setupOAuthPending = false
	m.setupOAuthProvider = ""
	for index, option := range setupAuthMethodOptions {
		if option.ID == authMethod {
			m.setProfileModelSetupSelection(index, len(setupAuthMethodOptions))
			break
		}
	}
	m.resize()

	return *m, m.setStatus("choose an auth method")
}

func (m *model) showSetupProviderSelection() (tea.Model, tea.Cmd) {
	m.cancelSetupOAuthLogin()
	provider := strings.TrimSpace(m.setupModelProvider)
	m.setupModelStep = setupModelStepProvider
	m.setupModels = nil
	m.setupModelProvider = ""
	m.setupModelBaseURL = ""
	m.setupProviderAPIKey = ""
	m.setupPendingModelID = ""
	m.setupNoticeMessage = ""
	m.setupNoticeHint = ""
	m.setupOAuthPending = false
	m.setupOAuthProvider = ""
	for index, option := range m.setupProviders {
		if strings.TrimSpace(option.ID) == provider {
			m.setProfileModelSetupSelection(index, len(m.setupProviders))
			break
		}
	}
	m.resize()

	return *m, m.setStatus("choose a model provider")
}

func (m *model) showSetupModelSelection() (tea.Model, tea.Cmd) {
	m.setupModelStep = setupModelStepModel
	m.setupNoticeMessage = ""
	m.setupNoticeHint = ""
	m.setupPendingModelID = ""
	m.resize()

	return *m, m.setStatus("choose a model")
}

func (m *model) handleProfileModelSetupListKey(
	msg tea.KeyPressMsg,
	count int,
	submit func() (tea.Model, tea.Cmd),
) (tea.Model, tea.Cmd) {
	if count == 0 {
		return *m, nil
	}

	selection := m.setupItemSelected
	switch msg.Key().Code {
	case tea.KeyUp:
		selection--
	case tea.KeyDown:
		selection++
	case tea.KeyHome:
		selection = 0
	case tea.KeyEnd:
		selection = count - 1
	case tea.KeyPgUp:
		selection -= m.getProfileModelSetupListHeight()
	case tea.KeyPgDown:
		selection += m.getProfileModelSetupListHeight()
	case tea.KeyEnter:
		return submit()
	default:
		return *m, nil
	}

	m.setProfileModelSetupSelection(selection, count)
	return *m, nil
}

func isSetupModelRefreshKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return msg.Keystroke() == "ctrl+r" || key.Code == 'r' && key.Mod == tea.ModCtrl
}

func (m model) currentSetupModelID() string {
	models := m.filteredSetupModels()
	if len(models) == 0 {
		return ""
	}

	index := min(max(m.setupItemSelected, 0), len(models)-1)
	return strings.TrimSpace(models[index].ID)
}

func (m *model) setProfileModelSetupModelSelection(modelID string) {
	models := m.filteredSetupModels()
	if len(models) == 0 {
		m.setProfileModelSetupSelection(0, 0)
		return
	}

	modelID = strings.TrimSpace(modelID)
	if modelID != "" {
		for index, option := range models {
			if strings.TrimSpace(option.ID) == modelID {
				m.setProfileModelSetupSelection(index, len(models))
				return
			}
		}
	}

	m.setProfileModelSetupSelection(0, len(models))
}

func (m *model) handleProfileModelSetupPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if m.setupModelStep == setupModelStepModel {
		var cmd tea.Cmd
		m.modelFilterInput, cmd = m.modelFilterInput.Update(msg)
		m.setupItemSelected = 0
		m.setupOffset = 0
		m.resize()
		return *m, cmd
	}
	if m.setupModelStep != setupModelStepAPIKey {
		return *m, nil
	}

	msg.Content = normalizeProviderAPIKeyPaste(msg.Content)
	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	m.resize()

	return *m, cmd
}

func (m *model) handleProfileModelSetupWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	count := 0
	switch m.setupModelStep {
	case setupModelStepAuthMethod:
		count = len(setupAuthMethodOptions)
	case setupModelStepProvider:
		count = len(m.setupProviders)
	case setupModelStepModel:
		count = len(m.filteredSetupModels())
	default:
		return *m, nil
	}

	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.setProfileModelSetupSelection(m.setupItemSelected-1, count)
	case tea.MouseWheelDown:
		m.setProfileModelSetupSelection(m.setupItemSelected+1, count)
	}

	return *m, nil
}

func (m *model) handleProfileModelSetupClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return *m, nil
	}

	count := 0
	submit := func() (tea.Model, tea.Cmd) {
		return *m, nil
	}
	switch m.setupModelStep {
	case setupModelStepAuthMethod:
		count = len(setupAuthMethodOptions)
		submit = m.selectCurrentSetupAuthMethodOption
	case setupModelStepProvider:
		count = len(m.setupProviders)
		submit = m.selectCurrentSetupProviderOption
	case setupModelStepModel:
		count = len(m.filteredSetupModels())
		submit = m.selectCurrentSetupModelOption
	default:
		return *m, nil
	}
	if count == 0 {
		return *m, nil
	}

	row := msg.Y - m.getProfileModelSetupListFirstRow()
	if row < 0 || row >= m.getProfileModelSetupRenderedListHeight() {
		return *m, nil
	}
	if m.getProfileModelSetupRowHeight() > 1 {
		row /= 2
	}

	selection := m.setupOffset + row
	if selection < 0 || selection >= count {
		return *m, nil
	}

	m.setProfileModelSetupSelection(selection, count)
	return submit()
}

func (m *model) setProfileModelSetupSelection(selection int, count int) {
	if count <= 0 {
		m.setupItemSelected = 0
		m.setupOffset = 0
		return
	}

	height := m.getProfileModelSetupListHeight()
	m.setupItemSelected = min(max(selection, 0), count-1)
	m.setupOffset = getChatsCommandViewOffsetForSelection(m.setupItemSelected, m.setupOffset, height, count)
}

func (m model) renderProfileModelSetup() string {
	switch m.setupModelStep {
	case setupModelStepAuthMethod:
		hint := "enter to select · esc to go back"

		return m.renderProfileModelSetupList(
			"Select login method",
			hint,
			m.renderProfileModelSetupAuthMethodRows(),
		)
	case setupModelStepProvider:
		return m.renderProfileModelSetupList(
			"Select model provider",
			"enter to select · esc to go back",
			m.renderProfileModelSetupProviderRows(),
		)
	case setupModelStepModel:
		return m.renderProfileModelSetupModelList()
	case setupModelStepAPIKey:
		return m.renderProfileModelSetupAPIKey()
	case setupModelStepNotice:
		return m.renderProfileModelSetupNotice()
	default:
		return ""
	}
}

func (m model) renderProfileModelSetupAuthMethodRows() []string {
	height := m.getProfileModelSetupListHeight()
	end := min(m.setupOffset+height, len(setupAuthMethodOptions))
	rows := make([]string, 0, max((end-m.setupOffset)*2, 1))
	for index := m.setupOffset; index < end; index++ {
		option := setupAuthMethodOptions[index]
		row := renderProfileModelSetupOptionRow(
			option.Label,
			option.Description,
			m.getProfileModelSetupListWidth(),
			index == m.setupItemSelected,
		)
		rows = append(rows, strings.Split(row, "\n")...)
	}
	for len(rows) < height*m.getProfileModelSetupRowHeight() {
		rows = append(rows, "")
	}

	return rows
}

func (m model) renderProfileModelSetupList(title string, hintText string, rows []string) string {
	boxWidth := m.getProfileModelSetupBoxWidth()

	list := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Width(boxWidth).
		Render(strings.Join(rows, "\n"))

	return m.renderProfileModelSetupFrame(title, hintText, list)
}

func (m model) renderProfileModelSetupModelList() string {
	boxWidth := m.getProfileModelSetupBoxWidth()

	list := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Width(boxWidth).
		Render(strings.Join(m.renderProfileModelSetupModelRows(), "\n"))

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Width(boxWidth).
		Render(m.renderProfileModelSetupHint("enter to select · ctrl+r to refresh · esc to go back", boxWidth))

	return m.renderProfileModelSetupFrameWithTitleContent(
		m.renderProfileModelSetupModelTitle(boxWidth),
		hint,
		list,
	)
}

func (m model) renderProfileModelSetupModelTitle(width int) string {
	width = max(width, 1)
	title := "Select model from " + getProviderDisplayName(m.setupModelProvider)
	inputWidth := min(setupModelFilterWidth, max(width/2, 10))
	titleText := m.renderProfileModelSetupTitleLeft(title, max(width-inputWidth-1, 1))
	filter := m.renderInlineModelFilterInput(inputWidth)
	gap := max(width-lipgloss.Width(titleText)-lipgloss.Width(filter), 1)

	return titleText + strings.Repeat(" ", gap) + filter
}

func (m model) renderProfileModelSetupTitle(width int, title string) string {
	width = max(width, 1)
	right := m.getProfileModelSetupLoginMethodLabel()
	if right == "" {
		return m.renderProfileModelSetupTitleLeft(title, width)
	}

	rightText := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Render(renderProfileModelSetupPaddedLabel(right, min(lipgloss.Width(right)+2, width)))
	titleText := m.renderProfileModelSetupTitleLeft(title, max(width-lipgloss.Width(rightText)-1, 1))
	gap := max(width-lipgloss.Width(titleText)-lipgloss.Width(rightText), 1)

	return titleText + strings.Repeat(" ", gap) + rightText
}

func (m model) renderProfileModelSetupTitleLeft(title string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Bold(true).
		Render(renderProfileModelSetupPaddedLabel(title, width))
}

func (m model) getProfileModelSetupLoginMethodLabel() string {
	switch strings.TrimSpace(m.setupAuthMethod) {
	case setupAuthMethodSubscription:
		return "login type: subscription"
	case setupAuthMethodAPIKey:
		return "login type: api key"
	default:
		return ""
	}
}

func (m model) renderInlineModelFilterInput(width int) string {
	width = max(width, 1)
	input := m.modelFilterInput
	input.SetWidth(width)
	content := truncateCommandMenuText(input.View(), width)
	content += strings.Repeat(" ", max(width-lipgloss.Width(content), 0))

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Width(width).
		Render(content)
}

func (m model) renderProfileModelSetupFrame(title string, hintText string, body string) string {
	boxWidth := m.getProfileModelSetupBoxWidth()
	hint := m.renderProfileModelSetupHint(hintText, boxWidth)

	return m.renderProfileModelSetupFrameWithHint(title, hint, body)
}

func (m model) renderProfileModelSetupFrameWithHint(title string, hint string, body string) string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	height := max(m.transcript.Height(), 1)
	boxWidth := m.getProfileModelSetupBoxWidth()

	titleText := m.renderProfileModelSetupTitle(boxWidth, strings.TrimSpace(title))
	mark := lipgloss.NewStyle().
		Width(boxWidth).
		Align(lipgloss.Center).
		Render(renderMorphBanner(morphHeaderMark))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		mark,
		"",
		titleText,
		body,
		hint,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderProfileModelSetupFrameWithTitleContent(titleContent string, hint string, body string) string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	height := max(m.transcript.Height(), 1)
	boxWidth := m.getProfileModelSetupBoxWidth()
	mark := lipgloss.NewStyle().
		Width(boxWidth + 2).
		Align(lipgloss.Center).
		Render(renderMorphBanner(morphHeaderMark))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		mark,
		"",
		titleContent,
		body,
		hint,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderProfileModelSetupProviderRows() []string {
	height := m.getProfileModelSetupListHeight()
	end := min(m.setupOffset+height, len(m.setupProviders))
	rows := make([]string, 0, max((end-m.setupOffset)*m.getProfileModelSetupRowHeight(), 1))
	for index := m.setupOffset; index < end; index++ {
		row := renderProfileModelSetupProviderRow(
			m.setupProviders[index],
			m.setupAuthMethod,
			m.getProfileModelSetupListWidth(),
			index == m.setupItemSelected,
		)
		rows = append(rows, strings.Split(row, "\n")...)
	}
	for len(rows) < height*m.getProfileModelSetupRowHeight() {
		rows = append(rows, "")
	}

	return rows
}

func renderProfileModelSetupProviderRow(provider rpcclient.ProviderOption, authMethod string, width int, selected bool) string {
	return renderProfileModelSetupOptionRow(
		getProviderOptionDisplayName(provider),
		getProfileModelSetupProviderDescription(provider, authMethod),
		width,
		selected,
	)
}

func renderProfileModelSetupOptionRow(label string, description string, width int, selected bool) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	name := truncateCommandMenuText(label, contentWidth)
	description = truncateCommandMenuText(description, contentWidth)
	if width <= 1 {
		return truncateChatsCommandRow(name, width) + "\n" + truncateChatsCommandRow(description, width)
	}

	nameForeground := ""
	if selected {
		nameForeground = defaultTUITheme.MarkdownLinkForeground
	}
	nameLine := renderProfileModelSetupProviderLine(name, width, nameForeground, selected)
	descriptionLine := renderProfileModelSetupProviderLine(description, width, defaultTUITheme.MutedText, selected)

	return nameLine + "\n" + descriptionLine
}

func renderProfileModelSetupProviderLine(text string, width int, foreground string, selected bool) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	line := " " + truncateChatsCommandRow(text, contentWidth) + " "
	line += strings.Repeat(" ", max(width-lipgloss.Width(line), 0))
	style := lipgloss.NewStyle().
		Width(width)
	if strings.TrimSpace(foreground) != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if selected {
		style = style.Background(lipgloss.Color(defaultTUITheme.JumpToBottomBackground))
	}

	return style.Render(line)
}

func getProfileModelSetupProviderDescription(provider rpcclient.ProviderOption, authMethod string) string {
	authMethod = strings.TrimSpace(authMethod)
	switch strings.TrimSpace(strings.ToLower(provider.ID)) {
	case constants.ModelProviderAnthropic:
		if authMethod == setupAuthMethodAPIKey {
			return "Use your Anthropic API key"
		}

		return "Use your Anthropic subscription"
	case constants.ModelProviderOpenAICodex:
		return "Use your OpenAI account"
	case constants.ModelProviderGitHubCopilot:
		return "Use your GitHub Copilot subscription"
	case constants.ModelProviderOpenAI:
		return "Use your OpenAI API key"
	case constants.ModelProviderOpenRouter:
		return "Use your OpenRouter API key"
	default:
		name := getProviderOptionDisplayName(provider)
		if authMethod == setupAuthMethodSubscription || provider.SupportsOAuth && !provider.SupportsAPIKey {
			return "Use your " + name + " account"
		}
		if authMethod == setupAuthMethodAPIKey || provider.SupportsAPIKey {
			return "Use your " + name + " API key"
		}
		if detail := getProviderOptionDetail(provider); detail != "" {
			return detail
		}

		return "Use " + name
	}
}

func (m model) renderProfileModelSetupModelRows() []string {
	height := m.getProfileModelSetupListHeight()
	models := m.filteredSetupModels()
	end := min(m.setupOffset+height, len(models))
	rows := make([]string, 0, max(end-m.setupOffset, 1))
	if len(models) == 0 {
		rows = append(rows, renderNoMatchingModelsRow(m.getProfileModelSetupListWidth()))
	} else {
		for index := m.setupOffset; index < end; index++ {
			row := renderModelsCommandRow(
				models[index],
				m.getProfileModelSetupListWidth(),
				index == m.setupItemSelected,
			)
			rows = append(rows, row)
		}
	}
	for len(rows) < height {
		rows = append(rows, "")
	}

	return rows
}

func (m model) filteredSetupModels() []rpcclient.ModelOption {
	return filterModelOptions(m.setupModels, m.modelFilterInput.Value())
}

func (m model) renderProfileModelSetupAPIKey() string {
	boxWidth := m.getProfileModelSetupBoxWidth()
	input := m.apiKeyInput
	input.Placeholder = "Enter key"
	input.SetWidth(max(boxWidth-4, 1))

	body := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Padding(0, 1).
		Width(boxWidth).
		Render(input.View())
	hint := renderProfileModelSetupAPIKeyHint(m.setupDismissible, lipgloss.Width(body))

	return m.renderProfileModelSetupFrameWithHint(
		"Enter API key for "+getProviderDisplayName(m.setupModelProvider),
		hint,
		body,
	)
}

func renderProfileModelSetupSplitHint(left string, right string, width int) string {
	width = max(width, 1)
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	spacer := max(width-2-leftWidth-rightWidth, 1)
	row := " " + left + strings.Repeat(" ", spacer) + right + " "

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Width(width).
		Render(row)
}

func (m model) renderProfileModelSetupHint(hintText string, width int) string {
	width = max(width, 1)
	hintText = strings.TrimSpace(hintText)
	if m.setupDismissible {
		return renderProfileModelSetupSplitHint(hintText, setupCloseHint, width)
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Width(width).
		Render(renderProfileModelSetupPaddedLabel(hintText, width))
}

func renderProfileModelSetupAPIKeyHint(dismissible bool, width int) string {
	if dismissible {
		return renderProfileModelSetupSplitHint("enter to save · esc to go back", setupCloseHint, width)
	}

	return renderProfileModelSetupSplitHint("enter to save", "esc to go back", width)
}

func renderProfileModelSetupPaddedLabel(label string, width int) string {
	width = max(width, 1)
	label = strings.TrimSpace(label)
	if width <= 2 {
		return truncateCommandMenuText(label, width)
	}

	return " " + truncateCommandMenuText(label, width-2) + " "
}

func (m model) renderProfileModelSetupNotice() string {
	boxWidth := m.getProfileModelSetupBoxWidth()
	hint := strings.TrimSpace(m.setupNoticeHint)
	if !strings.Contains(strings.ToLower(hint), "esc") {
		hint = strings.TrimSpace(hint + " · esc to go back")
	}

	message := lipgloss.NewStyle().
		Width(max(boxWidth-2, 1)).
		Render(renderProfileModelSetupNoticeMessage(m.setupNoticeMessage, max(boxWidth-2, 1)))
	body := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(defaultTUITheme.InputFrameBorder)).
		Padding(0, 1).
		Width(boxWidth).
		Render(message)

	return m.renderProfileModelSetupFrame(
		strings.TrimSpace(m.setupPendingModelID),
		hint,
		body,
	)
}

func renderProfileModelSetupNoticeMessage(message string, width int) string {
	width = max(width, 1)
	wrapped := strings.TrimSpace(wordwrap.String(strings.TrimSpace(message), width))
	if wrapped == "" {
		return ""
	}

	lines := strings.Split(wrapped, "\n")
	for index, line := range lines {
		lines[index] = renderProfileModelSetupNoticeMessageLine(line)
	}

	return strings.Join(lines, "\n")
}

func renderProfileModelSetupNoticeMessageLine(line string) string {
	command := getProfileModelSetupNoticeAuthCommand(line)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.MutedText))
	if command == "" {
		return mutedStyle.Render(line)
	}

	commandIndex := strings.Index(line, command)
	commandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.MarkdownLinkForeground))

	return strings.Join([]string{
		mutedStyle.Render(line[:commandIndex]),
		commandStyle.Render(command),
		mutedStyle.Render(line[commandIndex+len(command):]),
	}, "")
}

func getProfileModelSetupNoticeAuthCommand(line string) string {
	fields := strings.Fields(line)
	for index := 0; index+3 < len(fields); index++ {
		if fields[index] == "morph" && fields[index+1] == "auth" && fields[index+2] == "login" {
			return strings.Join(fields[index:index+4], " ")
		}
	}

	return ""
}

func (m model) getProfileModelSetupListHeight() int {
	count := 0
	switch m.setupModelStep {
	case setupModelStepAuthMethod:
		count = len(setupAuthMethodOptions)
	case setupModelStepProvider:
		count = len(m.setupProviders)
	case setupModelStepModel:
		count = len(m.setupModels)
	}
	return min(max(count, 1), setupModelMaxListHeight)
}

func (m model) getProfileModelSetupRenderedListHeight() int {
	return m.getProfileModelSetupListHeight() * m.getProfileModelSetupRowHeight()
}

func (m model) getProfileModelSetupRowHeight() int {
	switch m.setupModelStep {
	case setupModelStepAuthMethod, setupModelStepProvider:
		return 2
	default:
		return 1
	}
}

func (m model) getProfileModelSetupListWidth() int {
	return max(m.getProfileModelSetupBoxWidth()-2, 1)
}

func (m model) getProfileModelSetupBoxWidth() int {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	minWidth := setupModelMinWidth
	if m.setupDismissible || m.getProfileModelSetupLoginMethodLabel() != "" {
		minWidth = max(minWidth, setupModelLoginMinWidth)
	}

	return min(max(width/2, minWidth), min(setupModelMaxWidth, width))
}

func (m model) getProfileModelSetupListFirstRow() int {
	height := max(m.transcript.Height(), 1)
	listHeight := m.getProfileModelSetupRenderedListHeight()
	markHeight := lipgloss.Height(renderMorphBanner(morphHeaderMark))
	contentHeight := markHeight + listHeight + 6
	top := max((height-contentHeight)/2, 0)

	return m.getTranscriptTop() + top + markHeight + 4
}

func (m *model) clearProfileModelSetup() {
	m.cancelSetupOAuthLogin()
	m.setupModelStep = ""
	m.setupAuthMethod = ""
	m.setupProviders = nil
	m.setupModels = nil
	m.setupModelProvider = ""
	m.setupModelBaseURL = ""
	m.setupProviderAPIKey = ""
	m.setupPendingModelID = ""
	m.setupNoticeMessage = ""
	m.setupNoticeHint = ""
	m.setupItemSelected = 0
	m.setupOffset = 0
	m.modelFilterInput = newModelFilterInput()
	m.setupDismissible = false
	m.setupOAuthPending = false
	m.setupOAuthProvider = ""
	m.setupOAuthCancel = nil
}

func (m model) closeProfileSetup() (tea.Model, tea.Cmd) {
	m.namePromptEnabled = false
	m.setupNamePromptActive = false
	m.setupDismissible = false
	m.namePromptError = ""
	m.namePromptErrorStartedAt = time.Time{}
	m.nameInput.SetValue("")
	m.clearProfileModelSetup()
	m.resize()
	m.setTranscriptContent()

	return m, m.setStatus("setup closed")
}

func isSetupCloseKey(msg tea.KeyPressMsg) bool {
	return msg.Keystroke() == "ctrl+x"
}

func getSetupModelProvider(provider string, option rpcclient.ModelOption) string {
	if provider = strings.TrimSpace(provider); provider != "" {
		return provider
	}

	return strings.TrimSpace(option.Provider)
}

func isMissingModelCredentialError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "API key is required")
}

func isEmbeddingSetupError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "embedding model") ||
		strings.Contains(message, "embedding API key")
}

func getEmbeddingSetupInstruction() string {
	return "embedding setup required; configure models.embedding or run morph config set search.vector.enabled false"
}

func (m model) profileModelSetupMissing() bool {
	raw := m.loadRawProfileConfig()
	return strings.TrimSpace(raw.Models.Main.Provider) == "" ||
		strings.TrimSpace(raw.Models.Main.Name) == ""
}

func (m model) shouldShowProfileModelSetup() bool {
	return strings.TrimSpace(m.setupModelStep) != ""
}

func (m model) loadRawProfileMainProvider() string {
	return strings.TrimSpace(m.loadRawProfileConfig().Models.Main.Provider)
}

func (m model) loadRawProfileMainModel() string {
	return strings.TrimSpace(m.loadRawProfileConfig().Models.Main.Name)
}

func (m model) loadRawProviderAPIKey(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}

	raw := m.loadRawProfileConfig()
	if raw.Models.Providers == nil {
		return ""
	}

	return strings.TrimSpace(raw.Models.Providers[provider].APIKey)
}

func (m model) loadRawProfileConfig() *config.Config {
	cfg := config.NewProfileConfig()
	if strings.TrimSpace(m.configPath) == "" {
		return cfg
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return cfg
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return config.NewProfileConfig()
	}

	return cfg
}

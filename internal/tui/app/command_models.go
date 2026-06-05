package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wandxy/hand/internal/config"
	modelcatalog "github.com/wandxy/hand/internal/model"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type modelSelector interface {
	SelectModel(context.Context, string, ...rpcclient.ModelSelectOptions) (rpcclient.ModelOption, error)
}

type providerAPIKeySetter interface {
	SetProviderAPIKey(context.Context, string, string) error
}

const (
	modelOptionReasoningWidth = len("reasoning")
	modelOptionContextWidth   = 5
)

type providersLoadedMsg struct {
	List rpcclient.ProviderList
	Err  error
}

type modelsLoadedMsg struct {
	List rpcclient.ModelList
	Err  error
}

type modelSelectedMsg struct {
	Model rpcclient.ModelOption
	Err   error
}

type providerAPIKeySetMsg struct {
	Provider string
	ModelID  string
	Err      error
}

func newProviderAPIKeyInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = strings.TrimSpace(placeholder)
	if input.Placeholder == "" {
		input.Placeholder = "API key"
	}
	input.CharLimit = 4096
	input.SetWidth(80)
	input.Focus()

	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.
		Foreground(lipgloss.Color("15")).
		UnsetBackground()
	styles.Focused.Placeholder = styles.Focused.Placeholder.
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		UnsetBackground()
	styles.Focused.Prompt = styles.Focused.Prompt.
		UnsetBackground()
	input.SetStyles(styles)

	return input
}

func newModelFilterInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "Filter models"
	input.CharLimit = 120
	input.SetWidth(24)
	input.Focus()

	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.
		Foreground(lipgloss.Color(defaultTUITheme.MarkdownLinkForeground)).
		UnsetBackground()
	styles.Focused.Placeholder = styles.Focused.Placeholder.
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		UnsetBackground()
	styles.Focused.Prompt = styles.Focused.Prompt.UnsetBackground()
	input.SetStyles(styles)

	return input
}

func (m *model) startProvidersCommand() tea.Cmd {
	return tea.Batch(
		m.setStatus("loading providers"),
		loadProvidersCmd(m.loadRawProfileMainProvider()),
	)
}

func loadProvidersCmd(currentProvider string) tea.Cmd {
	return func() tea.Msg {
		providers := modelcatalog.ListProviders(modelcatalog.ProviderQuery{
			Current: currentProvider,
		})
		return providersLoadedMsg{List: rpcclient.ProviderList{Providers: providers}}
	}
}

func (m *model) completeProvidersCommand(msg providersLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("providers unavailable")
	}

	m.showCommandView(commandViewPayload{
		TitleLeft:       "Providers",
		TitleRight:      getProvidersCommandTitleRight(),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindProviders,
		Providers:       msg.List.Providers,
	})

	return nil
}

func (m *model) startModelsCommand() tea.Cmd {
	provider := m.loadRawProfileMainProvider()

	return tea.Batch(
		m.setStatus("loading models"),
		loadModelsCmd(provider, m.loadRawProfileMainModel()),
	)
}

func loadModelsCmd(provider string, currentModel string) tea.Cmd {
	return func() tea.Msg {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			return modelsLoadedMsg{Err: errors.New("model provider is required")}
		}

		models := modelcatalog.ListOptions(modelcatalog.OptionQuery{
			Provider: provider,
			Current:  currentModel,
		})
		return modelsLoadedMsg{List: rpcclient.ModelList{
			Provider: provider,
			Models:   models,
		}}
	}
}

func (m *model) completeModelsCommand(msg modelsLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("models unavailable")
	}

	m.showCommandView(commandViewPayload{
		TitleLeft:       "Models",
		TitleSubtext:    getProviderDisplayName(msg.List.Provider),
		TitleRight:      getModelsCommandTitleRight(),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindModels,
		Models:          msg.List.Models,
		ModelProvider:   msg.List.Provider,
		ModelAuthType:   msg.List.AuthType,
	})
	m.modelFilterInput = newModelFilterInput()

	return nil
}

func (m model) isModelsCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindModels
}

func (m model) isProvidersCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindProviders
}

func (m model) isProviderAPIKeyCommandView() bool {
	return m.commandView.Visible && m.commandView.Kind == commandViewKindProviderAPIKey
}

func (m model) renderProvidersCommandViewContent(content commandViewContent) string {
	providers := m.commandView.Providers
	if len(providers) == 0 {
		return "No providers available."
	}

	offset := min(max(content.Offset, 0), max(len(providers)-1, 0))
	height := max(content.Height, 1)
	end := min(offset+height, len(providers))
	rows := make([]string, 0, end-offset)
	for index := offset; index < end; index++ {
		row := renderProvidersCommandRow(
			providers[index],
			content.Width,
			index == m.commandViewItemSelected,
		)
		rows = append(rows, row)
	}

	for len(rows) <= height+1 {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func renderProvidersCommandRow(provider rpcclient.ProviderOption, width int, selected bool) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	name := getProviderOptionDisplayName(provider)
	detail := getProviderOptionDetail(provider)

	return renderCommandListEntryRow(name, detail, width, contentWidth, selected)
}

func getProviderOptionDisplayName(provider rpcclient.ProviderOption) string {
	if strings.TrimSpace(provider.Name) != "" {
		return strings.TrimSpace(provider.Name)
	}

	return getProviderDisplayName(provider.ID)
}

func getProviderDisplayName(providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ""
	}

	if provider, ok := modelprovider.DefaultRegistry().GetProvider(providerID); ok {
		if name := strings.TrimSpace(provider.DisplayName); name != "" {
			return name
		}
	}

	return providerID
}

func getProviderOptionDetail(provider rpcclient.ProviderOption) string {
	parts := make([]string, 0, 3)
	if provider.Current {
		parts = append(parts, "current")
	}
	if authType := strings.TrimSpace(provider.AuthType); authType != "" && authType != "none" {
		parts = append(parts, authType)
	}
	if provider.ModelCount > 0 {
		parts = append(parts, fmt.Sprintf("%d models", provider.ModelCount))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(provider.Type)
	}

	return strings.Join(parts, " · ")
}

func (m model) renderModelsCommandViewContent(content commandViewContent) string {
	if len(m.commandView.Models) == 0 {
		return "No models available."
	}

	filterBlock := m.renderModelFilterBlock(content.Width)
	filterHeight := lipgloss.Height(filterBlock)
	height := max(content.Height-filterHeight, 1)
	models := m.filteredCommandModels()
	offset := min(max(content.Offset, 0), max(len(models)-1, 0))
	end := min(offset+height, len(models))
	rows := make([]string, 0, height+filterHeight)
	rows = append(rows, strings.Split(filterBlock, "\n")...)
	if len(models) == 0 {
		rows = append(rows, renderNoMatchingModelsRow(content.Width))
	} else {
		for index := offset; index < end; index++ {
			row := renderModelsCommandRow(models[index], content.Width, index == m.commandViewItemSelected)
			rows = append(rows, row)
		}
	}

	for len(rows) < height+filterHeight {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func renderNoMatchingModelsRow(width int) string {
	width = max(width, 1)
	if width == 1 {
		return " "
	}

	return " " + truncateCommandMenuText("No matching models.", width-1)
}

func (m model) renderModelFilterBlock(width int) string {
	return strings.Join([]string{"", m.renderModelFilterRow(width), ""}, "\n")
}

func (m model) renderModelFilterRow(width int) string {
	width = max(width, 1)
	input := m.modelFilterInput
	input.SetWidth(max(width-1, 1))

	return lipgloss.NewStyle().
		Width(width).
		PaddingLeft(1).
		Render(input.View())
}

func (m model) filteredCommandModels() []rpcclient.ModelOption {
	return filterModelOptions(m.commandView.Models, m.modelFilterInput.Value())
}

func filterModelOptions(models []rpcclient.ModelOption, query string) []rpcclient.ModelOption {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return models
	}

	filtered := make([]rpcclient.ModelOption, 0, len(models))
	for _, model := range models {
		haystack := strings.ToLower(strings.Join([]string{
			model.ID,
			model.Name,
		}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, model)
		}
	}

	return filtered
}

func renderModelsCommandRow(model rpcclient.ModelOption, width int, selected bool) string {
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	name := getModelOptionDisplayName(model)
	detail := getModelOptionMutedDetail(model)
	return renderCommandListEntryRow(name, detail, width, contentWidth, selected)
}

func renderCommandListEntryRow(label string, detail string, width int, contentWidth int, selected bool) string {
	detailWidth := lipgloss.Width(detail)
	labelWidth := max(contentWidth-detailWidth-2, 1)
	label = truncateCommandMenuText(label, labelWidth)
	gap := 0
	if detail != "" {
		gap = max(contentWidth-lipgloss.Width(label)-detailWidth, 1)
	}
	trailing := max(contentWidth-lipgloss.Width(label)-gap-detailWidth, 0)
	row := label + strings.Repeat(" ", gap) + detail
	if selected {
		background := lipgloss.NewStyle().
			Background(lipgloss.Color(defaultTUITheme.JumpToBottomBackground))
		label = background.
			Foreground(lipgloss.Color(defaultTUITheme.MarkdownLinkForeground)).
			Render(label)
		if detail != "" {
			detail = background.
				Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
				Render(detail)
		}
		row = label +
			background.Render(strings.Repeat(" ", gap)) +
			detail +
			background.Render(strings.Repeat(" ", trailing))
	} else if detail != "" {
		detail = lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
			Render(detail)
		row = label + strings.Repeat(" ", gap) + detail
	}
	if width <= 1 {
		return truncateChatsCommandRow(row, width)
	}

	if selected {
		background := lipgloss.NewStyle().
			Background(lipgloss.Color(defaultTUITheme.JumpToBottomBackground))
		return background.Render(" ") +
			truncateChatsCommandRow(row, contentWidth) +
			background.Render(" ")
	}

	return " " + truncateChatsCommandRow(row, contentWidth) + " "
}

func getModelOptionDisplayName(model rpcclient.ModelOption) string {
	if strings.TrimSpace(model.Name) != "" {
		return strings.TrimSpace(model.Name)
	}

	return strings.TrimSpace(model.ID)
}

func getModelOptionContextLength(model rpcclient.ModelOption) string {
	if model.ContextWindow > 0 {
		return fmt.Sprintf("%dk", model.ContextWindow/1000)
	}

	return ""
}

func getModelOptionMutedDetail(model rpcclient.ModelOption) string {
	contextLength := getModelOptionContextLength(model)
	if !model.Reasoning && contextLength == "" {
		return ""
	}

	if model.Reasoning {
		reasoning := padCommandCell("reasoning", modelOptionReasoningWidth, false)
		if contextLength != "" {
			contextCell := padCommandCell(contextLength, modelOptionContextWidth, true)
			return reasoning + " · " + contextCell
		}

		return reasoning
	}

	return strings.Repeat(" ", modelOptionReasoningWidth+3) +
		padCommandCell(contextLength, modelOptionContextWidth, true)
}

func padCommandCell(value string, width int, alignRight bool) string {
	value = truncateCommandMenuText(strings.TrimSpace(value), max(width, 1))
	padding := strings.Repeat(" ", max(width-lipgloss.Width(value), 0))
	if alignRight {
		return padding + value
	}

	return value + padding
}

func (m *model) updateProvidersCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if loaded, ok := msg.(modelsLoadedMsg); ok {
		return *m, m.completeModelsCommand(loaded)
	}

	if len(m.commandView.Providers) == 0 {
		return *m, nil
	}

	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyUp:
			selection--
		case tea.KeyDown:
			selection++
		case tea.KeyHome:
			selection = 0
		case tea.KeyEnd:
			selection = len(m.commandView.Providers) - 1
		case tea.KeyPgUp:
			selection -= max(m.getCommandViewContentHeight(), 1)
		case tea.KeyPgDown:
			selection += max(m.getCommandViewContentHeight(), 1)
		case tea.KeyEnter:
			return m.selectCurrentProviderOption()
		default:
			return *m, nil
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			selection--
		case tea.MouseWheelDown:
			selection++
		default:
			return *m, nil
		}
	default:
		return *m, nil
	}

	m.commandViewItemSelected = min(max(selection, 0), len(m.commandView.Providers)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		m.getCommandViewContentHeight(),
		len(m.commandView.Providers),
	)
	m.clearCommandViewSelection()

	return *m, nil
}

func (m *model) updateModelsCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if selected, ok := msg.(modelSelectedMsg); ok {
		return m.completeSelectModel(selected)
	}

	models := m.filteredCommandModels()
	if len(m.commandView.Models) == 0 {
		return *m, nil
	}

	selection := m.commandViewItemSelected
	switch msg := msg.(type) {
	case tea.PasteMsg:
		var cmd tea.Cmd
		m.modelFilterInput, cmd = m.modelFilterInput.Update(msg)
		m.commandViewItemSelected = 0
		m.commandViewOffset = 0
		m.clearCommandViewSelection()
		return *m, cmd
	case tea.KeyPressMsg:
		if isModelFilterKey(msg) {
			var cmd tea.Cmd
			m.modelFilterInput, cmd = m.modelFilterInput.Update(msg)
			m.commandViewItemSelected = 0
			m.commandViewOffset = 0
			m.clearCommandViewSelection()
			return *m, cmd
		}

		switch msg.Key().Code {
		case tea.KeyUp:
			selection--
		case tea.KeyDown:
			selection++
		case tea.KeyHome:
			selection = 0
		case tea.KeyEnd:
			selection = len(models) - 1
		case tea.KeyPgUp:
			selection -= max(m.getCommandViewContentHeight(), 1)
		case tea.KeyPgDown:
			selection += max(m.getCommandViewContentHeight(), 1)
		case tea.KeyLeft:
			return m.showProvidersFromModelCommand()
		case tea.KeyEnter:
			if len(models) == 0 {
				return *m, nil
			}
			return m.selectCurrentModelOption()
		default:
			return *m, nil
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			selection--
		case tea.MouseWheelDown:
			selection++
		default:
			return *m, nil
		}
	default:
		return *m, nil
	}

	if len(models) == 0 {
		m.commandViewItemSelected = 0
		m.commandViewOffset = 0
		return *m, nil
	}

	m.commandViewItemSelected = min(max(selection, 0), len(models)-1)
	m.commandViewOffset = getChatsCommandViewOffsetForSelection(
		m.commandViewItemSelected,
		m.commandViewOffset,
		max(m.getCommandViewContentHeight()-lipgloss.Height(m.renderModelFilterBlock(m.getCommandViewContentWidth())), 1),
		len(models),
	)
	m.clearCommandViewSelection()

	return *m, nil
}

func (m *model) updateProviderAPIKeyCommandView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if set, ok := msg.(providerAPIKeySetMsg); ok {
		return m.completeProviderAPIKeySet(set)
	}

	switch msg := msg.(type) {
	case tea.PasteMsg:
		msg.Content = normalizeProviderAPIKeyPaste(msg.Content)
		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		return *m, cmd
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyEsc:
			next := m.hideCommandView()
			return next, next.setStatus("provider API key cancelled")
		case tea.KeyEnter:
			return m.submitProviderAPIKey()
		}
	}

	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return *m, cmd
}

func normalizeProviderAPIKeyPaste(value string) string {
	return strings.TrimSpace(value)
}

func getModelsCommandTitleRight() string {
	return "enter to select · left to providers · esc to close"
}

func getProvidersCommandTitleRight() string {
	return "enter to view models · esc to close"
}

func (m *model) selectCurrentProviderOption() (tea.Model, tea.Cmd) {
	provider := m.commandView.Providers[m.commandViewItemSelected]
	providerID := strings.TrimSpace(provider.ID)
	if providerID == "" {
		return *m, m.setStatus("provider selection unavailable")
	}

	return *m, tea.Batch(
		m.setStatus("loading models"),
		loadModelsCmd(providerID, m.loadRawProfileMainModel()),
	)
}

func (m *model) showProvidersFromModelCommand() (tea.Model, tea.Cmd) {
	providers := modelcatalog.ListProviders(modelcatalog.ProviderQuery{
		Current: m.commandView.ModelProvider,
	})
	m.showCommandView(commandViewPayload{
		TitleLeft:       "Providers",
		TitleRight:      getProvidersCommandTitleRight(),
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindProviders,
		Providers:       providers,
	})

	return *m, nil
}

func isModelFilterKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	switch key.Code {
	case tea.KeyBackspace, tea.KeyDelete:
		return true
	}

	return key.Mod == 0 && (key.Text != "" || key.Code >= ' ' && key.Code <= '~')
}

func (m *model) selectCurrentModelOption() (tea.Model, tea.Cmd) {
	models := m.filteredCommandModels()
	if len(models) == 0 {
		return *m, nil
	}

	model := models[min(max(m.commandViewItemSelected, 0), len(models)-1)]
	modelID := strings.TrimSpace(model.ID)
	if modelID == "" {
		return *m, m.setStatus("model selection unavailable")
	}
	if model.Current {
		return *m, m.setStatus("model already selected")
	}
	if m.shouldPromptForProviderAPIKey(model) {
		return m.showProviderAPIKeyPrompt(model)
	}

	client, ok := m.modelClient.(modelSelector)
	if m.modelClient == nil || !ok {
		return *m, m.setStatus("model selection unavailable")
	}

	m.applySelectedModelToRuntime(model)
	next := m.hideCommandView()
	statusCmd := next.setStatus("selecting model")
	return next, tea.Batch(
		statusCmd,
		selectModelCmd(m.chatCtx, client, m.commandView.ModelProvider, modelID),
	)
}

func selectModelCmd(ctx context.Context, client modelSelector, provider string, modelID string) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return modelSelectedMsg{Err: errors.New("model id is required")}
		}

		model, err := client.SelectModel(ctx, modelID, rpcclient.ModelSelectOptions{Provider: provider})
		return modelSelectedMsg{Model: model, Err: err}
	}
}

func (m *model) completeSelectModel(msg modelSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if getModelSelectionLoginCommand(msg.Err) != "" {
			m.addTranscriptMessage(sessionErrorMsg{Message: getModelSelectionLoginCommand(msg.Err)})
			return *m, m.setStatus("model authentication required")
		}
		return *m, m.setStatus("model selection unavailable")
	}

	modelID := strings.TrimSpace(msg.Model.ID)
	if modelID == "" {
		return *m, m.setStatus("model selection unavailable")
	}

	m.applySelectedModelToRuntime(msg.Model)
	for index := range m.commandView.Models {
		m.commandView.Models[index].Current = strings.TrimSpace(m.commandView.Models[index].ID) == modelID
	}
	m.commandView.Models = orderModelsCommandOptions(m.commandView.Models)
	m.commandViewItemSelected = 0
	m.commandViewOffset = 0

	next := m.hideCommandView()
	return next, next.setStatus("model selected; daemon restarting")
}

func getModelSelectionLoginCommand(err error) string {
	if err == nil {
		return ""
	}

	message := err.Error()
	index := strings.Index(message, "hand auth login ")
	if index < 0 {
		return ""
	}

	command := strings.TrimSpace(message[index:])
	return "run " + command + " in a new terminal"
}

func (m model) shouldPromptForProviderAPIKey(option rpcclient.ModelOption) bool {
	if option.SupportsOAuth {
		return false
	}
	if m.hasModelAuth(option) {
		return false
	}

	return strings.TrimSpace(m.commandView.ModelProvider) != ""
}

func (m model) hasModelAuth(option rpcclient.ModelOption) bool {
	provider := strings.TrimSpace(m.commandView.ModelProvider)
	if provider == "" {
		provider = strings.TrimSpace(option.Provider)
	}
	modelID := strings.TrimSpace(option.ID)
	if provider == "" || modelID == "" {
		return false
	}

	cfg, err := config.Load(m.configEnvPath, m.configPath)
	if err != nil {
		return false
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

	return err == nil
}

func (m *model) showProviderAPIKeyPrompt(option rpcclient.ModelOption) (tea.Model, tea.Cmd) {
	provider := strings.TrimSpace(m.commandView.ModelProvider)
	if provider == "" {
		provider = strings.TrimSpace(option.Provider)
	}
	if provider == "" {
		return *m, m.setStatus("model selection unavailable")
	}

	providerLabel := getProviderDisplayName(provider)
	m.apiKeyInput = newProviderAPIKeyInput("API key for " + providerLabel)
	m.showCommandView(commandViewPayload{
		TitleLeft:       "Provider API Key",
		TitleSubtext:    providerLabel,
		TitleRight:      "enter to save · esc to cancel",
		TitleRightColor: defaultTUITheme.MutedText,
		Kind:            commandViewKindProviderAPIKey,
		ModelProvider:   provider,
		PendingModelID:  strings.TrimSpace(option.ID),
		Height:          commandViewMinHeight,
	})

	return *m, m.setStatus("provider API key required")
}

func (m model) renderProviderAPIKeyCommandViewContent(content commandViewContent) string {
	width := max(content.Width, 1)
	input := m.apiKeyInput
	input.SetWidth(width)

	return input.View()
}

func (m *model) submitProviderAPIKey() (tea.Model, tea.Cmd) {
	provider := strings.TrimSpace(m.commandView.ModelProvider)
	modelID := strings.TrimSpace(m.commandView.PendingModelID)
	apiKey := strings.TrimSpace(m.apiKeyInput.Value())
	if provider == "" || modelID == "" {
		return *m, m.setStatus("provider API key unavailable")
	}
	if apiKey == "" {
		return *m, m.setStatus("provider API key required")
	}

	client, ok := m.modelClient.(providerAPIKeySetter)
	if m.modelClient == nil || !ok {
		return *m, m.setStatus("provider API key unavailable")
	}

	return *m, tea.Batch(
		m.setStatus("saving provider API key"),
		setProviderAPIKeyCmd(m.chatCtx, client, provider, modelID, apiKey),
	)
}

func setProviderAPIKeyCmd(
	ctx context.Context,
	client providerAPIKeySetter,
	provider string,
	modelID string,
	apiKey string,
) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		err := client.SetProviderAPIKey(ctx, provider, apiKey)
		return providerAPIKeySetMsg{
			Provider: provider,
			ModelID:  modelID,
			Err:      err,
		}
	}
}

func (m *model) completeProviderAPIKeySet(msg providerAPIKeySetMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return *m, m.setStatus("provider API key unavailable")
	}

	client, ok := m.modelClient.(modelSelector)
	if m.modelClient == nil || !ok {
		return *m, m.setStatus("model selection unavailable")
	}

	next := m.hideCommandView()
	return next, tea.Batch(
		next.setStatus("selecting model"),
		selectModelCmd(m.chatCtx, client, msg.Provider, msg.ModelID),
	)
}

func (m *model) applySelectedModelToRuntime(option rpcclient.ModelOption) {
	modelID := strings.TrimSpace(option.ID)
	if modelID == "" {
		return
	}

	m.modelName = getModelDisplayName(modelID)
	m.runtimeInfo.Model = modelID
	m.runtimeInfo.SummaryModel = modelID
	if provider := strings.TrimSpace(option.Provider); provider != "" {
		m.runtimeInfo.Provider = provider
	} else if provider := strings.TrimSpace(m.commandView.ModelProvider); provider != "" {
		m.runtimeInfo.Provider = provider
	}
	m.setTranscriptContentForActiveTurn()
}

func orderModelsCommandOptions(models []rpcclient.ModelOption) []rpcclient.ModelOption {
	ordered := append([]rpcclient.ModelOption(nil), models...)
	for index, model := range ordered {
		if !model.Current || index == 0 {
			continue
		}

		current := ordered[index]
		copy(ordered[1:index+1], ordered[:index])
		ordered[0] = current
		break
	}

	return ordered
}

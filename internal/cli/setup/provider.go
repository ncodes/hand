package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	clibase "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	appcredential "github.com/wandxy/morph/internal/credential"
	modelcatalog "github.com/wandxy/morph/internal/model"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	_ "github.com/wandxy/morph/internal/model/provider_anthropic"
	_ "github.com/wandxy/morph/internal/model/provider_copilot"
	provider_ollama "github.com/wandxy/morph/internal/model/provider_ollama"
	_ "github.com/wandxy/morph/internal/model/provider_openai"
	tuirender "github.com/wandxy/morph/internal/tui/render"
)

var (
	discoverOllamaModels = func(ctx context.Context, baseURL string) ([]modelprovider.ModelDefinition, error) {
		discoverer, err := provider_ollama.NewDiscoverer(baseURL)
		if err != nil {
			return nil, err
		}

		return discoverer.DiscoverModels(ctx)
	}
	pullOllamaModel         = provider_ollama.EnsureModel
	setConfigValuesRelaxed  = config.SetConfigValuesRelaxed
	getSubscriptionProvider = appcredential.GetSubscriptionProvider
	newCredentialStore      = func() setupCredentialStore {
		return appcredential.NewFileStore("")
	}
	runSetupSelectorProgram = func(
		ctx context.Context,
		input io.Reader,
		output io.Writer,
		model selectorModel,
	) (tea.Model, error) {
		return tea.NewProgram(
			model,
			tea.WithContext(ctx),
			tea.WithInput(input),
			tea.WithOutput(output),
			tea.WithoutSignalHandler(),
		).Run()
	}
	runSetupWizardProgram = func(
		ctx context.Context,
		input io.Reader,
		output io.Writer,
		model setupWizardModel,
	) (tea.Model, error) {
		return tea.NewProgram(
			model,
			tea.WithContext(ctx),
			tea.WithInput(input),
			tea.WithOutput(output),
			tea.WithoutSignalHandler(),
		).Run()
	}
)

type setupCredentialStore interface {
	Set(string, appcredential.StoredCredential) error
}

type ProviderOptions struct {
	Input      io.Reader
	Output     io.Writer
	EnvPath    string
	ConfigPath string
	Provider   string
	Model      string
	BaseURL    string
	API        string
	APIKey     string
	Pull       bool
	PullQuiet  bool
	Registry   *modelprovider.Registry
}

type ProviderResult struct {
	Provider   string
	Model      string
	ConfigPath string
}

type providerRunner struct {
	input    io.Reader
	output   io.Writer
	registry *modelprovider.Registry
	selector func(context.Context, string, []selectChoice) (string, error)
}

type setupSelection struct {
	provider          string
	api               string
	baseURL           string
	model             string
	apiKey            string
	authMethod        string
	localModelMissing bool
	pullAnswered      bool
	pullSelected      bool
}

type modelSelection struct {
	id           string
	localMissing bool
}

type selectChoice struct {
	ID          string
	Label       string
	Description string
}

const setupOptionIndent = " "

type selectorModel struct {
	title    string
	choices  []selectChoice
	selected int
	choice   string
	err      error
}

type setupWizardStep string

const (
	setupWizardStepProvider setupWizardStep = "provider"
	setupWizardStepModel    setupWizardStep = "model"
	setupWizardStepAuth     setupWizardStep = "auth"
	setupWizardStepAPIKey   setupWizardStep = "api-key"
	setupWizardStepPull     setupWizardStep = "pull"
)

type setupWizardModel struct {
	ctx         context.Context
	runner      providerRunner
	opts        ProviderOptions
	cfg         *config.Config
	step        setupWizardStep
	choices     []selectChoice
	selected    int
	apiKeyInput textinput.Model
	selection   setupSelection
	err         error
	done        bool
}

func RunProvider(ctx context.Context, opts ProviderOptions) (ProviderResult, error) {
	opts = normalizeProviderOptions(opts)
	cfg, err := config.Load(opts.EnvPath, opts.ConfigPath)
	if err != nil {
		return ProviderResult{}, err
	}

	runner := providerRunner{input: opts.Input, output: opts.Output, registry: opts.Registry}
	selection, err := runner.getSetupSelection(ctx, opts, cfg)
	if err != nil {
		return ProviderResult{}, err
	}

	selection, err = runner.ensureSetupAuth(ctx, cfg, selection)
	if err != nil {
		return ProviderResult{}, err
	}

	if err := runner.pullMissingLocalModel(ctx, opts, selection); err != nil {
		return ProviderResult{}, err
	}

	if err := persistProviderSelection(opts, selection); err != nil {
		return ProviderResult{}, err
	}

	return ProviderResult{
		Provider:   selection.provider,
		Model:      selection.model,
		ConfigPath: opts.ConfigPath,
	}, nil
}

func normalizeProviderOptions(opts ProviderOptions) ProviderOptions {
	if opts.Input == nil {
		opts.Input = strings.NewReader("")
	}
	if opts.Output == nil {
		opts.Output = io.Discard
	}
	if opts.Registry == nil {
		opts.Registry = modelprovider.DefaultRegistry()
	}
	opts.Provider = strings.TrimSpace(strings.ToLower(opts.Provider))
	opts.Model = strings.TrimSpace(opts.Model)
	opts.BaseURL = strings.TrimSpace(opts.BaseURL)
	opts.API = strings.TrimSpace(opts.API)
	opts.APIKey = strings.TrimSpace(opts.APIKey)

	return opts
}

func (r providerRunner) getSetupSelection(
	ctx context.Context,
	opts ProviderOptions,
	cfg *config.Config,
) (setupSelection, error) {
	if r.selector == nil {
		needsPagedSetup, err := r.shouldRunPagedSetup(ctx, opts, cfg)
		if err != nil {
			return setupSelection{}, err
		}
		if needsPagedSetup {
			return r.runPagedSetup(ctx, opts, cfg)
		}
	}

	provider, err := r.getSetupProvider(ctx, opts, cfg)
	if err != nil {
		return setupSelection{}, err
	}

	providerDef, ok := r.registry.GetProvider(provider)
	if !ok || !providerDef.SupportsModels {
		return setupSelection{}, fmt.Errorf("model provider must be one of: %s", r.getProviderList())
	}

	api := opts.API
	if api == "" {
		api = strings.TrimSpace(providerDef.DefaultAPI)
	}
	if err := config.ValidateModelGenerationAPIForProvider("model API", provider, api); err != nil {
		return setupSelection{}, err
	}

	baseURL := r.getSetupBaseURL(opts, cfg, providerDef, api)
	model, err := r.getSetupModel(ctx, opts, cfg, providerDef, baseURL)
	if err != nil {
		return setupSelection{}, err
	}

	return setupSelection{
		provider:          provider,
		api:               api,
		baseURL:           baseURL,
		model:             model.id,
		apiKey:            opts.APIKey,
		localModelMissing: model.localMissing,
	}, nil
}

func (r providerRunner) shouldRunPagedSetup(
	ctx context.Context,
	opts ProviderOptions,
	cfg *config.Config,
) (bool, error) {
	if opts.Provider == "" || opts.Model == "" {
		return true, nil
	}

	providerDef, ok := r.registry.GetProvider(opts.Provider)
	if !ok || !providerDef.SupportsModels {
		return false, fmt.Errorf("model provider must be one of: %s", r.getProviderList())
	}

	api := opts.API
	if api == "" {
		api = strings.TrimSpace(providerDef.DefaultAPI)
	}
	if err := config.ValidateModelGenerationAPIForProvider("model API", opts.Provider, api); err != nil {
		return false, err
	}

	baseURL := r.getSetupBaseURL(opts, cfg, providerDef, api)
	missing, err := r.checkLocalModelMissing(ctx, opts, providerDef, baseURL, opts.Model)
	if err != nil {
		return false, err
	}

	selection := setupSelection{
		provider:          opts.Provider,
		api:               api,
		baseURL:           baseURL,
		model:             opts.Model,
		apiKey:            opts.APIKey,
		localModelMissing: missing,
	}
	if err := checkSetupAuth(cfg, selection); err != nil {
		if isMissingModelCredentialError(err) {
			return true, nil
		}

		return false, err
	}
	if selection.provider == constants.ModelProviderOllama && selection.localModelMissing && !opts.Pull {
		return true, nil
	}

	return false, nil
}

func (r providerRunner) getSetupProvider(
	ctx context.Context,
	opts ProviderOptions,
	cfg *config.Config,
) (string, error) {
	if opts.Provider != "" {
		return opts.Provider, nil
	}

	options := modelcatalog.ListProviders(modelcatalog.ProviderQuery{
		Current:  cfg.Models.Main.Provider,
		Registry: r.registry,
	})
	if len(options) == 0 {
		return "", errors.New("no model providers are available")
	}

	choices := make([]selectChoice, 0, len(options))
	for _, option := range options {
		choices = append(choices, selectChoice{
			ID:          strings.TrimSpace(option.ID),
			Label:       strings.TrimSpace(option.Name),
			Description: strings.TrimSpace(option.ID),
		})
	}

	return r.selectChoice(ctx, "Select a provider", choices)
}

func (r providerRunner) getSetupBaseURL(
	opts ProviderOptions,
	cfg *config.Config,
	provider modelprovider.ProviderDefinition,
	api string,
) string {
	if opts.BaseURL != "" {
		return opts.BaseURL
	}
	if cfg != nil && strings.EqualFold(cfg.Models.Main.Provider, provider.ID) {
		if value := strings.TrimSpace(cfg.Models.Main.BaseURL); value != "" {
			return value
		}
	}

	return strings.TrimSpace(provider.BaseURLs[strings.TrimSpace(strings.ToLower(api))])
}

func (r providerRunner) getSetupModel(
	ctx context.Context,
	opts ProviderOptions,
	cfg *config.Config,
	provider modelprovider.ProviderDefinition,
	baseURL string,
) (modelSelection, error) {
	if opts.Model != "" {
		missing, err := r.checkLocalModelMissing(ctx, opts, provider, baseURL, opts.Model)
		if err != nil {
			return modelSelection{}, err
		}

		return modelSelection{id: opts.Model, localMissing: missing}, nil
	}

	options, _, err := r.getModelOptions(ctx, provider, cfg.Models.Main.Name, baseURL)
	if err != nil {
		return modelSelection{}, err
	}
	if len(options) == 0 {
		return modelSelection{}, errors.New("models unavailable")
	}

	choices := make([]selectChoice, 0, len(options))
	for _, option := range options {
		name := strings.TrimSpace(option.Name)
		if name == "" || strings.EqualFold(name, option.ID) {
			name = option.ID
		}
		choices = append(choices, selectChoice{
			ID:          strings.TrimSpace(option.ID),
			Label:       name,
			Description: getSetupModelDescription(option),
		})
	}

	modelID, err := r.selectChoice(ctx, "Select a model", choices)
	if err != nil {
		return modelSelection{}, err
	}

	missing, err := r.checkLocalModelMissing(ctx, opts, provider, baseURL, modelID)
	if err != nil {
		return modelSelection{}, err
	}

	return modelSelection{id: modelID, localMissing: missing}, nil
}

func (r providerRunner) runPagedSetup(
	ctx context.Context,
	opts ProviderOptions,
	cfg *config.Config,
) (setupSelection, error) {
	model, err := newSetupWizardModel(ctx, r, opts, cfg)
	if err != nil {
		return setupSelection{}, err
	}

	finalModel, err := runSetupWizardProgram(ctx, r.input, r.output, model)
	if err != nil {
		return setupSelection{}, err
	}

	selected, ok := finalModel.(setupWizardModel)
	if !ok {
		return setupSelection{}, errors.New("setup selection unavailable")
	}
	if selected.err != nil {
		return setupSelection{}, selected.err
	}
	if !selected.done {
		return setupSelection{}, errors.New("setup selection cancelled")
	}

	return selected.selection, nil
}

func newSetupWizardModel(
	ctx context.Context,
	runner providerRunner,
	opts ProviderOptions,
	cfg *config.Config,
) (setupWizardModel, error) {
	model := setupWizardModel{
		ctx:    ctx,
		runner: runner,
		opts:   opts,
		cfg:    cfg,
	}
	if opts.Provider != "" {
		if err := model.setProvider(opts.Provider); err != nil {
			return setupWizardModel{}, err
		}
		return model, nil
	}

	model.showProviderPage()
	return model, nil
}

func (m *setupWizardModel) showProviderPage() {
	options := modelcatalog.ListProviders(modelcatalog.ProviderQuery{
		Current:  m.cfg.Models.Main.Provider,
		Registry: m.runner.registry,
	})
	m.choices = make([]selectChoice, 0, len(options))
	for _, option := range options {
		m.choices = append(m.choices, selectChoice{
			ID:          strings.TrimSpace(option.ID),
			Label:       strings.TrimSpace(option.Name),
			Description: strings.TrimSpace(option.ID),
		})
	}
	m.step = setupWizardStepProvider
	m.selected = 0
}

func (m *setupWizardModel) setProvider(provider string) error {
	providerDef, ok := m.runner.registry.GetProvider(provider)
	if !ok || !providerDef.SupportsModels {
		return fmt.Errorf("model provider must be one of: %s", m.runner.getProviderList())
	}

	api := m.opts.API
	if api == "" {
		api = strings.TrimSpace(providerDef.DefaultAPI)
	}
	if err := config.ValidateModelGenerationAPIForProvider("model API", provider, api); err != nil {
		return err
	}

	m.selection = setupSelection{
		provider: strings.TrimSpace(providerDef.ID),
		api:      api,
		baseURL:  m.runner.getSetupBaseURL(m.opts, m.cfg, providerDef, api),
		apiKey:   m.opts.APIKey,
	}
	if m.opts.Model != "" {
		return m.setModel(m.opts.Model)
	}

	return m.showModelPage(providerDef)
}

func (m *setupWizardModel) showModelPage(provider modelprovider.ProviderDefinition) error {
	options, _, err := m.runner.getModelOptions(
		m.ctx,
		provider,
		m.cfg.Models.Main.Name,
		m.selection.baseURL,
	)
	if err != nil {
		return err
	}
	if len(options) == 0 {
		return errors.New("models unavailable")
	}

	m.choices = make([]selectChoice, 0, len(options))
	for _, option := range options {
		name := strings.TrimSpace(option.Name)
		if name == "" || strings.EqualFold(name, option.ID) {
			name = option.ID
		}
		m.choices = append(m.choices, selectChoice{
			ID:          strings.TrimSpace(option.ID),
			Label:       name,
			Description: getSetupModelDescription(option),
		})
	}
	m.step = setupWizardStepModel
	m.selected = 0
	return nil
}

func (m *setupWizardModel) setModel(modelID string) error {
	provider, ok := m.runner.registry.GetProvider(m.selection.provider)
	if !ok {
		return fmt.Errorf("model provider must be one of: %s", m.runner.getProviderList())
	}

	missing, err := m.runner.checkLocalModelMissing(
		m.ctx,
		m.opts,
		provider,
		m.selection.baseURL,
		modelID,
	)
	if err != nil {
		return err
	}

	m.selection.model = strings.TrimSpace(modelID)
	m.selection.localModelMissing = missing
	return m.advanceAfterModel()
}

func (m *setupWizardModel) advanceAfterModel() error {
	if err := checkSetupAuth(m.cfg, m.selection); err == nil {
		return m.showPullPageOrFinish()
	} else if !isMissingModelCredentialError(err) {
		return err
	}

	provider, ok := m.runner.registry.GetProvider(m.selection.provider)
	if !ok {
		return fmt.Errorf("model provider must be one of: %s", m.runner.getProviderList())
	}
	choices := m.runner.getSetupAuthChoices(provider, m.selection)
	if len(choices) == 0 {
		return fmt.Errorf(
			"model API key is required for provider %q; run morph auth login or set a provider API key",
			m.selection.provider,
		)
	}

	m.step = setupWizardStepAuth
	m.choices = choices
	m.selected = 0
	return nil
}

func (m *setupWizardModel) showPullPageOrFinish() error {
	if m.selection.provider == constants.ModelProviderOllama &&
		m.selection.localModelMissing &&
		!m.opts.Pull {
		m.step = setupWizardStepPull
		m.choices = []selectChoice{
			{ID: "no", Label: "No"},
			{ID: "yes", Label: "Yes"},
		}
		m.selected = 0
		return nil
	}

	m.done = true
	return nil
}

func (m setupWizardModel) Init() tea.Cmd {
	return nil
}

func (m setupWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.step == setupWizardStepAPIKey {
			return m.updateAPIKey(msg)
		}
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m setupWizardModel) updateAPIKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if isSetupCancelKey(msg) {
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	}

	switch msg.Key().Code {
	case tea.KeyEnter:
		apiKey := strings.TrimSpace(m.apiKeyInput.Value())
		if apiKey == "" {
			m.err = errors.New("api key is required")
			return m, tea.Quit
		}
		m.selection.apiKey = apiKey
		m.done = true
		return m, tea.Quit
	case tea.KeyEsc:
		return m.goBack()
	default:
		input, cmd := m.apiKeyInput.Update(msg)
		m.apiKeyInput = input
		return m, cmd
	}
}

func (m setupWizardModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if isSetupCancelKey(msg) {
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	}

	switch msg.Key().Code {
	case tea.KeyUp:
		if len(m.choices) > 0 {
			m.selected = (m.selected - 1 + len(m.choices)) % len(m.choices)
		}
	case tea.KeyDown:
		if len(m.choices) > 0 {
			m.selected = (m.selected + 1) % len(m.choices)
		}
	case tea.KeyHome:
		m.selected = 0
	case tea.KeyEnd:
		m.selected = max(0, len(m.choices)-1)
	case tea.KeyLeft, tea.KeyBackspace:
		return m.goBack()
	case tea.KeyEsc:
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	case tea.KeyEnter:
		return m.chooseSelected()
	default:
		if index, ok := numericSelectionIndex(msg, len(m.choices)); ok {
			m.selected = index
			return m.chooseSelected()
		}
	}

	return m, nil
}

func (m setupWizardModel) goBack() (tea.Model, tea.Cmd) {
	switch m.step {
	case setupWizardStepProvider:
		return m, nil
	case setupWizardStepModel:
		if m.hasLockedProvider() {
			return m, nil
		}
		m.selection = setupSelection{apiKey: m.opts.APIKey}
		m.showProviderPage()
	case setupWizardStepAuth, setupWizardStepPull:
		if m.hasLockedModel() {
			return m.goBackFromLockedModel()
		}

		provider, ok := m.runner.registry.GetProvider(m.selection.provider)
		if !ok {
			m.err = fmt.Errorf("model provider must be one of: %s", m.runner.getProviderList())
			return m, tea.Quit
		}
		m.selection.model = ""
		m.selection.authMethod = ""
		m.selection.pullAnswered = false
		m.selection.pullSelected = false
		if err := m.showModelPage(provider); err != nil {
			m.err = err
			return m, tea.Quit
		}
	case setupWizardStepAPIKey:
		provider, ok := m.runner.registry.GetProvider(m.selection.provider)
		if !ok {
			m.err = fmt.Errorf("model provider must be one of: %s", m.runner.getProviderList())
			return m, tea.Quit
		}
		m.selection.apiKey = ""
		m.step = setupWizardStepAuth
		m.choices = m.runner.getSetupAuthChoices(provider, m.selection)
		m.selected = 0
	default:
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	}

	return m, nil
}

func (m setupWizardModel) goBackFromLockedModel() (tea.Model, tea.Cmd) {
	if m.hasLockedProvider() {
		return m, nil
	}

	m.selection = setupSelection{apiKey: m.opts.APIKey}
	m.showProviderPage()
	return m, nil
}

func (m setupWizardModel) hasLockedProvider() bool {
	return strings.TrimSpace(m.opts.Provider) != ""
}

func (m setupWizardModel) hasLockedModel() bool {
	return strings.TrimSpace(m.opts.Model) != ""
}

func (m setupWizardModel) chooseSelected() (tea.Model, tea.Cmd) {
	if len(m.choices) == 0 {
		m.err = errors.New("no setup options are available")
		return m, tea.Quit
	}
	if m.selected < 0 || m.selected >= len(m.choices) {
		m.selected = 0
	}

	choice := strings.TrimSpace(m.choices[m.selected].ID)
	var err error
	switch m.step {
	case setupWizardStepProvider:
		err = m.setProvider(choice)
	case setupWizardStepModel:
		err = m.setModel(choice)
	case setupWizardStepAuth:
		m.selection.authMethod = choice
		if choice == "api-key" {
			m.showAPIKeyPage()
		} else {
			m.done = true
		}
	case setupWizardStepPull:
		m.selection.pullAnswered = true
		m.selection.pullSelected = choice == "yes"
		m.done = true
	default:
		err = errors.New("setup selection unavailable")
	}
	if err != nil {
		m.err = err
		return m, tea.Quit
	}
	if m.done {
		return m, tea.Quit
	}

	return m, nil
}

func (m *setupWizardModel) showAPIKeyPage() {
	provider, _ := m.runner.registry.GetProvider(m.selection.provider)
	input := textinput.New()
	input.Placeholder = "API key for " + getProviderDisplayName(provider)
	input.CharLimit = 4096
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '*'
	styles := input.Styles()
	styles.Focused.Prompt = renderSetupAccentStyle()
	styles.Blurred.Prompt = renderSetupAccentStyle()
	input.SetStyles(styles)
	input.Focus()
	m.apiKeyInput = input
	m.step = setupWizardStepAPIKey
	m.choices = nil
	m.selected = 0
}

func (m setupWizardModel) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

func (m setupWizardModel) render() string {
	title := "Setup"
	description := "Choose setup options for this profile."
	switch m.step {
	case setupWizardStepProvider:
		title = "Select a provider"
		description = "Choose the model provider Morph should use by default."
	case setupWizardStepModel:
		title = "Select a model"
		description = "Choose the model Morph should use for chat, one-shot commands, and summaries."
	case setupWizardStepAuth:
		provider, _ := m.runner.registry.GetProvider(m.selection.provider)
		title = "Authenticate " + getProviderDisplayName(provider)
		description = "Choose how Morph should authenticate with this provider."
	case setupWizardStepAPIKey:
		provider, _ := m.runner.registry.GetProvider(m.selection.provider)
		title = "API key for " + getProviderDisplayName(provider)
		description = "Enter the API key for this provider."
	case setupWizardStepPull:
		title = "Pull " + m.selection.model + " if missing?"
		description = "Install the selected local model before saving setup."
	}
	if m.step == setupWizardStepAPIKey {
		return renderSetupInputPage(title, description, m.apiKeyInput.View())
	}

	return renderSetupChoices(title, description, m.choices, m.selected, true)
}

func (r providerRunner) getModelOptions(
	ctx context.Context,
	provider modelprovider.ProviderDefinition,
	current string,
	baseURL string,
) ([]modelcatalog.Option, bool, error) {
	if provider.ID == constants.ModelProviderOllama {
		models, err := discoverOllamaModels(ctx, baseURL)
		if err != nil {
			return nil, false, err
		}

		liveOptions := modelDefinitionsToOptions(models, current)
		catalogOptions := modelcatalog.ListOptions(modelcatalog.OptionQuery{
			Provider: provider.ID,
			Current:  current,
			Registry: r.registry,
		})

		return mergeOllamaModelOptions(liveOptions, catalogOptions), len(liveOptions) > 0, nil
	}

	return modelcatalog.ListOptions(modelcatalog.OptionQuery{
		Provider: provider.ID,
		Current:  current,
		Registry: r.registry,
	}), false, nil
}

func getSetupModelDescription(option modelcatalog.Option) string {
	description := strings.TrimSpace(option.ID)
	if option.LocalMissing {
		if description == "" {
			return "not installed"
		}
		return description + " - not installed"
	}

	return description
}

func mergeOllamaModelOptions(
	liveOptions []modelcatalog.Option,
	catalogOptions []modelcatalog.Option,
) []modelcatalog.Option {
	merged := make([]modelcatalog.Option, 0, len(liveOptions)+len(catalogOptions))
	seen := make(map[string]struct{}, len(liveOptions)+len(catalogOptions))
	for _, option := range liveOptions {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			continue
		}
		option.ID = id
		option.LocalMissing = false
		merged = append(merged, option)
		seen[strings.ToLower(id)] = struct{}{}
	}
	for _, option := range catalogOptions {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(id)]; ok {
			continue
		}
		option.ID = id
		option.LocalMissing = true
		merged = append(merged, option)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].LocalMissing != merged[j].LocalMissing {
			return !merged[i].LocalMissing
		}
		if merged[i].DisplayDefault != merged[j].DisplayDefault {
			return merged[i].DisplayDefault
		}
		if merged[i].Current != merged[j].Current {
			return merged[i].Current
		}

		return strings.ToLower(merged[i].ID) < strings.ToLower(merged[j].ID)
	})

	return merged
}

func (r providerRunner) checkLocalModelMissing(
	ctx context.Context,
	opts ProviderOptions,
	provider modelprovider.ProviderDefinition,
	baseURL string,
	modelID string,
) (bool, error) {
	if provider.ID != constants.ModelProviderOllama {
		return false, nil
	}
	if opts.Pull {
		return true, nil
	}

	models, err := discoverOllamaModels(ctx, baseURL)
	if err != nil {
		return false, err
	}
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model.ID), strings.TrimSpace(modelID)) {
			return false, nil
		}
	}

	return true, nil
}

func (r providerRunner) pullMissingLocalModel(
	ctx context.Context,
	opts ProviderOptions,
	selection setupSelection,
) error {
	if selection.provider != constants.ModelProviderOllama {
		return nil
	}

	pull := opts.Pull
	if selection.pullAnswered {
		pull = selection.pullSelected
	}
	if !pull && !selection.localModelMissing {
		return nil
	}
	if !pull {
		return nil
	}

	progressPrinter := clibase.NewPullProgressPrinter(r.output, !opts.PullQuiet)
	var onProgress func(provider_ollama.PullProgress)
	if progressPrinter != nil {
		onProgress = progressPrinter.Progress
	}

	err := pullOllamaModel(ctx, selection.baseURL, selection.model, nil, onProgress)
	if progressPrinter != nil {
		progressPrinter.Finish()
	}

	return err
}

func (r providerRunner) ensureSetupAuth(
	ctx context.Context,
	cfg *config.Config,
	selection setupSelection,
) (setupSelection, error) {
	if err := checkSetupAuth(cfg, selection); err == nil {
		return selection, nil
	} else if !isMissingModelCredentialError(err) {
		return setupSelection{}, err
	}

	provider, ok := r.registry.GetProvider(selection.provider)
	if !ok {
		return setupSelection{}, fmt.Errorf("model provider must be one of: %s", r.getProviderList())
	}
	method, err := r.getSetupAuthMethod(ctx, provider, selection)
	if err != nil {
		return setupSelection{}, err
	}

	switch method {
	case "api-key":
		if selection.apiKey == "" {
			return setupSelection{}, errors.New("setup API key is unavailable")
		}
	case "oauth":
		if err := r.loginSetupProvider(ctx, provider); err != nil {
			return setupSelection{}, err
		}
	default:
		return setupSelection{}, errors.New("authentication method unavailable")
	}
	if err := checkSetupAuth(cfg, selection); err != nil {
		return setupSelection{}, err
	}

	return selection, nil
}

func (r providerRunner) getSetupAuthMethod(
	ctx context.Context,
	provider modelprovider.ProviderDefinition,
	selection setupSelection,
) (string, error) {
	if selection.authMethod != "" {
		return selection.authMethod, nil
	}

	choices := r.getSetupAuthChoices(provider, selection)
	if len(choices) == 0 {
		return "", fmt.Errorf(
			"model API key is required for provider %q; run morph auth login or set a provider API key",
			selection.provider,
		)
	}
	if len(choices) == 1 {
		return choices[0].ID, nil
	}

	return r.selectChoice(ctx, "Authenticate "+getProviderDisplayName(provider), choices)
}

func (r providerRunner) getSetupAuthChoices(
	provider modelprovider.ProviderDefinition,
	selection setupSelection,
) []selectChoice {
	choices := make([]selectChoice, 0, 2)
	if provider.SupportsOAuth && r.modelSupportsOAuth(selection) {
		if _, ok := getSubscriptionProvider(provider.ID); ok {
			choices = append(choices, selectChoice{
				ID:          "oauth",
				Label:       "Use " + getProviderDisplayName(provider) + " account",
				Description: "subscription login",
			})
		}
	}
	if provider.SupportsAPIKey {
		choices = append(choices, selectChoice{
			ID:          "api-key",
			Label:       "Enter API key",
			Description: "stored in profile config",
		})
	}
	return choices
}

func (r providerRunner) modelSupportsOAuth(selection setupSelection) bool {
	model, ok := r.registry.GetModel(selection.provider, selection.model)
	return ok && model.SupportsOAuth
}

func (r providerRunner) loginSetupProvider(
	ctx context.Context,
	provider modelprovider.ProviderDefinition,
) error {
	subscriptionProvider, ok := getSubscriptionProvider(provider.ID)
	if !ok {
		return fmt.Errorf("subscription login is not available for %s", getProviderDisplayName(provider))
	}
	if _, err := fmt.Fprintf(r.output, "Authenticating %s...\n", getProviderDisplayName(provider)); err != nil {
		return err
	}

	credential, err := subscriptionProvider.Login(ctx, appcredential.LoginOptions{
		Provider: provider.ID,
		Input:    r.input,
		Output:   r.output,
	})
	if err != nil {
		return err
	}

	return newCredentialStore().Set(provider.ID, credential)
}

func checkSetupAuth(cfg *config.Config, selection setupSelection) error {
	if cfg == nil {
		return errors.New("config is required")
	}

	candidate := *cfg
	candidate.Models = cfg.Models
	candidate.Models.Providers = maps.Clone(cfg.Models.Providers)
	candidate.Models.Main.Provider = selection.provider
	candidate.Models.Main.Name = selection.model
	candidate.Models.Main.API = selection.api
	candidate.Models.Main.BaseURL = selection.baseURL
	candidate.Models.Summary.Provider = selection.provider
	candidate.Models.Summary.Name = selection.model
	candidate.Models.Summary.API = selection.api
	candidate.Models.Summary.BaseURL = selection.baseURL
	candidate.Search.Vector.Enabled = false
	if selection.apiKey != "" {
		if candidate.Models.Providers == nil {
			candidate.Models.Providers = make(map[string]config.ProviderModelConfig)
		}
		providerConfig := candidate.Models.Providers[selection.provider]
		providerConfig.APIKey = selection.apiKey
		candidate.Models.Providers[selection.provider] = providerConfig
	}

	_, err := candidate.ResolveModelAuth()
	return err
}

func isMissingModelCredentialError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "API key is required")
}

func getProviderDisplayName(provider modelprovider.ProviderDefinition) string {
	if value := strings.TrimSpace(provider.DisplayName); value != "" {
		return value
	}
	if value := strings.TrimSpace(provider.ID); value != "" {
		return value
	}

	return "provider"
}

func (r providerRunner) selectChoice(
	ctx context.Context,
	title string,
	choices []selectChoice,
) (string, error) {
	if r.selector != nil {
		return r.selector(ctx, title, choices)
	}

	finalModel, err := runSetupSelectorProgram(ctx, r.input, r.output, newSelectorModel(title, choices))
	if err != nil {
		return "", err
	}

	selected, ok := finalModel.(selectorModel)
	if !ok {
		return "", errors.New("setup selection unavailable")
	}
	if selected.err != nil {
		return "", selected.err
	}
	if strings.TrimSpace(selected.choice) == "" {
		return "", errors.New("setup selection cancelled")
	}

	return selected.choice, nil
}

func (r providerRunner) getProviderList() string {
	providers := r.registry.GetProviderIDs()
	sort.Strings(providers)

	return strings.Join(providers, ", ")
}

func persistProviderSelection(opts ProviderOptions, selection setupSelection) error {
	updates := []config.ConfigUpdate{
		{Path: "models.main.provider", Value: selection.provider},
		{Path: "models.main.name", Value: selection.model},
		{Path: "models.main.api", Value: selection.api},
		{Path: "models.main.baseURL", Value: selection.baseURL},
		{Path: "models.summary.provider", Value: selection.provider},
		{Path: "models.summary.name", Value: selection.model},
		{Path: "models.summary.api", Value: selection.api},
		{Path: "models.summary.baseURL", Value: selection.baseURL},
	}
	updates = append(updates, config.ModelSetupEmbeddingUpdates(selection.provider)...)
	if selection.apiKey != "" {
		updates = append(updates, config.ConfigUpdate{
			Path:  fmt.Sprintf("models.providers.%s.apiKey", selection.provider),
			Value: selection.apiKey,
		})
	}
	if _, err := setConfigValuesRelaxed(opts.EnvPath, opts.ConfigPath, updates); err != nil {
		return err
	}

	cfg, err := config.Load(opts.EnvPath, opts.ConfigPath)
	if err == nil {
		config.Set(cfg)
	}

	return nil
}

func newSelectorModel(title string, choices []selectChoice) selectorModel {
	return selectorModel{
		title:   strings.TrimSpace(title),
		choices: choices,
	}
}

func (m selectorModel) Init() tea.Cmd {
	return nil
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m selectorModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if isSetupCancelKey(msg) {
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	}

	switch msg.Key().Code {
	case tea.KeyUp:
		if len(m.choices) > 0 {
			m.selected = (m.selected - 1 + len(m.choices)) % len(m.choices)
		}
	case tea.KeyDown:
		if len(m.choices) > 0 {
			m.selected = (m.selected + 1) % len(m.choices)
		}
	case tea.KeyHome:
		m.selected = 0
	case tea.KeyEnd:
		m.selected = max(0, len(m.choices)-1)
	case tea.KeyEnter:
		return m.chooseSelected()
	case tea.KeyEsc:
		m.err = errors.New("setup selection cancelled")
		return m, tea.Quit
	default:
		if index, ok := numericSelectionIndex(msg, len(m.choices)); ok {
			m.selected = index
			return m.chooseSelected()
		}
	}

	return m, nil
}

func (m selectorModel) chooseSelected() (tea.Model, tea.Cmd) {
	if len(m.choices) == 0 {
		m.err = errors.New("no setup options are available")
		return m, tea.Quit
	}
	if m.selected < 0 || m.selected >= len(m.choices) {
		m.selected = 0
	}

	m.choice = strings.TrimSpace(m.choices[m.selected].ID)

	return m, tea.Quit
}

func (m selectorModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m selectorModel) render() string {
	return renderSetupChoices(m.title, "", m.choices, m.selected, false)
}

func renderSetupInputPage(title string, description string, input string) string {
	return strings.Join([]string{
		renderSetupTitle(title),
		renderSetupDescription(description),
		"",
		renderSetupIndentedLayer(input),
		"",
		renderSetupInputHint(),
	}, "\n")
}

func renderSetupChoices(title string, description string, choices []selectChoice, selected int, includeBackHint bool) string {
	var builder strings.Builder
	if title != "" {
		builder.WriteString(renderSetupTitle(title))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(description) != "" {
		builder.WriteString(renderSetupDescription(description))
		builder.WriteString("\n\n")
	}

	for index, choice := range choices {
		prefix := setupOptionIndent + "  "
		if index == selected {
			prefix = setupOptionIndent + renderSetupAccent(">") + " "
		}
		builder.WriteString(prefix)
		builder.WriteString(strconv.Itoa(index + 1))
		builder.WriteString(". ")
		builder.WriteString(strings.TrimSpace(choice.Label))
		if description := strings.TrimSpace(choice.Description); description != "" &&
			!strings.EqualFold(description, choice.Label) {
			builder.WriteString(" (")
			builder.WriteString(description)
			builder.WriteByte(')')
		}
		builder.WriteByte('\n')
	}

	builder.WriteByte('\n')
	builder.WriteString(renderSetupChoiceHint(includeBackHint))
	return builder.String()
}

func renderSetupTitle(value string) string {
	return lipgloss.NewStyle().Bold(true).Render(strings.TrimSpace(value))
}

func renderSetupDescription(value string) string {
	return renderSetupMuted(strings.TrimSpace(value))
}

func renderSetupChoiceHint(includeBack bool) string {
	lines := []string{
		renderSetupMuted("Use"),
		"   " + renderSetupKey("arrow") + renderSetupMuted(" to choose"),
		"   " + renderSetupKey("number") + renderSetupMuted(" to select"),
		"   " + renderSetupKey("enter") + renderSetupMuted(" to select"),
	}
	if includeBack {
		lines = append(
			lines,
			"   "+renderSetupKey("backspace")+renderSetupMuted(" or ")+renderSetupKey("back arrow")+renderSetupMuted(" to go back."),
		)
	}

	return strings.Join(lines, "\n")
}

func renderSetupInputHint() string {
	return strings.Join([]string{
		renderSetupMuted("Use"),
		"   " + renderSetupKey("enter") + renderSetupMuted(" to save"),
		"   " + renderSetupKey("esc") + renderSetupMuted(" to go back."),
	}, "\n")
}

func renderSetupAccent(value string) string {
	return renderSetupAccentStyle().Render(value)
}

func renderSetupMuted(value string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(tuirender.DefaultTheme.MutedText)).
		Render(value)
}

func renderSetupKey(value string) string {
	return lipgloss.NewStyle().Bold(true).Render(value)
}

func renderSetupAccentStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(tuirender.DefaultTheme.MarkdownLinkForeground))
}

func renderSetupIndentedLayer(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[index] = setupOptionIndent + line
		}
	}

	return strings.Join(lines, "\n")
}

func numericSelectionIndex(msg tea.KeyPressMsg, length int) (int, bool) {
	value := strings.TrimSpace(msg.Key().Text)
	if value == "" && msg.Key().Code >= '0' && msg.Key().Code <= '9' {
		value = string(msg.Key().Code)
	}
	index, ok := parseSelectionIndex(value, length)

	return index, ok
}

func isSetupCancelKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return msg.String() == "ctrl+c" || key.Code == 'c' && key.Mod == tea.ModCtrl
}

func parseSelectionIndex(value string, length int) (int, bool) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 1 || number > length {
		return 0, false
	}

	return number - 1, true
}

func modelDefinitionsToOptions(
	models []modelprovider.ModelDefinition,
	current string,
) []modelcatalog.Option {
	options := make([]modelcatalog.Option, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		inputs := make([]string, 0, len(model.Input))
		for _, input := range model.Input {
			value := strings.TrimSpace(string(input))
			if value != "" {
				inputs = append(inputs, value)
			}
		}
		options = append(options, modelcatalog.Option{
			ID:             strings.TrimSpace(model.ID),
			Name:           strings.TrimSpace(model.Name),
			Provider:       strings.TrimSpace(model.Provider),
			API:            strings.TrimSpace(model.API),
			ContextWindow:  model.ContextWindow,
			MaxTokens:      model.MaxTokens,
			Input:          inputs,
			Reasoning:      model.Reasoning,
			SupportsTools:  model.SupportsTools,
			DisplayDefault: model.DisplayDefault,
			Current:        strings.TrimSpace(model.ID) == strings.TrimSpace(current),
		})
	}

	sort.Slice(options, func(i, j int) bool {
		if options[i].Current != options[j].Current {
			return options[i].Current
		}

		return strings.ToLower(options[i].ID) < strings.ToLower(options[j].ID)
	})

	return options
}

package model

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
)

type Option struct {
	ID             string
	Name           string
	Provider       string
	API            string
	ContextWindow  int
	MaxTokens      int
	Input          []string
	Reasoning      bool
	SupportsTools  bool
	SupportsOAuth  bool
	DisplayDefault bool
	Current        bool
	LocalMissing   bool
	BaseURL        string
	Source         OptionSource
}

type OptionSource string

const (
	OptionSourceCatalog   OptionSource = "catalog"
	OptionSourceConfig    OptionSource = "config"
	OptionSourceDiscovery OptionSource = "discovery"
)

type ProviderOption struct {
	ID              string
	Name            string
	DisplayIndex    int
	HasDisplayIndex bool
	Type            string
	ModelCount      int
	SupportsAPIKey  bool
	SupportsOAuth   bool
	Local           bool
	AuthType        string
	Current         bool
}

type OptionQuery struct {
	Context             context.Context
	Provider            string
	Current             string
	OAuthOnly           bool
	Registry            *modelprovider.Registry
	Config              *config.Config
	BaseURL             string
	LocalDiscovery      bool
	Refresh             bool
	DiscoveryTTL        time.Duration
	DiscoverLocalModels func(context.Context, string) ([]modelprovider.ModelDefinition, error)
}

type ProviderQuery struct {
	Current    string
	Auth       map[string]string
	OAuthOnly  bool
	APIKeyOnly bool
	Registry   *modelprovider.Registry
}

type localDiscoveryCacheEntry struct {
	options []Option
	stored  time.Time
}

var localDiscoveryCache = struct {
	sync.Mutex
	values map[string]localDiscoveryCacheEntry
}{
	values: make(map[string]localDiscoveryCacheEntry),
}

const defaultLocalDiscoveryTTL = 30 * time.Second

func ListOptions(query OptionQuery) ([]Option, error) {
	registry := query.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}

	provider := strings.TrimSpace(strings.ToLower(query.Provider))
	current := strings.TrimSpace(query.Current)
	options := listRegistryOptions(registry, provider, current, query.OAuthOnly)

    hasExplicitConfig := hasExplicitProviderModelDefinitions(query.Config, provider)
	explicitOptions := listExplicitConfigOptions(query.Config, registry, provider, current, query.OAuthOnly)
	if len(explicitOptions) > 0 {
		options = mergeOptions(explicitOptions, options, false)
	}

	providerDef, _ := registry.GetProvider(provider)
	if query.LocalDiscovery &&
		query.DiscoverLocalModels != nil &&
		providerDef.Local != nil &&
		provider == constants.ModelProviderOllama &&
		!hasExplicitConfig {
		discovered, err := getDiscoveredLocalOptions(query, providerDef, current)
		if err != nil {
			return nil, err
		}
		options = mergeOptions(discovered, options, true)
	}

	sortOptions(options)

	return options, nil
}

func getDiscoveredLocalOptions(
	query OptionQuery,
	provider modelprovider.ProviderDefinition,
	current string,
) ([]Option, error) {
	ctx := query.Context
	if ctx == nil {
		ctx = context.Background()
	}

	baseURL := getLocalDiscoveryBaseURL(query, provider)
	cacheKey := strings.Join([]string{provider.ID, baseURL}, "\x00")
	ttl := query.DiscoveryTTL
	if ttl <= 0 {
		ttl = defaultLocalDiscoveryTTL
	}
	if !query.Refresh {
		if options, ok := getCachedLocalDiscoveryOptions(cacheKey, ttl, current); ok {
			return options, nil
		}
	}

	models, err := query.DiscoverLocalModels(ctx, baseURL)
	if err != nil {
		return nil, err
	}

	options := modelDefinitionsToOptions(models, current, baseURL, OptionSourceDiscovery)
	setCachedLocalDiscoveryOptions(cacheKey, options)

	return cloneOptionsWithCurrent(options, current), nil
}

func getLocalDiscoveryBaseURL(query OptionQuery, provider modelprovider.ProviderDefinition) string {
	if value := strings.TrimSpace(query.BaseURL); value != "" {
		return value
	}
	api := strings.TrimSpace(provider.DefaultAPI)
	if query.Config != nil {
		if strings.EqualFold(query.Config.Models.Main.Provider, provider.ID) {
			if value := strings.TrimSpace(query.Config.Models.Main.BaseURL); value != "" {
				return value
			}
			if value := strings.TrimSpace(query.Config.Models.Main.API); value != "" {
				api = value
			}
		}
		if providerConfig, ok := getExplicitProviderConfig(query.Config, provider.ID); ok {
			if value := strings.TrimSpace(providerConfig.BaseURL); value != "" {
				return value
			}
			if value := strings.TrimSpace(providerConfig.API); value != "" {
				api = value
			}
		}
	}

	return strings.TrimSpace(provider.BaseURLs[strings.TrimSpace(strings.ToLower(api))])
}

func getCachedLocalDiscoveryOptions(cacheKey string, ttl time.Duration, current string) ([]Option, bool) {
	localDiscoveryCache.Lock()
	defer localDiscoveryCache.Unlock()

	entry, ok := localDiscoveryCache.values[cacheKey]
	if !ok || time.Since(entry.stored) > ttl {
		return nil, false
	}

	return cloneOptionsWithCurrent(entry.options, current), true
}

func setCachedLocalDiscoveryOptions(cacheKey string, options []Option) {
	localDiscoveryCache.Lock()
	defer localDiscoveryCache.Unlock()

	localDiscoveryCache.values[cacheKey] = localDiscoveryCacheEntry{
		options: cloneOptionsWithCurrent(options, ""),
		stored:  time.Now(),
	}
}

func cloneOptionsWithCurrent(options []Option, current string) []Option {
	cloned := make([]Option, 0, len(options))
	current = strings.TrimSpace(current)
	for _, option := range options {
		option.Input = append([]string(nil), option.Input...)
		option.Current = strings.TrimSpace(option.ID) == current
		cloned = append(cloned, option)
	}

	return cloned
}

func ListProviders(query ProviderQuery) []ProviderOption {
	registry := query.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}

	current := strings.TrimSpace(strings.ToLower(query.Current))
	providers := registry.GetProviders()
	options := make([]ProviderOption, 0, len(providers))
	for _, provider := range providers {
		if !provider.SupportsModels {
			continue
		}
		if query.OAuthOnly && !provider.SupportsOAuth {
			continue
		}
		if query.APIKeyOnly && !provider.SupportsAPIKey {
			continue
		}

		count := countGenerationModels(registry, provider.ID)
		if count == 0 {
			continue
		}

		options = append(options, ProviderOption{
			ID:              strings.TrimSpace(provider.ID),
			Name:            strings.TrimSpace(provider.DisplayName),
			DisplayIndex:    provider.DisplayIndex,
			HasDisplayIndex: provider.HasDisplayIndex,
			Type:            getProviderOptionType(provider),
			ModelCount:      count,
			SupportsAPIKey:  provider.SupportsAPIKey,
			SupportsOAuth:   provider.SupportsOAuth,
			Local:           provider.Local != nil,
			AuthType:        strings.TrimSpace(query.Auth[provider.ID]),
			Current:         strings.TrimSpace(provider.ID) == current,
		})
	}

	sort.Slice(options, func(i, j int) bool {
		if options[i].HasDisplayIndex != options[j].HasDisplayIndex {
			return options[i].HasDisplayIndex
		}
		if options[i].HasDisplayIndex && options[i].DisplayIndex != options[j].DisplayIndex {
			return options[i].DisplayIndex < options[j].DisplayIndex
		}
		if options[i].Current != options[j].Current {
			return options[i].Current
		}

		return strings.ToLower(options[i].ID) < strings.ToLower(options[j].ID)
	})

	return options
}

func isGenerationAPI(api string) bool {
	switch strings.TrimSpace(api) {
	case modelprovider.APIOpenAICompletions,
		modelprovider.APIOpenAIResponses,
		modelprovider.APIOllamaNative,
		modelprovider.APIAnthropicMessages:
		return true
	default:
		return false
	}
}

func countGenerationModels(registry *modelprovider.Registry, provider string) int {
	count := 0
	for _, model := range registry.GetModels(provider) {
		if isGenerationAPI(model.API) {
			count++
		}
	}

	return count
}

func listRegistryOptions(
	registry *modelprovider.Registry,
	provider string,
	current string,
	oauthOnly bool,
) []Option {
	models := registry.GetModels(provider)
	options := make([]Option, 0, len(models))
	for _, model := range models {
		if !isGenerationAPI(model.API) {
			continue
		}
		if oauthOnly && !model.SupportsOAuth {
			continue
		}

		option := modelDefinitionToOption(model, current)
		option.Source = OptionSourceCatalog
		options = append(options, option)
	}

	sortOptions(options)

	return options
}

func listExplicitConfigOptions(
	cfg *config.Config,
	registry *modelprovider.Registry,
	provider string,
	current string,
	oauthOnly bool,
) []Option {
	if cfg == nil || len(cfg.Models.Providers) == 0 {
		return nil
	}

	providerConfig, ok := getExplicitProviderConfig(cfg, provider)
	if !ok || len(providerConfig.Models) == 0 {
		return nil
	}

	api := strings.TrimSpace(providerConfig.API)
	if api == "" {
		api = getProviderDefaultAPI(registry, provider)
	}
	if !isGenerationAPI(api) {
		return nil
	}

	options := make([]Option, 0, len(providerConfig.Models))
	for modelID, metadata := range providerConfig.Models {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}

		option := Option{
			ID:             modelID,
			Name:           modelID,
			Provider:       provider,
			API:            api,
			ContextWindow:  metadata.ContextLength,
			MaxTokens:      int(metadata.MaxOutputTokens),
			Input:          []string{string(modelprovider.InputText)},
			Current:        modelID == strings.TrimSpace(current),
			LocalMissing:   false,
			BaseURL:        strings.TrimSpace(providerConfig.BaseURL),
			Source:         OptionSourceConfig,
			SupportsTools:  boolPtrValue(metadata.SupportsTools),
			Reasoning:      boolPtrValue(metadata.Reasoning),
			DisplayDefault: modelID == strings.TrimSpace(current),
		}
		if boolPtrValue(metadata.SupportsVision) {
			option.Input = append(option.Input, string(modelprovider.InputImage))
		}
		if oauthOnly && !option.SupportsOAuth {
			continue
		}

		options = append(options, option)
	}

	sortOptions(options)

	return options
}

func hasExplicitProviderModelDefinitions(cfg *config.Config, provider string) bool {
	providerConfig, ok := getExplicitProviderConfig(cfg, provider)
	return ok && len(providerConfig.Models) > 0
}

func getExplicitProviderConfig(
	cfg *config.Config,
	provider string,
) (config.ProviderModelConfig, bool) {
	if cfg == nil || len(cfg.Models.Providers) == 0 {
		return config.ProviderModelConfig{}, false
	}

	provider = strings.TrimSpace(strings.ToLower(provider))
	if providerConfig, ok := cfg.Models.Providers[provider]; ok {
		return providerConfig, true
	}

	for key, providerConfig := range cfg.Models.Providers {
		if strings.EqualFold(strings.TrimSpace(key), provider) {
			return providerConfig, true
		}
	}

	return config.ProviderModelConfig{}, false
}

func getProviderDefaultAPI(registry *modelprovider.Registry, provider string) string {
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}
	providerDef, ok := registry.GetProvider(provider)
	if !ok {
		return ""
	}

	return strings.TrimSpace(providerDef.DefaultAPI)
}

func boolPtrValue(value *bool) bool {
	return value != nil && *value
}

func mergeOptions(primary []Option, secondary []Option, markSecondaryMissing bool) []Option {
	merged := make([]Option, 0, len(primary)+len(secondary))
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	for _, option := range primary {
		option.ID = strings.TrimSpace(option.ID)
		if option.ID == "" {
			continue
		}
		merged = append(merged, option)
		seen[strings.ToLower(option.ID)] = struct{}{}
	}
	for _, option := range secondary {
		option.ID = strings.TrimSpace(option.ID)
		if option.ID == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(option.ID)]; ok {
			continue
		}
		if markSecondaryMissing {
			option.LocalMissing = true
		}
		merged = append(merged, option)
	}

	return merged
}

func sortOptions(options []Option) {
	sort.Slice(options, func(i, j int) bool {
		if options[i].LocalMissing != options[j].LocalMissing {
			return !options[i].LocalMissing
		}
		if options[i].DisplayDefault != options[j].DisplayDefault {
			return options[i].DisplayDefault
		}
		if options[i].Current != options[j].Current {
			return options[i].Current
		}

		return strings.ToLower(options[i].ID) < strings.ToLower(options[j].ID)
	})
}

func getProviderOptionType(provider modelprovider.ProviderDefinition) string {
	switch {
	case provider.Local != nil:
		return "local"
	case provider.SupportsAPIKey && provider.SupportsOAuth:
		return "api-key/oauth"
	case provider.SupportsOAuth:
		return "oauth"
	case provider.SupportsAPIKey:
		return "api-key"
	default:
		return "none"
	}
}

func modelDefinitionToOption(model modelprovider.ModelDefinition, current string) Option {
	inputs := make([]string, 0, len(model.Input))
	for _, input := range model.Input {
		value := strings.TrimSpace(string(input))
		if value != "" {
			inputs = append(inputs, value)
		}
	}

	return Option{
		ID:             strings.TrimSpace(model.ID),
		Name:           strings.TrimSpace(model.Name),
		Provider:       strings.TrimSpace(model.Provider),
		API:            strings.TrimSpace(model.API),
		ContextWindow:  model.ContextWindow,
		MaxTokens:      model.MaxTokens,
		Input:          inputs,
		Reasoning:      model.Reasoning,
		SupportsTools:  model.SupportsTools,
		SupportsOAuth:  model.SupportsOAuth,
		DisplayDefault: model.DisplayDefault,
		Current:        strings.TrimSpace(model.ID) == current,
		Source:         OptionSourceCatalog,
	}
}

func modelDefinitionsToOptions(
	models []modelprovider.ModelDefinition,
	current string,
	baseURL string,
	source OptionSource,
) []Option {
	options := make([]Option, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		option := modelDefinitionToOption(model, current)
		option.BaseURL = strings.TrimSpace(baseURL)
		option.Source = source
		options = append(options, option)
	}

	sortOptions(options)

	return options
}

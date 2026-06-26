package model

import (
	"sort"
	"strings"

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
}

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
	Provider  string
	Current   string
	OAuthOnly bool
	Registry  *modelprovider.Registry
}

type ProviderQuery struct {
	Current    string
	Auth       map[string]string
	OAuthOnly  bool
	APIKeyOnly bool
	Registry   *modelprovider.Registry
}

func ListOptions(query OptionQuery) []Option {
	registry := query.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}

	provider := strings.TrimSpace(strings.ToLower(query.Provider))
	current := strings.TrimSpace(query.Current)
	models := registry.GetModels(provider)
	options := make([]Option, 0, len(models))
	for _, model := range models {
		if !isGenerationAPI(model.API) {
			continue
		}
		if query.OAuthOnly && !model.SupportsOAuth {
			continue
		}

		options = append(options, modelDefinitionToOption(model, current))
	}

	sort.Slice(options, func(i, j int) bool {
		if options[i].DisplayDefault != options[j].DisplayDefault {
			return options[i].DisplayDefault
		}
		if options[i].Current != options[j].Current {
			return options[i].Current
		}

		return strings.ToLower(options[i].ID) < strings.ToLower(options[j].ID)
	})

	return options
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
	}
}

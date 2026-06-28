package setup

import (
	"context"
	"strings"

	"github.com/wandxy/morph/internal/config"
	modelcatalog "github.com/wandxy/morph/internal/model"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
)

type ModelOptions struct {
	Provider  string
	Current   string
	BaseURL   string
	OAuthOnly bool
	Config    *config.Config
	Refresh   bool
	Registry  *modelprovider.Registry
}

func ListModelOptions(ctx context.Context, opts ModelOptions) ([]modelcatalog.Option, bool, error) {
	registry := opts.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}

	options, err := modelcatalog.ListOptions(modelcatalog.OptionQuery{
		Context:             ctx,
		Provider:            strings.TrimSpace(opts.Provider),
		Current:             opts.Current,
		OAuthOnly:           opts.OAuthOnly,
		Config:              opts.Config,
		BaseURL:             ResolveModelOptionsBaseURL(opts),
		LocalDiscovery:      true,
		Refresh:             opts.Refresh,
		Registry:            registry,
		DiscoverLocalModels: discoverOllamaModels,
	})

	return options, hasInstalledLocalOptions(options), err
}

func ResolveModelOptionsBaseURL(opts ModelOptions) string {
	if value := strings.TrimSpace(opts.BaseURL); value != "" {
		return value
	}

	registry := opts.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}
	providerID := strings.TrimSpace(strings.ToLower(opts.Provider))
	provider, ok := registry.GetProvider(providerID)
	if !ok {
		return ""
	}

	api := strings.TrimSpace(provider.DefaultAPI)
	if opts.Config != nil {
		if strings.EqualFold(opts.Config.Models.Main.Provider, provider.ID) {
			if value := strings.TrimSpace(opts.Config.Models.Main.BaseURL); value != "" {
				return value
			}
			if value := strings.TrimSpace(opts.Config.Models.Main.API); value != "" {
				api = value
			}
		}
		if providerConfig, ok := getProviderModelConfig(opts.Config, provider.ID); ok {
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

func getProviderModelConfig(cfg *config.Config, provider string) (config.ProviderModelConfig, bool) {
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

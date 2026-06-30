package setup

import (
	"context"
	"strings"

	"github.com/wandxy/morph/internal/config"
	modelcatalog "github.com/wandxy/morph/internal/model"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	"github.com/wandxy/morph/pkg/stringx"
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
		Provider:            stringx.String(opts.Provider).Trim(),
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
	if value := stringx.String(opts.BaseURL).Trim(); value != "" {
		return value
	}

	registry := opts.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}
	providerID := stringx.String(opts.Provider).Normalized()
	provider, ok := registry.GetProvider(providerID)
	if !ok {
		return ""
	}

	api := stringx.String(provider.DefaultAPI).Trim()
	if opts.Config != nil {
		if strings.EqualFold(opts.Config.Models.Main.Provider, provider.ID) {
			if value := stringx.String(opts.Config.Models.Main.BaseURL).Trim(); value != "" {
				return normalizeInheritedSetupBaseURL(provider.ID, provider.DefaultAPI, value)
			}
			if value := stringx.String(opts.Config.Models.Main.API).Trim(); value != "" {
				api = value
			}
		}
		if providerConfig, ok := getProviderModelConfig(opts.Config, provider.ID); ok {
			if value := stringx.String(providerConfig.BaseURL).Trim(); value != "" {
				return normalizeInheritedSetupBaseURL(provider.ID, provider.DefaultAPI, value)
			}
			if value := stringx.String(providerConfig.API).Trim(); value != "" {
				api = value
			}
		}
	}

	return stringx.String(provider.BaseURLs[stringx.String(api).Normalized()]).Trim()
}

func getProviderModelConfig(cfg *config.Config, provider string) (config.ProviderModelConfig, bool) {
	if cfg == nil || len(cfg.Models.Providers) == 0 {
		return config.ProviderModelConfig{}, false
	}

	provider = stringx.String(provider).Normalized()
	if providerConfig, ok := cfg.Models.Providers[provider]; ok {
		return providerConfig, true
	}
	for key, providerConfig := range cfg.Models.Providers {
		if strings.EqualFold(stringx.String(key).Trim(), provider) {
			return providerConfig, true
		}
	}

	return config.ProviderModelConfig{}, false
}

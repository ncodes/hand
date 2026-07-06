package setup

import (
	"context"
	"strings"

	"github.com/wandxy/morph/internal/config"
	modelcatalog "github.com/wandxy/morph/internal/model"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(opts.Provider)
	options, err := modelcatalog.ListOptions(modelcatalog.OptionQuery{
		Context:             ctx,
		Provider:            stringValue1.Trim(),
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
	stringValue2 := str.String(opts.BaseURL)
	if value := stringValue2.Trim(); value != "" {
		return value
	}

	registry := opts.Registry
	if registry == nil {
		registry = modelprovider.DefaultRegistry()
	}
	stringValue3 := str.String(opts.Provider)
	providerID := stringValue3.Normalized()
	provider, ok := registry.GetProvider(providerID)
	if !ok {
		return ""
	}
	stringValue4 := str.String(provider.DefaultAPI)
	api := stringValue4.Trim()
	if opts.Config != nil {
		if strings.EqualFold(opts.Config.Models.Main.Provider, provider.ID) {
			stringValue7 := str.String(opts.Config.Models.Main.BaseURL)
			if value := stringValue7.Trim(); value != "" {
				return normalizeInheritedSetupBaseURL(provider.ID, provider.DefaultAPI, value)
			}
			stringValue8 := str.String(opts.Config.Models.Main.API)
			if value := stringValue8.Trim(); value != "" {
				api = value
			}
		}
		if providerConfig, ok := getProviderModelConfig(opts.Config, provider.ID); ok {
			stringValue9 := str.String(providerConfig.BaseURL)
			if value := stringValue9.Trim(); value != "" {
				return normalizeInheritedSetupBaseURL(provider.ID, provider.DefaultAPI, value)
			}
			stringValue10 := str.String(providerConfig.API)
			if value := stringValue10.Trim(); value != "" {
				api = value
			}
		}
	}
	stringValue5 := str.String(api)
	stringValue6 := str.String(provider.BaseURLs[stringValue5.Normalized()])
	return stringValue6.Trim()
}

func getProviderModelConfig(cfg *config.Config, provider string) (config.ProviderModelConfig, bool) {
	if cfg == nil || len(cfg.Models.Providers) == 0 {
		return config.ProviderModelConfig{}, false
	}
	stringValue11 := str.String(provider)
	provider = stringValue11.Normalized()
	if providerConfig, ok := cfg.Models.Providers[provider]; ok {
		return providerConfig, true
	}
	for key, providerConfig := range cfg.Models.Providers {
		stringValue12 := str.String(key)
		if strings.EqualFold(stringValue12.Trim(), provider) {
			return providerConfig, true
		}
	}

	return config.ProviderModelConfig{}, false
}

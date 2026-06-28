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
		BaseURL:             opts.BaseURL,
		LocalDiscovery:      true,
		Refresh:             opts.Refresh,
		Registry:            registry,
		DiscoverLocalModels: discoverOllamaModels,
	})

	return options, hasInstalledLocalOptions(options), err
}

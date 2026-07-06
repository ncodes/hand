package readiness

import (
	"fmt"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/str"
)

var resolveWebAPIKeySource = func(cfg *config.Config) (config.WebCredentialSource, error) {
	return cfg.WebAPIKeySourceEffective()
}

func buildCapabilityGroup(cfg *config.Config) Group {
	if cfg == nil {
		return Group{Name: "tools", Checks: []Check{check("config", StatusFail, "config is required")}}
	}

	checks := []Check{
		buildWebCheck(cfg),
	}

	return Group{Name: "tools", Checks: checks}
}

func buildWebCheck(cfg *config.Config) Check {
	if cfg.Cap.Network != nil && !*cfg.Cap.Network {
		return check("web tools", StatusWarn, "network capability is disabled")
	}
	stringValue1 := str.String(cfg.Web.Provider)
	provider := stringValue1.Normalized()
	if provider == "" || provider == "native" {
		return check("web tools", StatusWarn, "native web extraction is configured; web search requires a configured web provider")
	}
	if !config.IsWebCredentialProvider(provider) {
		return check("web tools", StatusWarn, fmt.Sprintf("web provider %q does not use managed credentials", provider))
	}

	source, err := resolveWebAPIKeySource(cfg)
	if err != nil {
		return check("web tools", StatusWarn, err.Error(), webAuthAction(provider))
	}
	if source.Configured {
		return check("web tools", StatusPass, fmt.Sprintf("%s web credentials are configured via %s", provider, source.Source))
	}

	return check(
		"web tools",
		StatusWarn,
		fmt.Sprintf("%s web credentials are not configured", provider),
		webAuthAction(provider),
	)
}

func webAuthAction(provider string) Action {
	stringValue2 := str.String(provider)
	if stringValue2.Trim() == "" {
		provider = constants.WebProviderExa
	}

	return commandAction(
		fmt.Sprintf("morph config set web.provider %s && morph config set web.apiKey <api-key>", provider),
		"configure web provider credentials",
	)
}

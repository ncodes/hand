package readiness

import (
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
)

var resolveWebAPIKeySource = func(cfg *config.Config) (config.WebCredentialSource, error) {
	return cfg.WebAPIKeySourceEffective()
}

func buildCapabilityGroup(cfg *config.Config) Group {
	if cfg == nil {
		return Group{Name: "capabilities", Checks: []Check{check("config", StatusFail, "config is required")}}
	}

	checks := []Check{
		buildMemoryCheck(cfg),
		buildVectorSearchCheck(cfg),
		buildRerankerCheck(cfg),
		buildWebCheck(cfg),
	}

	return Group{Name: "capabilities", Checks: checks}
}

func buildMemoryCheck(cfg *config.Config) Check {
	if !cfg.MemoryEnabled() {
		return check("memory", StatusWarn, "memory is disabled")
	}

	return check(
		"memory",
		StatusPass,
		fmt.Sprintf(
			"enabled with provider %q, backend %q, retrieval=%t, flush=%t, write=%t",
			cfg.Memory.Provider,
			cfg.Memory.Backend,
			cfg.MemoryRetrievalEnabled(),
			cfg.MemoryFlushEnabled(),
			cfg.MemoryWriteEnabled(),
		),
	)
}

func buildVectorSearchCheck(cfg *config.Config) Check {
	if !cfg.Search.Vector.Enabled {
		return check("vector search", StatusWarn, "vector search is disabled")
	}

	status := StatusPass
	message := "vector search is enabled"
	if cfg.Search.Vector.Required {
		message = "vector search is enabled and required"
	}
	if cfg.Search.Vector.Required {
		if _, err := cfg.ResolveEmbeddingModelAuth(); err != nil {
			status = StatusFail
			message = "required vector search cannot resolve embedding auth: " + err.Error()
		}
	}

	return check("vector search", status, message)
}

func buildRerankerCheck(cfg *config.Config) Check {
	enabled := true
	if cfg.Reranker.Enabled != nil {
		enabled = *cfg.Reranker.Enabled
	}
	if cfg.Search.EnableRerank != nil && !*cfg.Search.EnableRerank {
		enabled = false
	}
	if !enabled {
		return check("reranker", StatusWarn, "reranking is disabled")
	}

	return check("reranker", StatusPass, fmt.Sprintf("using %q reranker", cfg.RerankerEffective()))
}

func buildWebCheck(cfg *config.Config) Check {
	if cfg.Cap.Network != nil && !*cfg.Cap.Network {
		return check("web tools", StatusWarn, "network capability is disabled")
	}

	provider := strings.TrimSpace(strings.ToLower(cfg.Web.Provider))
	if provider == "" {
		return check("web tools", StatusWarn, "no web provider configured")
	}
	if provider == "native" {
		return check("web tools", StatusPass, "native web extraction is configured")
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
	if strings.TrimSpace(provider) == "" {
		provider = constants.WebProviderExa
	}

	return commandAction(
		fmt.Sprintf("hand config set web.provider %s && hand config set web.apiKey <api-key>", provider),
		"configure web provider credentials",
	)
}

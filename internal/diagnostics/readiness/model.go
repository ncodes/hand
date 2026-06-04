package readiness

import (
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
)

func buildModelGroup(cfg *config.Config) Group {
	if cfg == nil {
		return Group{
			Name:   "models",
			Checks: []Check{check("config", StatusFail, "config is required")},
		}
	}

	checks := []Check{
		buildModelRoleCheck("main", cfg.Models.Main.Provider, cfg.Models.Main.Name, cfg.ResolveModelAuth),
		buildModelRoleCheck("summary", cfg.SummaryProviderEffective(), cfg.SummaryModelEffective(),
			cfg.ResolveSummaryModelAuth),
		buildEmbeddingRoleCheck(cfg),
	}

	return Group{Name: "models", Checks: checks}
}

func buildEmbeddingRoleCheck(cfg *config.Config) Check {
	if cfg.Search.Vector.Enabled {
		return buildModelRoleCheck("embedding", cfg.ModelEmbeddingProviderEffective(), cfg.Models.Embedding.Name,
			cfg.ResolveEmbeddingModelAuth)
	}

	return check(
		"embedding",
		StatusWarn,
		fmt.Sprintf(
			"embedding model %q on provider %q is configured; vector search is disabled",
			defaultString(cfg.Models.Embedding.Name, "default"),
			defaultString(cfg.ModelEmbeddingProviderEffective(), cfg.Models.Main.Provider),
		),
	)
}

func buildModelRoleCheck(
	role string,
	provider string,
	model string,
	resolve func() (config.ModelAuth, error),
) Check {
	auth, err := resolve()
	if err != nil {
		return check(
			role,
			StatusFail,
			err.Error(),
			modelErrorActions(provider, err)...,
		)
	}

	return check(
		role,
		StatusPass,
		fmt.Sprintf(
			"%s model %q on provider %q using %s auth",
			role,
			defaultString(model, "default"),
			defaultString(auth.Provider, provider),
			formatCredentialSource(auth),
		),
	)
}

func modelErrorActions(provider string, err error) []Action {
	actions := []Action{
		commandAction("/providers", "review known providers in the TUI"),
		commandAction("/models", "review models for the selected provider"),
	}
	if isMissingAuthError(err) {
		actions = append([]Action{missingAuthAction(provider)}, actions...)
	}

	return actions
}

func isMissingAuthError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "api key is required") ||
		strings.Contains(message, "hand auth login")
}

func missingAuthAction(provider string) Action {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = constants.DefaultModelProvider
	}
	switch provider {
	case constants.ModelProviderOpenAI, constants.ModelProviderOpenAICodex,
		constants.ModelProviderAnthropic, constants.ModelProviderGitHubCopilot:
		return commandAction("hand auth login "+provider, "store OAuth credentials for this provider")
	default:
		return commandAction(
			fmt.Sprintf("hand config set models.providers.%s.apiKey <api-key>", provider),
			"configure a provider API key",
		)
	}
}

func formatCredentialSource(auth config.ModelAuth) string {
	source := auth.CredentialSource
	switch source.Kind {
	case config.ModelCredentialSourceRoleConfig:
		return "role-config"
	case config.ModelCredentialSourceProviderConfig:
		return "provider-config"
	case config.ModelCredentialSourceProviderEnv:
		if strings.TrimSpace(source.Type) != "" {
			return strings.TrimSpace(source.Type) + " env"
		}
		return "environment"
	case config.ModelCredentialSourceTokenStore:
		parts := []string{"token-store"}
		if strings.TrimSpace(source.Type) != "" {
			parts = append(parts, strings.TrimSpace(source.Type))
		}
		if source.HasExpiry {
			parts = append(parts, "refreshable")
		}
		return strings.Join(parts, " ")
	default:
		return auth.AuthType()
	}
}

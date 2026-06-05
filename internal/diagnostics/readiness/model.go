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
		return buildModelRoleCheckWithActions(
			"embedding",
			cfg.ModelEmbeddingProviderEffective(),
			cfg.Models.Embedding.Name,
			cfg.ResolveEmbeddingModelAuth,
			embeddingModelErrorActions,
		)
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
	return buildModelRoleCheckWithActions(role, provider, model, resolve, modelErrorActions)
}

func buildModelRoleCheckWithActions(
	role string,
	provider string,
	model string,
	resolve func() (config.ModelAuth, error),
	actions func(string, error) []Action,
) Check {
	auth, err := resolve()
	if err != nil {
		return check(
			role,
			StatusFail,
			err.Error(),
			actions(provider, err)...,
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
		actions = append(missingAuthActions(provider), actions...)
	}

	return actions
}

func embeddingModelErrorActions(provider string, err error) []Action {
	actions := []Action{
		commandAction("/providers", "review known providers in the TUI"),
		commandAction("/models", "review models for the selected provider"),
	}
	if isMissingAuthError(err) {
		actions = append(providerAPIKeyActions(provider), actions...)
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

func missingAuthActions(provider string) []Action {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil
	}
	switch provider {
	case constants.ModelProviderOpenAI, constants.ModelProviderOpenAICodex,
		constants.ModelProviderAnthropic, constants.ModelProviderGitHubCopilot:
		return []Action{commandAction("hand auth login "+provider, "store OAuth credentials for this provider")}
	default:
		return providerAPIKeyActions(provider)
	}
}

func providerAPIKeyActions(provider string) []Action {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil
	}

	return []Action{
		commandAction(
			fmt.Sprintf("hand auth login %s --api-key <api-key>", provider),
			"store a provider API key",
		),
		commandAction(
			fmt.Sprintf("hand config set models.providers.%s.apiKey <api-key>", provider),
			"write the provider API key to the profile config",
		),
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

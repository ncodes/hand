package readiness

import (
	"context"
	"fmt"
	"strings"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	provider_ollama "github.com/wandxy/morph/internal/model/provider_ollama"
)

var discoverOllamaModels = func(ctx context.Context, baseURL string) ([]modelprovider.ModelDefinition, error) {
	discoverer, err := provider_ollama.NewDiscoverer(baseURL)
	if err != nil {
		return nil, err
	}

	return discoverer.DiscoverModels(ctx)
}

func buildModelGroup(ctx context.Context, cfg *config.Config) Group {
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
	checks = append(checks, buildOllamaReadinessChecks(ctx, cfg)...)

	return Group{Name: "models", Checks: checks}
}

func buildOllamaReadinessChecks(ctx context.Context, cfg *config.Config) []Check {
	if cfg == nil || strings.TrimSpace(strings.ToLower(cfg.Models.Main.Provider)) != constants.ModelProviderOllama {
		return nil
	}

	baseURL := strings.TrimSpace(cfg.Models.Main.BaseURL)
	modelID := strings.TrimSpace(cfg.Models.Main.Name)
	models, err := discoverOllamaModels(ctx, baseURL)
	if err != nil {
		return []Check{check(
			"ollama",
			StatusFail,
			err.Error(),
			commandAction("ollama serve", "start Ollama"),
			commandAction("morph setup provider --provider ollama --base-url "+baseURL, "update Ollama base URL"),
		)}
	}

	checks := []Check{check(
		"ollama",
		StatusPass,
		fmt.Sprintf("reachable at %s, discovered %d model(s)", baseURL, len(models)),
	)}
	selected, ok := getOllamaReadinessModel(models, modelID)
	if !ok {
		return append(checks, check(
			"ollama model",
			StatusFail,
			fmt.Sprintf("model %q is not installed", modelID),
			commandAction(ollamaSetupPullCommand(baseURL, modelID), "pull the selected Ollama model"),
		))
	}

	checks = append(checks, check(
		"ollama model",
		StatusPass,
		fmt.Sprintf("model %q is installed", modelID),
	))
	checks = append(checks, buildOllamaContextCheck(cfg, selected))
	checks = append(checks, buildOllamaToolSupportCheck(selected))

	return checks
}

func getOllamaReadinessModel(models []modelprovider.ModelDefinition, modelID string) (modelprovider.ModelDefinition, bool) {
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model.ID), modelID) {
			return model, true
		}
	}

	return modelprovider.ModelDefinition{}, false
}

func buildOllamaContextCheck(cfg *config.Config, model modelprovider.ModelDefinition) Check {
	modelID := strings.TrimSpace(model.ID)
	if model.ContextWindow <= 0 {
		return check("ollama context", StatusWarn, fmt.Sprintf("context metadata is unavailable for model %q", modelID))
	}

	configured := cfg.Models.Main.ContextLength
	if configured > model.ContextWindow {
		return check(
			"ollama context",
			StatusWarn,
			fmt.Sprintf("configured contextLength=%d exceeds model %q reported context window=%d", configured, modelID, model.ContextWindow),
			commandAction(
				fmt.Sprintf("morph config set models.main.contextLength %d", model.ContextWindow),
				"fit configured context to the selected model",
			),
		)
	}

	return check(
		"ollama context",
		StatusPass,
		fmt.Sprintf("model %q reports context window=%d, configured contextLength=%d", modelID, model.ContextWindow, configured),
	)
}

func buildOllamaToolSupportCheck(model modelprovider.ModelDefinition) Check {
	modelID := strings.TrimSpace(model.ID)
	if model.SupportsTools {
		return check("ollama tools", StatusPass, fmt.Sprintf("model %q reports tool support", modelID))
	}

	return check(
		"ollama tools",
		StatusWarn,
		fmt.Sprintf("model %q does not report tool support; tool-using workflows may fail", modelID),
	)
}

func ollamaSetupPullCommand(baseURL string, modelID string) string {
	parts := []string{"morph setup provider --provider ollama"}
	if strings.TrimSpace(baseURL) != "" {
		parts = append(parts, "--base-url "+strings.TrimSpace(baseURL))
	}
	if strings.TrimSpace(modelID) != "" {
		parts = append(parts, "--model "+strings.TrimSpace(modelID))
	}
	parts = append(parts, "--pull")

	return strings.Join(parts, " ")
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
		strings.Contains(message, "morph auth login")
}

func missingAuthActions(provider string) []Action {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil
	}
	switch provider {
	case constants.ModelProviderOpenAI, constants.ModelProviderOpenAICodex,
		constants.ModelProviderAnthropic, constants.ModelProviderGitHubCopilot:
		return []Action{commandAction("morph auth login "+provider, "store OAuth credentials for this provider")}
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
			fmt.Sprintf("morph auth login %s --api-key <api-key>", provider),
			"store a provider API key",
		),
		commandAction(
			fmt.Sprintf("morph config set models.providers.%s.apiKey <api-key>", provider),
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

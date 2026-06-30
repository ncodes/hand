package readiness

import (
	"context"
	"fmt"
	"strings"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	provider_ollama "github.com/wandxy/morph/internal/model/provider_ollama"
	"github.com/wandxy/morph/pkg/stringx"
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
	if cfg == nil {
		return nil
	}

	mainIsOllama := stringx.String(cfg.Models.Main.Provider).Normalized() == constants.ModelProviderOllama
	embeddingIsOllama := cfg.Search.Vector.Enabled &&
		stringx.String(cfg.ModelEmbeddingProviderEffective()).Normalized() == constants.ModelProviderOllama
	if !mainIsOllama && !embeddingIsOllama {
		return nil
	}

	baseURL := getOllamaReadinessBaseURL(cfg)
	modelID := stringx.String(cfg.Models.Main.Name).Trim()
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
	if !mainIsOllama {
		checks = append(checks, buildOllamaEmbeddingCheck(cfg, models)...)
		return checks
	}

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
	checks = append(checks, buildOllamaEmbeddingCheck(cfg, models)...)

	return checks
}

func getOllamaReadinessBaseURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if stringx.String(cfg.Models.Main.Provider).Normalized() == constants.ModelProviderOllama {
		if value := stringx.String(cfg.Models.Main.BaseURL).Trim(); value != "" {
			return value
		}
	}
	if auth, err := cfg.ResolveEmbeddingModelAuth(); err == nil &&
		stringx.String(auth.Provider).Normalized() == constants.ModelProviderOllama {
		return stringx.String(auth.BaseURL).Trim()
	}
	if value := stringx.String(cfg.Models.Embedding.BaseURL).Trim(); value != "" {
		return value
	}
	if providerConfig, ok := cfg.Models.Providers[constants.ModelProviderOllama]; ok {
		if value := stringx.String(providerConfig.BaseURL).Trim(); value != "" {
			return value
		}
	}

	return constants.DefaultOllamaBaseURL
}

func getOllamaReadinessModel(models []modelprovider.ModelDefinition, modelID string) (modelprovider.ModelDefinition, bool) {
	modelID = stringx.String(modelID).Trim()
	for _, model := range models {
		if provider_ollama.ModelIDMatches(model.ID, modelID) {
			return model, true
		}
	}

	return modelprovider.ModelDefinition{}, false
}

func buildOllamaContextCheck(cfg *config.Config, model modelprovider.ModelDefinition) Check {
	modelID := stringx.String(model.ID).Trim()
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
	modelID := stringx.String(model.ID).Trim()
	if model.SupportsTools {
		return check("ollama tools", StatusPass, fmt.Sprintf("model %q reports tool support", modelID))
	}

	return check(
		"ollama tools",
		StatusWarn,
		fmt.Sprintf("model %q does not report tool support; tool-using workflows may fail", modelID),
	)
}

func buildOllamaEmbeddingCheck(cfg *config.Config, models []modelprovider.ModelDefinition) []Check {
	if cfg == nil ||
		!cfg.Search.Vector.Enabled ||
		stringx.String(cfg.ModelEmbeddingProviderEffective()).Normalized() != constants.ModelProviderOllama {
		return nil
	}

	modelID := stringx.String(cfg.Models.Embedding.Name).Trim()
	if modelID == "" {
		return []Check{check("ollama embeddings", StatusFail, "embedding model is required")}
	}
	if _, ok := getOllamaReadinessModel(models, modelID); ok {
		return []Check{check(
			"ollama embeddings",
			StatusPass,
			fmt.Sprintf("embedding model %q is installed", modelID),
		)}
	}

	return []Check{check(
		"ollama embeddings",
		StatusWarn,
		fmt.Sprintf("embedding model %q is not installed; vector search will fail until it is pulled", modelID),
		commandAction(ollamaSetupPullCommand(getOllamaReadinessBaseURL(cfg), modelID), "pull the selected Ollama embedding model"),
	)}
}

func ollamaSetupPullCommand(baseURL string, modelID string) string {
	parts := []string{"morph setup provider --provider ollama"}
	if stringx.String(baseURL).Trim() != "" {
		parts = append(parts, "--base-url "+stringx.String(baseURL).Trim())
	}
	if stringx.String(modelID).Trim() != "" {
		parts = append(parts, "--model "+stringx.String(modelID).Trim())
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

	message := stringx.String(err.Error()).Normalized()
	return strings.Contains(message, "api key is required") ||
		strings.Contains(message, "morph auth login")
}

func missingAuthActions(provider string) []Action {
	provider = stringx.String(provider).Normalized()
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
	provider = stringx.String(provider).Normalized()
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
		if stringx.String(source.Type).Trim() != "" {
			return stringx.String(source.Type).Trim() + " env"
		}
		return "environment"
	case config.ModelCredentialSourceTokenStore:
		parts := []string{"token-store"}
		if stringx.String(source.Type).Trim() != "" {
			parts = append(parts, stringx.String(source.Type).Trim())
		}
		if source.HasExpiry {
			parts = append(parts, "refreshable")
		}
		return strings.Join(parts, " ")
	default:
		return auth.AuthType()
	}
}

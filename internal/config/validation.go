package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is required")
	}

	requestedContextLength := c.Models.Main.ContextLength
	if err := c.validatePersonalityNames(); err != nil {
		return err
	}

	c.Normalize()
	applyRegistryModelMetadata(c, requestedContextLength)

	if err := c.validatePersonalities(); err != nil {
		return err
	}

	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required; set HAND_NAME, provide it in config, or use --name")
	}

	if !isValidModelID(c.Models.Main.Name) {
		return errors.New("model is required")
	}

	if c.Models.Summary.Name != "" && !isValidModelID(c.Models.Summary.Name) {
		return errors.New("summary model is invalid")
	}

	if !hasModelProvider(c.Models.Main.Provider) {
		return fmt.Errorf("model provider must be one of: %s", getModelProviderList())
	}

	if c.Models.Summary.Provider != "" {
		if !hasModelProvider(c.Models.Summary.Provider) {
			return fmt.Errorf("summary model provider must be one of: %s", getModelProviderList())
		}
	}

	if err := c.validateModelSettings(); err != nil {
		return err
	}

	if err := c.validateRerankerSettings(); err != nil {
		return err
	}

	if err := c.validateSearchVectorSettings(); err != nil {
		return err
	}

	if _, err := c.ResolveModelAuth(); err != nil {
		return err
	}

	if _, err := c.ResolveSummaryModelAuth(); err != nil {
		return err
	}

	if strings.TrimSpace(c.RPC.Address) == "" {
		return errors.New("rpc address is required; set HAND_RPC_ADDRESS, provide it in config, or use --rpc.address")
	}

	if c.RPC.Port < 0 {
		return errors.New("rpc port must be non-negative; set HAND_RPC_PORT, provide it in config, or use --rpc.port")
	}

	if c.Session.MaxIterations <= 0 {
		return errors.New("max iterations must be greater than zero; set HAND_SESSION_MAX_ITERATIONS, provide it in config, " +
			"or use --max-iterations")
	}
	if c.ModelMaxRetriesEffective() < 0 {
		return errors.New("model max retries must be greater than or equal to zero; use --model.max-retries")
	}

	if c.Storage.Backend != "memory" && c.Storage.Backend != "sqlite" {
		return errors.New("storage backend must be one of: memory, sqlite")
	}
	if c.Memory.Backend != "" && c.Memory.Backend != "memory" && c.Memory.Backend != "sqlite" {
		return errors.New("memory backend must be one of: memory, sqlite")
	}
	if c.Compaction.TriggerPercent >= 1 {
		return errors.New("compaction trigger percent must be greater than zero and less than one")
	}
	if c.Compaction.WarnPercent >= 1 {
		return errors.New("compaction warn percent must be greater than zero and less than one")
	}
	if c.Compaction.WarnPercent < c.Compaction.TriggerPercent {
		return errors.New("compaction warn percent must be greater than or equal to compaction trigger percent")
	}
	if c.Compaction.RecentSessionTail != nil && *c.Compaction.RecentSessionTail < 0 {
		return errors.New("compaction recent session tail must be greater than or equal to zero")
	}

	switch strings.TrimSpace(strings.ToLower(c.Log.Level)) {
	case "", "debug", "info", "warn", "error":
		return nil
	default:
		return errors.New("log level must be one of debug, info, warn, or error; use --log.level")
	}
}

func (c *Config) validateModelSettings() error {
	if err := validateModelRoleAPI("model API", c.MainModelAPIEffective(), modelGenerationAPIs()); err != nil {
		return err
	}
	if err := validateProviderAPI("model API", c.Models.Main.Provider, c.MainModelAPIEffective()); err != nil {
		return err
	}
	if err := validateRegistryModel(
		"models.main.name",
		c.Models.Main.Provider,
		c.MainModelAPIEffective(),
		c.Models.Main.Name,
		modelGenerationAPIs(),
	); err != nil {
		return err
	}

	summaryProvider := c.SummaryProviderEffective()
	summaryAPI := c.SummaryModelAPIEffective()
	if err := validateModelRoleAPI("summary model API", summaryAPI, modelGenerationAPIs()); err != nil {
		return err
	}
	if err := validateProviderAPI("summary model API", summaryProvider, summaryAPI); err != nil {
		return err
	}
	if err := validateRegistryModel(
		"models.summary.name",
		summaryProvider,
		summaryAPI,
		c.SummaryModelEffective(),
		modelGenerationAPIs(),
	); err != nil {
		return err
	}

	return nil
}

func (c *Config) validatePersonalityNames() error {
	if c == nil || len(c.Personalities) == 0 {
		return nil
	}

	seen := make(map[string]string, len(c.Personalities))
	names := make([]string, 0, len(c.Personalities))
	for name := range c.Personalities {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if !validPersonalityName.MatchString(trimmed) {
			return fmt.Errorf("invalid personality name %q: must match %s", trimmed, personalityNamePattern)
		}

		normalized := strings.ToLower(trimmed)
		if existing, ok := seen[normalized]; ok {
			return fmt.Errorf("duplicate personality name %q conflicts with %q", trimmed, existing)
		}
		seen[normalized] = trimmed
	}

	return nil
}

func (c *Config) validatePersonalities() error {
	if c == nil {
		return nil
	}

	for name, personality := range c.Personalities {
		if err := validatePersonalityConfig(name, personality); err != nil {
			return err
		}
	}

	return nil
}

func validatePersonalityConfig(name string, personality PersonalityConfig) error {
	switch personality.State {
	case personalityStateShared, personalityStateIsolated, personalityStateReadonly:
	default:
		return fmt.Errorf("personalities.%s.state must be one of: shared, isolated, readonly", name)
	}

	switch personality.Tools.Memory {
	case "", personalityToolMemoryNone, personalityToolMemoryRead, personalityToolMemoryWrite:
	default:
		return fmt.Errorf("personalities.%s.tools.mem must be one of: none, read, write", name)
	}

	if personality.MaxIterations < 0 {
		return fmt.Errorf("personalities.%s.maxIterations must be non-negative", name)
	}

	if personality.Model.Name != "" && !isValidModelID(personality.Model.Name) {
		return fmt.Errorf("personalities.%s.model.name is invalid", name)
	}
	if personality.Model.Provider != "" {
		if !hasModelProvider(personality.Model.Provider) {
			return fmt.Errorf("personalities.%s.model.provider must be one of: %s", name, getModelProviderList())
		}
	}
	switch personality.Model.API {
	case "", modelprovider.APIOpenAICompletions, modelprovider.APIOpenAIResponses, modelprovider.APIAnthropicMessages:
	default:
		return fmt.Errorf("personalities.%s.model.api must be one of: %s", name, getModelAPIList(modelGenerationAPIs()))
	}

	return nil
}

func (c *Config) validateSearchVectorSettings() error {
	if !c.Search.Vector.Enabled {
		return nil
	}
	provider := c.ModelEmbeddingProviderEffective()
	if !hasModelProvider(provider) {
		return fmt.Errorf("embedding provider must be one of: %s", getModelProviderList())
	}
	if c.Models.Embedding.Name == "" {
		return errors.New("embedding model is required")
	}
	if c.Search.Vector.RebuildBatchSize < 0 {
		return errors.New("vector rebuild batch size must be non-negative")
	}
	auth, err := c.ResolveEmbeddingModelAuth()
	if err != nil {
		return err
	}
	if err := validateProviderAPI("embedding model API", auth.Provider, auth.API); err != nil {
		return err
	}
	if err := validateModelRoleAPI("embedding model API", auth.API, modelEmbeddingAPIs()); err != nil {
		return err
	}
	if err := validateRegistryModel(
		"models.embedding.name",
		auth.Provider,
		auth.API,
		c.Models.Embedding.Name,
		modelEmbeddingAPIs(),
	); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateRerankerSettings() error {
	if err := validateRerankerType(c.RerankerEffective()); err != nil {
		return err
	}
	if c.Reranker.MaxCandidates < 0 {
		return errors.New("reranker max candidates must be non-negative")
	}
	if c.Reranker.MaxCandidateTextChars < 0 {
		return errors.New("reranker max candidate text chars must be non-negative")
	}
	if c.Reranker.MaxOutputTokens < 0 {
		return errors.New("reranker max output tokens must be non-negative")
	}
	for useCase, override := range c.Reranker.Overrides {
		if err := c.validateRerankerOverride(useCase, override); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) validateRerankerOverride(useCase string, override RerankerOverrideConfig) error {
	useCase = strings.TrimSpace(useCase)
	if useCase == "" {
		return errors.New("reranker override use case is required")
	}
	if strings.TrimSpace(override.Type) != "" {
		if err := validateRerankerType(override.Type); err != nil {
			return fmt.Errorf("reranker override %q: %w", useCase, err)
		}
	}
	if override.MaxCandidates != nil && *override.MaxCandidates < 0 {
		return fmt.Errorf("reranker override %q max candidates must be non-negative", useCase)
	}
	if override.MaxCandidateTextChars != nil && *override.MaxCandidateTextChars < 0 {
		return fmt.Errorf("reranker override %q max candidate text chars must be non-negative", useCase)
	}
	if override.MaxOutputTokens != nil && *override.MaxOutputTokens < 0 {
		return fmt.Errorf("reranker override %q max output tokens must be non-negative", useCase)
	}

	return nil
}

func validateRerankerType(rerankerType string) error {
	switch strings.TrimSpace(strings.ToLower(rerankerType)) {
	case constants.RerankerDeterministic, constants.RerankerNoop, constants.RerankerLLM:
		return nil
	default:
		return errors.New("reranker type must be one of: deterministic, noop, llm")
	}
}

package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func getDefaultBaseURLForProvider(provider, apiID string) string {
	if strings.TrimSpace(apiID) == "" {
		return modelRegistry.GetBaseURL(provider, "")
	}

	api, ok := getModelAPI(apiID)
	if !ok {
		return ""
	}

	return modelRegistry.GetBaseURL(provider, api.ID)
}

func getDefaultAPIForProvider(provider string) string {
	definition, ok := modelRegistry.GetProvider(provider)
	if !ok {
		return ""
	}

	return definition.DefaultAPI
}

func getModelAPI(apiID string) (modelprovider.APIDefinition, bool) {
	apiID = strings.TrimSpace(strings.ToLower(apiID))
	if apiID == "" {
		return modelprovider.APIDefinition{}, false
	}

	return modelRegistry.GetAPI(apiID)
}

func getModelAPIID(apiID string) string {
	api, ok := getModelAPI(apiID)
	if !ok {
		return ""
	}

	return api.ID
}

func hasGenerationAPI(apiID string) bool {
	api, ok := getModelAPI(apiID)
	if !ok {
		return false
	}

	return api.ID == modelprovider.APIOpenAICompletions || api.ID == modelprovider.APIOpenAIResponses
}

func hasModelProvider(provider string) bool {
	_, ok := modelRegistry.GetProvider(provider)
	return ok
}

func getModelProviderList() string {
	ids := modelRegistry.GetProviderIDs()
	sort.Strings(ids)

	return strings.Join(ids, ", ")
}

func (c *Config) VerifyEnabled() bool {
	if c == nil {
		return true
	}

	return getBoolValueDefault(c.Models.Verify, true)
}

func (c *Config) StreamEnabled() bool {
	if c == nil {
		return true
	}

	return getBoolValueDefault(c.Models.Main.Stream, true)
}

func (c *Config) InputSafetyEnabled() bool {
	if c == nil {
		return constants.DefaultSafetyInputEnabled
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Safety.Input, constants.DefaultSafetyInputEnabled)
}

func (c *Config) OutputSafetyEnabled() bool {
	if c == nil {
		return constants.DefaultSafetyOutputEnabled
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Safety.Output, constants.DefaultSafetyOutputEnabled)
}

func (c *Config) OutputPIIRedactionEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Safety.PII, constants.DefaultSafetyPIIEnabled)
}

func (c *Config) TUIThinkingComposerEnabled() bool {
	if c == nil {
		return constants.DefaultTUIThinkingComposerEnabled
	}

	c.normalizeFields()
	return getBoolValueDefault(c.TUI.ThinkingComposer, constants.DefaultTUIThinkingComposerEnabled)
}

func (c *Config) ModelMaxRetriesEffective() int {
	if c == nil {
		return constants.DefaultModelMaxRetries
	}

	c.normalizeFields()
	return *c.Models.MaxRetries
}

func (c *Config) SummaryModelEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Summary.Name != "" {
		return c.Models.Summary.Name
	}

	return c.Models.Main.Name
}

func (c *Config) SummaryProviderEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Summary.Provider != "" {
		return c.Models.Summary.Provider
	}

	return c.Models.Main.Provider
}

// MainModelAPIEffective returns the registry API ID for the main model.
func (c *Config) MainModelAPIEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	return getModelAPIID(c.Models.Main.API)
}

// SummaryModelAPIEffective returns the registry API ID for the summary model.
func (c *Config) SummaryModelAPIEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Summary.API != "" {
		return getModelAPIID(c.Models.Summary.API)
	}

	return getModelAPIID(c.Models.Main.API)
}

func (c *Config) RerankerEffective() string {
	if c == nil {
		return constants.RerankerDeterministic
	}

	c.normalizeFields()
	if c.Reranker.Type != "" {
		return c.Reranker.Type
	}

	return constants.RerankerDeterministic
}

func (c *Config) MemoryEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Enabled, true)
}

func (c *Config) MemoryRetrievalEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Retrieval.Enabled, true)
}

func (c *Config) MemoryFlushEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Flush.Enabled, true)
}

func (c *Config) MemoryWriteEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Write.Enabled, true)
}

func (c *Config) RerankerModelEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Reranker.Model != "" {
		return c.Reranker.Model
	}

	return c.SummaryModelEffective()
}

func (c *Config) RerankerOverrideEffective(override RerankerOverrideConfig) RerankerEffectiveConfig {
	if c == nil {
		return RerankerEffectiveConfig{}
	}

	c.normalizeFields()

	rerankerType := strings.TrimSpace(strings.ToLower(override.Type))
	if rerankerType == "" {
		rerankerType = c.RerankerEffective()
	}

	model := strings.TrimSpace(override.Model)
	if model == "" {
		model = c.RerankerModelEffective()
	}

	maxCandidates := c.Reranker.MaxCandidates
	maxCandidatesSet := maxCandidates != 0
	if override.MaxCandidates != nil {
		maxCandidates = *override.MaxCandidates
		maxCandidatesSet = true
	}

	maxCandidateTextChars := c.Reranker.MaxCandidateTextChars
	maxCandidateTextCharsSet := maxCandidateTextChars != 0
	if override.MaxCandidateTextChars != nil {
		maxCandidateTextChars = *override.MaxCandidateTextChars
		maxCandidateTextCharsSet = true
	}

	maxOutputTokens := c.Reranker.MaxOutputTokens
	if override.MaxOutputTokens != nil {
		maxOutputTokens = *override.MaxOutputTokens
	}

	return RerankerEffectiveConfig{
		Type:                     rerankerType,
		Model:                    model,
		MaxCandidates:            maxCandidates,
		MaxCandidatesSet:         maxCandidatesSet,
		MaxCandidateTextChars:    maxCandidateTextChars,
		MaxCandidateTextCharsSet: maxCandidateTextCharsSet,
		MaxOutputTokens:          maxOutputTokens,
	}
}

func (c *Config) summaryModelBaseURLEffective() string {
	main := c.Models.Main.Provider
	sum := c.SummaryProviderEffective()
	sumAPI := c.SummaryModelAPIEffective()
	mainAPI := c.MainModelAPIEffective()

	if sum == main && sumAPI == mainAPI {
		return c.Models.Main.BaseURL
	}

	if u := strings.TrimSpace(c.Models.Summary.BaseURL); u != "" {
		return u
	}

	return getDefaultBaseURLForProvider(sum, sumAPI)
}

func (c *Config) ResolveSummaryModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	prov := c.SummaryProviderEffective()
	auth := ModelAuth{
		Provider: prov,
		API:      getModelAPIID(c.SummaryModelAPIEffective()),
		BaseURL:  c.summaryModelBaseURLEffective(),
	}

	auth.APIKey = c.resolveAPIKeyForProvider(prov)
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

// ModelAuthEqual reports whether two auth values describe the same provider, API, endpoint, and key.
func ModelAuthEqual(a, b ModelAuth) bool {
	return strings.TrimSpace(strings.ToLower(a.Provider)) == strings.TrimSpace(strings.ToLower(b.Provider)) &&
		strings.TrimSpace(strings.ToLower(a.API)) == strings.TrimSpace(strings.ToLower(b.API)) &&
		strings.TrimSpace(a.BaseURL) == strings.TrimSpace(b.BaseURL) &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey)
}

func (c *Config) ResolveEmbeddingModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	provider := c.ModelEmbeddingProviderEffective()
	if !hasModelProvider(provider) {
		return ModelAuth{}, fmt.Errorf("embedding provider must be one of: %s", getModelProviderList())
	}

	auth := ModelAuth{
		Provider: provider,
		API:      modelprovider.APIOpenAIEmbeddings,
		BaseURL:  getDefaultBaseURLForProvider(provider, modelprovider.APIOpenAIEmbeddings),
		APIKey:   c.resolveAPIKeyForProvider(provider),
	}
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("embedding API key is required")
	}

	return auth, nil
}

func (c *Config) ModelEmbeddingProviderEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Embedding.Provider != "" {
		return c.Models.Embedding.Provider
	}

	return c.Models.Main.Provider
}

func (c *Config) ResolveModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	auth := ModelAuth{
		Provider: c.Models.Main.Provider,
		API:      getModelAPIID(c.Models.Main.API),
		BaseURL:  c.Models.Main.BaseURL,
	}

	auth.APIKey = c.resolveAPIKeyForProvider(c.Models.Main.Provider)
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

func (c *Config) resolveAPIKeyForProvider(provider string) string {
	switch provider {
	case "openrouter":
		return getFirstNonEmpty(c.Models.OpenRouterAPIKey, c.Models.Key)
	case "openai":
		return getFirstNonEmpty(c.Models.OpenAIAPIKey, c.Models.Key)
	default:
		return c.Models.Key
	}
}

func getFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

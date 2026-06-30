package config

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wandxy/morph/internal/constants"
	appcredential "github.com/wandxy/morph/internal/credential"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	"github.com/wandxy/morph/pkg/stringx"
)

func getDefaultBaseURLForProvider(provider, apiID string) string {
	if stringx.String(apiID).Trim() == "" {
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
	apiID = stringx.String(apiID).Normalized()
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

func (c *Config) getProviderConfig(provider string) ProviderModelConfig {
	if c == nil {
		return ProviderModelConfig{}
	}

	provider = stringx.String(provider).Normalized()
	if provider == "" || len(c.Models.Providers) == 0 {
		return ProviderModelConfig{}
	}

	return c.Models.Providers[provider]
}

func (c *Config) getProviderAPIConfig(provider string) string {
	return stringx.String(c.getProviderConfig(provider).API).Normalized()
}

func (c *Config) getProviderBaseURLConfig(provider string) string {
	return stringx.String(c.getProviderConfig(provider).BaseURL).Trim()
}

func (c *Config) getProviderHeadersConfig(provider string) map[string]string {
	return normalizeStringMap(c.getProviderConfig(provider).Headers)
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

func getModelAPIList(allowed map[string]struct{}) string {
	ids := modelRegistry.GetAPIIDs()
	if len(allowed) != 0 {
		ids = ids[:0]
		for id := range allowed {
			if _, ok := modelRegistry.GetAPI(id); ok {
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)

	return strings.Join(ids, ", ")
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
		return constants.DefaultSafetyPIIEnabled
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
	if provider := c.SummaryProviderEffective(); provider != "" && provider != c.Models.Main.Provider {
		if api := c.getProviderAPIConfig(provider); api != "" {
			return getModelAPIID(api)
		}
		return getModelAPIID(getDefaultAPIForProvider(provider))
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

func (c *Config) MemoryBackendEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Memory.Backend != "" {
		return c.Memory.Backend
	}

	return c.Storage.Backend
}

func (c *Config) MemoryPinnedEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Pinned.Enabled, constants.DefaultProfileMemoryPinnedEnabled)
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

func (c *Config) MemoryEpisodicEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Episodic.Enabled, constants.DefaultMemoryEpisodicEnabled)
}

func (c *Config) MemoryReflectionEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Reflection.Enabled, constants.DefaultMemoryReflectionEnabled)
}

func (c *Config) MemoryPromotionEnabled() bool {
	if c == nil {
		return false
	}

	c.normalizeFields()
	return getBoolValueDefault(c.Memory.Promotion.Enabled, constants.DefaultProfileMemoryPromotionEnabled)
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

func (c *Config) RerankerProviderEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	return c.SummaryProviderEffective()
}

func (c *Config) RerankerModelAPIEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	return c.RerankerModelAPIEffectiveForModel(c.RerankerModelEffective())
}

func (c *Config) RerankerModelAPIEffectiveForModel(modelID string) string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	provider := c.RerankerProviderEffective()
	if model, ok := modelRegistry.GetModel(provider, modelID); ok {
		return getModelAPIID(model.API)
	}

	return c.SummaryModelAPIEffective()
}

func (c *Config) RerankerOverrideEffective(override RerankerOverrideConfig) RerankerEffectiveConfig {
	if c == nil {
		return RerankerEffectiveConfig{}
	}

	c.normalizeFields()

	rerankerType := stringx.String(override.Type).Normalized()
	if rerankerType == "" {
		rerankerType = c.RerankerEffective()
	}

	model := stringx.String(override.Model).Trim()
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

	if u := stringx.String(c.Models.Summary.BaseURL).Trim(); u != "" {
		return u
	}
	if u := c.getProviderBaseURLConfig(sum); u != "" {
		return u
	}

	return getDefaultBaseURLForProvider(sum, sumAPI)
}

func (c *Config) summaryAPIKeyEffective() string {
	if key := stringx.String(c.Models.Summary.APIKey).Trim(); key != "" {
		return key
	}

	if c.SummaryProviderEffective() == c.Models.Main.Provider &&
		c.SummaryModelAPIEffective() == c.MainModelAPIEffective() {
		return c.Models.Main.APIKey
	}

	return ""
}

func (c *Config) rerankerModelBaseURLEffective() string {
	provider := c.RerankerProviderEffective()
	api := c.RerankerModelAPIEffective()

	if provider == c.SummaryProviderEffective() && api == c.SummaryModelAPIEffective() {
		return c.summaryModelBaseURLEffective()
	}
	if u := c.getProviderBaseURLConfig(provider); u != "" {
		return u
	}

	return getDefaultBaseURLForProvider(provider, api)
}

func (c *Config) rerankerAPIKeyEffective() string {
	if c.RerankerProviderEffective() == c.SummaryProviderEffective() &&
		c.RerankerModelAPIEffective() == c.SummaryModelAPIEffective() {
		return c.summaryAPIKeyEffective()
	}

	return ""
}

func (c *Config) ResolveSummaryModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	auth := ModelAuth{
		Provider: c.SummaryProviderEffective(),
		API:      getModelAPIID(c.SummaryModelAPIEffective()),
		BaseURL:  c.summaryModelBaseURLEffective(),
	}

	credential, err := c.resolveCredentialForProvider(
		auth.Provider,
		c.summaryAPIKeyEffective(),
		true,
		"summary model",
		c.SummaryModelEffective(),
	)
	if err != nil {
		return ModelAuth{}, err
	}

	auth.APIKey = credential.Value
	auth.Headers = mergeModelAuthHeaders(c.getProviderHeadersConfig(auth.Provider), credential.Headers)
	auth.CredentialSource = credential.Source
	auth.applySubscriptionDefaults()
	if stringx.String(auth.APIKey).Trim() == "" {
		return ModelAuth{}, newMissingModelCredentialError("model", auth.Provider)
	}

	return auth, nil
}

func (c *Config) ResolveRerankerModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	auth := ModelAuth{
		Provider: c.RerankerProviderEffective(),
		API:      c.RerankerModelAPIEffective(),
		BaseURL:  c.rerankerModelBaseURLEffective(),
	}

	credential, err := c.resolveCredentialForProvider(
		auth.Provider,
		c.rerankerAPIKeyEffective(),
		true,
		"reranker model",
		c.RerankerModelEffective(),
	)
	if err != nil {
		return ModelAuth{}, err
	}
	auth.APIKey = credential.Value
	auth.Headers = mergeModelAuthHeaders(c.getProviderHeadersConfig(auth.Provider), credential.Headers)
	auth.CredentialSource = credential.Source
	auth.applySubscriptionDefaults()
	if stringx.String(auth.APIKey).Trim() == "" {
		return ModelAuth{}, newMissingModelCredentialError("model", auth.Provider)
	}

	return auth, nil
}

// ModelAuthEqual reports whether two auth values describe the same provider, API, endpoint, and key.
func ModelAuthEqual(a, b ModelAuth) bool {
	return stringx.String(a.Provider).Normalized() == stringx.String(b.Provider).Normalized() &&
		stringx.String(a.API).Normalized() == stringx.String(b.API).Normalized() &&
		stringx.String(a.BaseURL).Trim() == stringx.String(b.BaseURL).Trim() &&
		stringx.String(a.APIKey).Trim() == stringx.String(b.APIKey).Trim() &&
		stringMapsEqual(a.Headers, b.Headers)
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
	api := c.EmbeddingModelAPIEffective()
	baseURL := stringx.String(c.Models.Embedding.BaseURL).Trim()
	if baseURL == "" {
		baseURL = c.getEmbeddingProviderRoleBaseURL(provider)
	}
	if baseURL == "" {
		baseURL = c.getProviderBaseURLConfig(provider)
	}
	if baseURL == "" {
		baseURL = getDefaultBaseURLForProvider(provider, api)
	}

	auth := ModelAuth{
		Provider: provider,
		API:      api,
		BaseURL:  baseURL,
	}
	credential, err := c.resolveCredentialForProvider(provider, c.Models.Embedding.APIKey, false, "", "")
	if err != nil {
		return ModelAuth{}, err
	}
	auth.APIKey = credential.Value
	auth.Headers = mergeModelAuthHeaders(c.getProviderHeadersConfig(auth.Provider), credential.Headers)
	auth.CredentialSource = credential.Source
	if stringx.String(auth.APIKey).Trim() == "" {
		return ModelAuth{}, newMissingModelCredentialError("embedding", auth.Provider)
	}

	return auth, nil
}

func (c *Config) EmbeddingModelAPIEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Embedding.API != "" {
		return getModelAPIID(c.Models.Embedding.API)
	}

	provider := c.ModelEmbeddingProviderEffective()
	if api := c.getProviderAPIConfig(provider); api != "" {
		if _, ok := modelEmbeddingAPIs()[api]; ok {
			return getModelAPIID(api)
		}
	}
	if model, ok := modelRegistry.GetModel(provider, c.Models.Embedding.Name); ok {
		if _, ok := modelEmbeddingAPIs()[model.API]; ok {
			return getModelAPIID(model.API)
		}
	}

	if modelRegistry.SupportsProviderAPI(provider, modelprovider.APIOpenRouterEmbeddings) {
		return modelprovider.APIOpenRouterEmbeddings
	}
	if modelRegistry.SupportsProviderAPI(provider, modelprovider.APIOpenAIEmbeddings) {
		return modelprovider.APIOpenAIEmbeddings
	}
	if modelRegistry.SupportsProviderAPI(provider, modelprovider.APIOllamaEmbeddings) {
		return modelprovider.APIOllamaEmbeddings
	}

	return ""
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

func (c *Config) getEmbeddingProviderRoleBaseURL(provider string) string {
	if c == nil || stringx.String(provider).Normalized() != constants.ModelProviderOllama {
		return ""
	}
	if !strings.EqualFold(c.Models.Main.Provider, provider) {
		return ""
	}

	return normalizeOllamaEmbeddingBaseURL(c.Models.Main.BaseURL)
}

func normalizeOllamaEmbeddingBaseURL(value string) string {
	value = strings.TrimRight(stringx.String(value).Trim(), "/")
	if strings.HasSuffix(strings.ToLower(value), "/v1") {
		value = strings.TrimRight(value[:len(value)-len("/v1")], "/")
	}

	return value
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

	credential, err := c.resolveCredentialForProvider(
		c.Models.Main.Provider,
		c.Models.Main.APIKey,
		true,
		"model",
		c.Models.Main.Name,
	)
	if err != nil {
		return ModelAuth{}, err
	}
	auth.APIKey = credential.Value
	auth.Headers = mergeModelAuthHeaders(c.getProviderHeadersConfig(auth.Provider), credential.Headers)
	auth.CredentialSource = credential.Source
	auth.applySubscriptionDefaults()
	if stringx.String(auth.APIKey).Trim() == "" {
		return ModelAuth{}, newMissingModelCredentialError("model", auth.Provider)
	}

	return auth, nil
}

type resolvedModelCredential struct {
	Value   string
	Headers map[string]string
	Source  ModelCredentialSource
}

func (c *Config) resolveCredentialForProvider(
	provider string,
	roleAPIKey string,
	allowOAuth bool,
	oauthModelField string,
	oauthModelID string,
) (resolvedModelCredential, error) {
	provider = stringx.String(provider).Normalized()
	if value := stringx.String(roleAPIKey).Trim(); value != "" {
		return resolvedModelCredential{
			Value:  value,
			Source: ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig},
		}, nil
	}

	stored, err := loadStoredModelCredential(provider)
	if err != nil {
		return resolvedModelCredential{}, err
	}
	var oauthModelErr error
	if stringx.String(stored.Type).Normalized() == appcredential.TypeOAuth && !allowOAuth {
		stored = StoredModelCredential{}
	}
	if stringx.String(stored.Type).Normalized() == appcredential.TypeOAuth && allowOAuth {
		if err := checkOAuthModelSupported(oauthModelField, provider, oauthModelID); err != nil {
			oauthModelErr = err
			stored = StoredModelCredential{}
		}
	}
	if appcredential.IsExpired(stored) {
		refreshed, ok, err := refreshStoredModelCredential(provider)
		if err != nil {
			return resolvedModelCredential{}, err
		}
		if ok {
			stored = refreshed
		} else {
			stored = StoredModelCredential{}
		}
		if stringx.String(stored.Type).Normalized() == appcredential.TypeOAuth && allowOAuth {
			if err := checkOAuthModelSupported(oauthModelField, provider, oauthModelID); err != nil {
				oauthModelErr = err
				stored = StoredModelCredential{}
			}
		}
	}
	if value := getStoredModelCredentialValue(stored); value != "" {
		headers, err := getStoredModelCredentialHeaders(provider, stored)
		if err != nil {
			return resolvedModelCredential{}, err
		}

		return resolvedModelCredential{
			Value:   value,
			Headers: headers,
			Source: ModelCredentialSource{
				Kind:      ModelCredentialSourceTokenStore,
				Name:      provider,
				Type:      stringx.String(stored.Type).Normalized(),
				HasExpiry: stored.ExpiresAt != nil,
			},
		}, nil
	}

	if allowOAuth {
		if value, envName := getProviderOAuthEnvCredential(provider); value != "" {
			credential := StoredModelCredential{Type: appcredential.TypeOAuth, Token: value}
			if err := checkOAuthModelSupported(oauthModelField, provider, oauthModelID); err != nil {
				oauthModelErr = err
			} else {
				headers, err := getStoredModelCredentialHeaders(provider, credential)
				if err != nil {
					return resolvedModelCredential{}, err
				}

				return resolvedModelCredential{
					Value:   value,
					Headers: headers,
					Source: ModelCredentialSource{
						Kind: ModelCredentialSourceProviderEnv,
						Name: envName,
						Type: appcredential.TypeOAuth,
					},
				}, nil
			}
		}
	}

	providerConfig := c.Models.Providers[provider]
	if value, envName := getCredentialFromEnv(providerConfig.APIKeyEnv); value != "" {
		return resolvedModelCredential{
			Value:  value,
			Source: ModelCredentialSource{Kind: ModelCredentialSourceProviderEnv, Name: envName},
		}, nil
	}

	if providerDef, ok := modelRegistry.GetProvider(provider); ok {
		if value, envName := getCredentialFromEnv(providerDef.APIKeyEnv); value != "" {
			return resolvedModelCredential{
				Value:  value,
				Source: ModelCredentialSource{Kind: ModelCredentialSourceProviderEnv, Name: envName},
			}, nil
		}
	}

	if value := stringx.String(providerConfig.APIKey).Trim(); value != "" {
		return resolvedModelCredential{
			Value:  value,
			Source: ModelCredentialSource{Kind: ModelCredentialSourceProviderConfig, Name: provider},
		}, nil
	}
	if marker := getLocalProviderAuthMarker(provider); marker != "" {
		return resolvedModelCredential{
			Value: marker,
			Source: ModelCredentialSource{
				Kind: ModelCredentialSourceLocalProvider,
				Name: provider,
			},
		}, nil
	}
	if oauthModelErr != nil {
		return resolvedModelCredential{}, oauthModelErr
	}

	return resolvedModelCredential{}, nil
}

func getLocalProviderAuthMarker(provider string) string {
	providerDef, ok := modelRegistry.GetProvider(provider)
	if !ok || providerDef.Local == nil {
		return ""
	}

	if marker := stringx.String(providerDef.Local.AuthMarker).Trim(); marker != "" {
		return marker
	}

	return constants.LocalProviderAuthMarker
}

func getStoredModelCredentialHeaders(
	provider string,
	credential StoredModelCredential,
) (map[string]string, error) {
	if stringx.String(credential.Type).Normalized() != appcredential.TypeOAuth {
		return nil, nil
	}

	if getSubscriptionProvider == nil {
		return nil, nil
	}

	subscriptionProvider, ok := getSubscriptionProvider(provider)
	if !ok {
		return nil, nil
	}

	headers, err := subscriptionProvider.AuthHeaders(context.Background(), credential)
	if err != nil {
		return nil, err
	}

	return normalizeStringMap(headers), nil
}

func checkOAuthModelSupported(
	field string,
	provider string,
	modelID string,
) error {
	field = stringx.String(field).Trim()
	if field == "" {
		field = "model"
	}
	modelID = stringx.String(modelID).Trim()
	if modelID == "" {
		return nil
	}

	provider = stringx.String(provider).Normalized()
	providerDef, ok := modelRegistry.GetProvider(provider)
	if !ok {
		return nil
	}
	if !providerDef.SupportsOAuth {
		return fmt.Errorf("%s %q is not available through OAuth for provider %q", field, modelID, provider)
	}

	model, ok := modelRegistry.GetModel(provider, modelID)
	if !ok || !model.SupportsOAuth {
		return fmt.Errorf("%s %q is not available through OAuth for provider %q", field, modelID, provider)
	}

	return nil
}

func (auth *ModelAuth) applySubscriptionDefaults() {
	if auth == nil {
		return
	}
	if auth.CredentialSource.Kind != ModelCredentialSourceTokenStore ||
		auth.CredentialSource.Type != appcredential.TypeOAuth {
		return
	}
	if stringx.String(auth.Provider).Normalized() != constants.ModelProviderOpenAI &&
		stringx.String(auth.Provider).Normalized() != constants.ModelProviderOpenAICodex {
		return
	}
	if !isProviderDefaultBaseURL(auth.BaseURL) {
		return
	}

	auth.BaseURL = constants.DefaultOpenAISubscriptionBaseURL
}

// SupportsMaxOutputTokens reports whether the resolved model route can request
// an explicit provider-enforced output token cap.
func (auth ModelAuth) SupportsMaxOutputTokens() bool {
	if auth.CredentialSource.Kind != ModelCredentialSourceTokenStore ||
		auth.CredentialSource.Type != appcredential.TypeOAuth {
		return true
	}

	provider := stringx.String(auth.Provider).Normalized()
	return provider != constants.ModelProviderOpenAI &&
		provider != constants.ModelProviderOpenAICodex
}

// SummaryModelSupportsMaxOutputTokens reports whether summary-model dependent
// background jobs should request explicit output token caps.
func (c *Config) SummaryModelSupportsMaxOutputTokens() bool {
	if c == nil {
		return true
	}

	auth, err := c.ResolveSummaryModelAuth()
	if err != nil {
		return true
	}

	return auth.SupportsMaxOutputTokens()
}

// SummaryModelMaxOutputTokensEffective returns maxOutputTokens only when the
// resolved summary model route supports the parameter.
func (c *Config) SummaryModelMaxOutputTokensEffective(maxOutputTokens int64) int64 {
	if maxOutputTokens <= 0 {
		return 0
	}
	if !c.SummaryModelSupportsMaxOutputTokens() {
		return 0
	}

	return maxOutputTokens
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = stringx.String(key).Trim()
		value = stringx.String(value).Trim()
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func mergeModelAuthHeaders(values ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, headers := range values {
		for key, value := range normalizeStringMap(headers) {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return nil
	}

	return merged
}

func stringMapsEqual(a map[string]string, b map[string]string) bool {
	a = normalizeStringMap(a)
	b = normalizeStringMap(b)
	if len(a) != len(b) {
		return false
	}

	for key, value := range a {
		if b[key] != value {
			return false
		}
	}

	return true
}

func refreshStoredModelCredential(provider string) (StoredModelCredential, bool, error) {
	if refreshStoredProviderToken == nil {
		return StoredModelCredential{}, false, nil
	}

	provider = stringx.String(provider).Normalized()
	return refreshStoredProviderToken(context.Background(), provider)
}

func loadStoredModelCredential(provider string) (StoredModelCredential, error) {
	if loadStoredProviderToken == nil {
		return StoredModelCredential{}, nil
	}

	provider = stringx.String(provider).Normalized()
	credential, err := loadStoredProviderToken(provider)
	if err != nil {
		return StoredModelCredential{}, err
	}

	return credential, nil
}

func getProviderOAuthEnvCredential(provider string) (string, string) {
	switch stringx.String(provider).Normalized() {
	case constants.ModelProviderAnthropic:
		return getCredentialFromEnv([]string{"ANTHROPIC_OAUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"})
	case constants.ModelProviderGitHubCopilot:
		return getCredentialFromEnv([]string{"COPILOT_GITHUB_TOKEN"})
	default:
		return "", ""
	}
}

func getStoredModelCredentialValue(credential StoredModelCredential) string {
	switch stringx.String(credential.Type).Normalized() {
	case appcredential.TypeAPIKey:
		return stringx.String(credential.Key).Trim()
	case appcredential.TypeOAuth, "":
		return stringx.String(credential.Token).Trim()
	default:
		return ""
	}
}

func newMissingModelCredentialError(role string, provider string) error {
	role = stringx.String(role).Trim()
	if role == "" {
		role = "model"
	}
	provider = stringx.String(provider).Normalized()
	if role == "embedding" {
		if provider == "" {
			return fmt.Errorf("%s API key is required; set a provider API key, provider env var, or role apiKey", role)
		}

		return fmt.Errorf("%s API key is required for provider %q; set a provider API key, provider env var, or role apiKey",
			role,
			provider,
		)
	}
	if provider == "" {
		return fmt.Errorf("%s API key is required; set a provider API key, provider env var, role apiKey, or provider login", role)
	}

	return fmt.Errorf("%s API key is required for provider %q; set a provider API key, provider env var, role apiKey, or provider login", role, provider)
}

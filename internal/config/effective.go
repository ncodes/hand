package config

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	appcredential "github.com/wandxy/hand/internal/credential"
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

func (c *Config) summaryAPIKeyEffective() string {
	if key := strings.TrimSpace(c.Models.Summary.APIKey); key != "" {
		return key
	}

	if c.SummaryProviderEffective() == c.Models.Main.Provider &&
		c.SummaryModelAPIEffective() == c.MainModelAPIEffective() {
		return c.Models.Main.APIKey
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
	auth.Headers = credential.Headers
	auth.CredentialSource = credential.Source
	auth.applySubscriptionDefaults()
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, newMissingModelCredentialError("model", auth.Provider)
	}

	return auth, nil
}

// ModelAuthEqual reports whether two auth values describe the same provider, API, endpoint, and key.
func ModelAuthEqual(a, b ModelAuth) bool {
	return strings.TrimSpace(strings.ToLower(a.Provider)) == strings.TrimSpace(strings.ToLower(b.Provider)) &&
		strings.TrimSpace(strings.ToLower(a.API)) == strings.TrimSpace(strings.ToLower(b.API)) &&
		strings.TrimSpace(a.BaseURL) == strings.TrimSpace(b.BaseURL) &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey) &&
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

	auth := ModelAuth{
		Provider: provider,
		API:      modelprovider.APIOpenAIEmbeddings,
		BaseURL:  getDefaultBaseURLForProvider(provider, modelprovider.APIOpenAIEmbeddings),
	}
	credential, err := c.resolveCredentialForProvider(provider, c.Models.Embedding.APIKey, false, "", "")
	if err != nil {
		return ModelAuth{}, err
	}
	auth.APIKey = credential.Value
	auth.Headers = credential.Headers
	auth.CredentialSource = credential.Source
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, newMissingModelCredentialError("embedding", auth.Provider)
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
	auth.Headers = credential.Headers
	auth.CredentialSource = credential.Source
	auth.applySubscriptionDefaults()
	if strings.TrimSpace(auth.APIKey) == "" {
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
	provider = strings.TrimSpace(strings.ToLower(provider))
	if value := strings.TrimSpace(roleAPIKey); value != "" {
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
	if strings.TrimSpace(strings.ToLower(stored.Type)) == appcredential.TypeOAuth && !allowOAuth {
		stored = StoredModelCredential{}
	}
	if strings.TrimSpace(strings.ToLower(stored.Type)) == appcredential.TypeOAuth && allowOAuth {
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
		if strings.TrimSpace(strings.ToLower(stored.Type)) == appcredential.TypeOAuth && allowOAuth {
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
				Type:      strings.TrimSpace(strings.ToLower(stored.Type)),
				HasExpiry: stored.ExpiresAt != nil,
			},
		}, nil
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

	if value := strings.TrimSpace(providerConfig.APIKey); value != "" {
		return resolvedModelCredential{
			Value:  value,
			Source: ModelCredentialSource{Kind: ModelCredentialSourceProviderConfig, Name: provider},
		}, nil
	}
	if oauthModelErr != nil {
		return resolvedModelCredential{}, oauthModelErr
	}

	return resolvedModelCredential{}, nil
}

func getStoredModelCredentialHeaders(
	provider string,
	credential StoredModelCredential,
) (map[string]string, error) {
	if strings.TrimSpace(strings.ToLower(credential.Type)) != appcredential.TypeOAuth {
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
	field = strings.TrimSpace(field)
	if field == "" {
		field = "model"
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	provider = strings.TrimSpace(strings.ToLower(provider))
	providerDef, ok := modelRegistry.GetProvider(provider)
	if !ok || !providerDef.SupportsOAuth {
		return nil
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
	if strings.TrimSpace(strings.ToLower(auth.Provider)) != constants.ModelProviderOpenAI {
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

	return strings.TrimSpace(strings.ToLower(auth.Provider)) != constants.ModelProviderOpenAI
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

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
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

	return refreshStoredProviderToken(context.Background(), provider)
}

func loadStoredModelCredential(provider string) (StoredModelCredential, error) {
	if loadStoredProviderToken == nil {
		return StoredModelCredential{}, nil
	}

	credential, err := loadStoredProviderToken(provider)
	if err != nil {
		return StoredModelCredential{}, err
	}

	return credential, nil
}

func getStoredModelCredentialValue(credential StoredModelCredential) string {
	switch strings.TrimSpace(strings.ToLower(credential.Type)) {
	case appcredential.TypeAPIKey:
		return strings.TrimSpace(credential.Key)
	case appcredential.TypeOAuth, "":
		return strings.TrimSpace(credential.Token)
	default:
		return ""
	}
}

func newMissingModelCredentialError(role string, provider string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "model"
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return fmt.Errorf("%s API key is required; set a provider API key, provider env var, role apiKey, "+
			"or run hand auth login <provider>", role)
	}

	return fmt.Errorf("%s API key is required for provider %q; set a provider API key, provider env var, role apiKey,"+
		" or run hand auth login %s", role, provider, provider)
}

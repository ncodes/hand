package provider

import (
	"strings"

	"github.com/wandxy/hand/internal/constants"
)

const (
	// APIOpenAICompletions identifies the OpenAI-compatible chat completions protocol.
	APIOpenAICompletions = "openai-completions"
	// APIOpenAIResponses identifies the OpenAI responses protocol.
	APIOpenAIResponses = "openai-responses"
	// APIOpenAIEmbeddings identifies the OpenAI-compatible embeddings protocol.
	APIOpenAIEmbeddings = "openai-embeddings"
	// APIAnthropicMessages identifies the Anthropic Messages protocol.
	APIAnthropicMessages = "anthropic-messages"
)

// InputKind describes an input modality supported by a model.
type InputKind string

const (
	// InputText identifies plain text input support.
	InputText InputKind = "text"
	// InputImage identifies image input support.
	InputImage InputKind = "image"
)

// APIDefinition describes a request protocol adapter known to the model registry.
type APIDefinition struct {
	ID          string
	DisplayName string
}

// ProviderDefinition describes provider-level routing, defaults, and credential metadata.
type ProviderDefinition struct {
	ID                 string
	DisplayName        string
	DefaultAPI         string
	BaseURLs           map[string]string
	Headers            map[string]string
	APIKeyEnv          []string
	SupportsModels     bool
	RequiresKnownModel bool
	SupportsAPIKey     bool
	SupportsOAuth      bool
}

// ModelDefinition describes provider-specific model metadata used for resolution and validation.
type ModelDefinition struct {
	ID            string
	Name          string
	Provider      string
	API           string
	Input         []InputKind
	Reasoning     bool
	ContextWindow int
	MaxTokens     int
}

// Registry stores API, provider, and model definitions for model resolution.
type Registry struct {
	apis      map[string]APIDefinition
	providers map[string]ProviderDefinition
	models    map[string]map[string]ModelDefinition
}

// NewRegistry builds a registry from API, provider, and model definitions.
func NewRegistry(
	apis []APIDefinition,
	providers []ProviderDefinition,
	models []ModelDefinition,
) *Registry {
	r := &Registry{
		apis:      make(map[string]APIDefinition, len(apis)),
		providers: make(map[string]ProviderDefinition, len(providers)),
		models:    make(map[string]map[string]ModelDefinition),
	}

	for _, api := range apis {
		api.ID = normalizeID(api.ID)
		if api.ID == "" {
			continue
		}
		r.apis[api.ID] = api
	}

	for _, provider := range providers {
		provider.ID = normalizeID(provider.ID)
		provider.DefaultAPI = normalizeID(provider.DefaultAPI)
		if provider.ID == "" {
			continue
		}
		provider.BaseURLs = cloneStringMap(provider.BaseURLs)
		provider.Headers = cloneStringMap(provider.Headers)
		provider.APIKeyEnv = append([]string(nil), provider.APIKeyEnv...)
		r.providers[provider.ID] = provider
	}

	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Provider = normalizeID(model.Provider)
		model.API = normalizeID(model.API)
		if model.Provider == "" || model.ID == "" {
			continue
		}
		model.Input = append([]InputKind(nil), model.Input...)
		if r.models[model.Provider] == nil {
			r.models[model.Provider] = make(map[string]ModelDefinition)
		}
		r.models[model.Provider][model.ID] = model
	}

	return r
}

// DefaultRegistry returns the built-in provider registry.
func DefaultRegistry() *Registry {
	return NewRegistry(defaultAPIs(), defaultProviders(), defaultModels())
}

// GetAPI looks up an API definition by ID.
func (r *Registry) GetAPI(id string) (APIDefinition, bool) {
	if r == nil {
		return APIDefinition{}, false
	}

	api, ok := r.apis[normalizeID(id)]
	return api, ok
}

// GetProvider looks up a provider definition by ID.
func (r *Registry) GetProvider(id string) (ProviderDefinition, bool) {
	if r == nil {
		return ProviderDefinition{}, false
	}

	provider, ok := r.providers[normalizeID(id)]
	if !ok {
		return ProviderDefinition{}, false
	}

	provider.BaseURLs = cloneStringMap(provider.BaseURLs)
	provider.Headers = cloneStringMap(provider.Headers)
	provider.APIKeyEnv = append([]string(nil), provider.APIKeyEnv...)
	return provider, true
}

// GetModel looks up a model definition by provider ID and model ID.
func (r *Registry) GetModel(providerID, modelID string) (ModelDefinition, bool) {
	if r == nil {
		return ModelDefinition{}, false
	}

	providerID = normalizeID(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return ModelDefinition{}, false
	}

	byProvider := r.models[providerID]
	model, ok := byProvider[modelID]
	if !ok {
		return ModelDefinition{}, false
	}

	model.Input = append([]InputKind(nil), model.Input...)
	return model, true
}

// GetProviderIDs returns the provider IDs registered in the registry.
func (r *Registry) GetProviderIDs() []string {
	if r == nil {
		return nil
	}

	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}

	return ids
}

// GetAPIIDs returns the API IDs registered in the registry.
func (r *Registry) GetAPIIDs() []string {
	if r == nil {
		return nil
	}

	ids := make([]string, 0, len(r.apis))
	for id := range r.apis {
		ids = append(ids, id)
	}

	return ids
}

// GetBaseURL returns the provider's base URL for an API ID.
func (r *Registry) GetBaseURL(providerID, apiID string) string {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return ""
	}

	apiID = normalizeID(apiID)
	if apiID == "" {
		apiID = provider.DefaultAPI
	}

	return strings.TrimSpace(provider.BaseURLs[apiID])
}

// SupportsProviderAPI reports whether the provider can use the given API.
func (r *Registry) SupportsProviderAPI(providerID, apiID string) bool {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return false
	}

	apiID = normalizeID(apiID)
	if apiID == "" {
		apiID = provider.DefaultAPI
	}
	if apiID == "" {
		return false
	}

	if provider.DefaultAPI == apiID {
		return true
	}
	return strings.TrimSpace(provider.BaseURLs[apiID]) != ""
}

func normalizeID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		key = normalizeID(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		cloned[key] = value
	}
	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func defaultAPIs() []APIDefinition {
	return []APIDefinition{
		{
			ID:          APIOpenAICompletions,
			DisplayName: "OpenAI Chat Completions",
		},
		{
			ID:          APIOpenAIResponses,
			DisplayName: "OpenAI Responses",
		},
		{
			ID:          APIOpenAIEmbeddings,
			DisplayName: "OpenAI Embeddings",
		},
		{
			ID:          APIAnthropicMessages,
			DisplayName: "Anthropic Messages",
		},
	}
}

func defaultProviders() []ProviderDefinition {
	return []ProviderDefinition{
		{
			ID:             constants.ModelProviderOpenRouter,
			DisplayName:    "OpenRouter",
			DefaultAPI:     APIOpenAIResponses,
			APIKeyEnv:      []string{"OPENROUTER_API_KEY"},
			SupportsModels: true,
			SupportsAPIKey: true,
			BaseURLs: map[string]string{
				APIOpenAICompletions: constants.DefaultOpenRouterBaseURL,
				APIOpenAIResponses:   constants.DefaultOpenRouterResponsesBaseURL,
				APIOpenAIEmbeddings:  constants.DefaultOpenRouterEmbeddingsBaseURL,
			},
		},
		{
			ID:             constants.ModelProviderOpenAI,
			DisplayName:    "OpenAI",
			DefaultAPI:     APIOpenAIResponses,
			APIKeyEnv:      []string{"OPENAI_API_KEY"},
			SupportsModels: true,
			SupportsAPIKey: true,
			BaseURLs: map[string]string{
				APIOpenAICompletions: constants.DefaultOpenAIBaseURL,
				APIOpenAIResponses:   constants.DefaultOpenAIBaseURL,
				APIOpenAIEmbeddings:  constants.DefaultOpenAIEmbeddingsBaseURL,
			},
		},
		{
			ID:             constants.ModelProviderAnthropic,
			DisplayName:    "Anthropic",
			DefaultAPI:     APIAnthropicMessages,
			APIKeyEnv:      []string{"ANTHROPIC_API_KEY"},
			SupportsModels: true,
			SupportsAPIKey: true,
			BaseURLs: map[string]string{
				APIAnthropicMessages: constants.DefaultAnthropicBaseURL,
			},
		},
		{
			ID:             constants.ModelProviderGitHubCopilot,
			DisplayName:    "GitHub Copilot",
			DefaultAPI:     APIOpenAIResponses,
			APIKeyEnv:      []string{"COPILOT_GITHUB_TOKEN"},
			SupportsModels: true,
			SupportsAPIKey: true,
			SupportsOAuth:  true,
		},
	}
}

func defaultModels() []ModelDefinition {
	return []ModelDefinition{
		{
			ID:            constants.DefaultModel,
			Name:          "GPT-4o mini",
			Provider:      constants.ModelProviderOpenAI,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultProfileEmbeddingModel,
			Name:          "Text Embedding 3 Small",
			Provider:      constants.ModelProviderOpenAI,
			API:           APIOpenAIEmbeddings,
			Input:         []InputKind{InputText},
			ContextWindow: 8191,
		},
		{
			ID:            constants.DefaultProfileModel,
			Name:          "MiniMax M2.7",
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultModel,
			Name:          "GPT-4o mini via OpenRouter",
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIResponses,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: constants.DefaultContextLength,
		},
		{
			ID:            constants.DefaultProfileEmbeddingModel,
			Name:          "Text Embedding 3 Small via OpenRouter",
			Provider:      constants.ModelProviderOpenRouter,
			API:           APIOpenAIEmbeddings,
			Input:         []InputKind{InputText},
			ContextWindow: 8191,
		},
		{
			ID:            "anthropic/claude-sonnet-4-5",
			Name:          "Claude Sonnet 4.5",
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     64000,
		},
		{
			ID:            "anthropic/claude-opus-4-1",
			Name:          "Claude Opus 4.1",
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     32000,
		},
		{
			ID:            "anthropic/claude-3-haiku-20240307",
			Name:          "Claude 3 Haiku",
			Provider:      constants.ModelProviderAnthropic,
			API:           APIAnthropicMessages,
			Input:         []InputKind{InputText, InputImage},
			ContextWindow: 200000,
			MaxTokens:     4096,
		},
	}
}

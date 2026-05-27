package config

import appcredential "github.com/wandxy/hand/internal/credential"

type ModelCredentialSourceKind string

const (
	// ModelCredentialSourceRoleConfig means the credential came from the concrete model role config.
	ModelCredentialSourceRoleConfig ModelCredentialSourceKind = "role-config"

	// ModelCredentialSourceProviderConfig means the credential came from provider-specific config.
	ModelCredentialSourceProviderConfig ModelCredentialSourceKind = "provider-config"

	// ModelCredentialSourceProviderEnv means the credential came from a provider-specific environment variable.
	ModelCredentialSourceProviderEnv ModelCredentialSourceKind = "provider-env"

	// ModelCredentialSourceTokenStore means the credential came from a local OAuth or subscription token store.
	ModelCredentialSourceTokenStore ModelCredentialSourceKind = "token-store"
)

// ModelsConfig contains provider credentials and model-specific settings.
type ModelsConfig struct {
	MaxRetries *int                           `yaml:"maxRetries"`
	Providers  map[string]ProviderModelConfig `yaml:"providers"`
	Main       MainModelConfig                `yaml:"main"`
	Summary    SummaryModelConfig             `yaml:"summary"`
	Embedding  EmbeddingModelConfig           `yaml:"embedding"`
}

// ProviderModelConfig describes static credential settings for one model provider.
type ProviderModelConfig struct {
	APIKey    string   `yaml:"apiKey"`
	APIKeyEnv []string `yaml:"apiKeyEnv"`
}

// MainModelConfig selects the model used for normal agent turns.
type MainModelConfig struct {
	Name          string `yaml:"name"`
	Provider      string `yaml:"provider"`
	API           string `yaml:"api"`
	APIKey        string `yaml:"apiKey"`
	BaseURL       string `yaml:"baseUrl"`
	ContextLength int    `yaml:"contextLength"`
	Stream        *bool  `yaml:"stream"`
}

// SummaryModelConfig selects the model used for summaries and compaction.
type SummaryModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	API      string `yaml:"api"`
	APIKey   string `yaml:"apiKey"`
	BaseURL  string `yaml:"baseUrl"`
}

// EmbeddingModelConfig selects the model used for vector embeddings.
type EmbeddingModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	APIKey   string `yaml:"apiKey"`
	BaseURL  string `yaml:"baseUrl"`
}

// ModelCredentialSource describes credential provenance without containing the credential value.
type ModelCredentialSource struct {
	Kind      ModelCredentialSourceKind
	Name      string
	Type      string
	HasExpiry bool
}

// StoredModelCredential describes a locally stored provider credential.
type StoredModelCredential = appcredential.StoredCredential

// ModelAuth describes authentication metadata for a model provider.
type ModelAuth struct {
	Provider         string
	API              string
	APIKey           string
	BaseURL          string
	Headers          map[string]string
	CredentialSource ModelCredentialSource
}

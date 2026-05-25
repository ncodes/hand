package config

// ModelsConfig contains provider credentials and model-specific settings.
type ModelsConfig struct {
	Verify           *bool                `yaml:"verify"`
	MaxRetries       *int                 `yaml:"maxRetries"`
	Key              string               `yaml:"key"`
	OpenAIAPIKey     string               `yaml:"openaiApiKey"`
	OpenRouterAPIKey string               `yaml:"openrouterApiKey"`
	Main             MainModelConfig      `yaml:"main"`
	Summary          SummaryModelConfig   `yaml:"summary"`
	Embedding        EmbeddingModelConfig `yaml:"embedding"`
}

// MainModelConfig selects the model used for normal agent turns.
type MainModelConfig struct {
	Name          string `yaml:"name"`
	Provider      string `yaml:"provider"`
	API           string `yaml:"api"`
	BaseURL       string `yaml:"baseUrl"`
	ContextLength int    `yaml:"contextLength"`
	Stream        *bool  `yaml:"stream"`
}

// SummaryModelConfig selects the model used for summaries and compaction.
type SummaryModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	API      string `yaml:"api"`
	BaseURL  string `yaml:"baseUrl"`
}

// EmbeddingModelConfig selects the model used for vector embeddings.
type EmbeddingModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"baseUrl"`
}

// ModelAuth describes authentication metadata for a model provider.
type ModelAuth struct {
	Provider string
	API      string
	APIKey   string
	BaseURL  string
}

// ModelMetadata describes metadata attached to model records.
type ModelMetadata struct {
	Exists        bool
	ContextLength int
}

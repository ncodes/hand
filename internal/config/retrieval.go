package config

// SearchConfig controls retrieval and reranking behavior.
type SearchConfig struct {
	EnableRerank *bool              `yaml:"enableRerank"`
	Vector       SearchVectorConfig `yaml:"vector"`
}

// SearchVectorConfig controls vector indexing and repair behavior.
type SearchVectorConfig struct {
	Enabled          bool `yaml:"enabled"`
	Required         bool `yaml:"required"`
	RebuildBatchSize int  `yaml:"rebuildBatchSize"`
}

// RerankerConfig controls retrieval reranker type, model, limits, and overrides.
type RerankerConfig struct {
	Enabled               *bool                             `yaml:"enabled"`
	Type                  string                            `yaml:"type"`
	Model                 string                            `yaml:"model"`
	MaxCandidates         int                               `yaml:"maxCandidates"`
	MaxCandidateTextChars int                               `yaml:"maxCandidateTextChars"`
	MaxOutputTokens       int                               `yaml:"maxOutputTokens"`
	Overrides             map[string]RerankerOverrideConfig `yaml:"overrides"`
}

// RerankerOverrideConfig overrides reranker settings for one use case.
type RerankerOverrideConfig struct {
	Type                  string `yaml:"type,omitempty"`
	Model                 string `yaml:"model,omitempty"`
	MaxCandidates         *int   `yaml:"maxCandidates,omitempty"`
	MaxCandidateTextChars *int   `yaml:"maxCandidateTextChars,omitempty"`
	MaxOutputTokens       *int   `yaml:"maxOutputTokens,omitempty"`
}

// RerankerEffectiveConfig is the resolved reranker configuration after defaults and overrides are applied.
type RerankerEffectiveConfig struct {
	Type                     string
	Model                    string
	MaxCandidates            int
	MaxCandidatesSet         bool
	MaxCandidateTextChars    int
	MaxCandidateTextCharsSet bool
	MaxOutputTokens          int
}

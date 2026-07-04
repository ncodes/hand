package config

import "time"

// MemoryConfig controls memory providers, retrieval, writes, and background jobs.
type MemoryConfig struct {
	Enabled    *bool                  `yaml:"enabled"`
	Provider   string                 `yaml:"provider"`
	Backend    string                 `yaml:"backend"`
	Pinned     PinnedMemoryConfig     `yaml:"pinned"`
	Retrieval  RetrievalMemoryConfig  `yaml:"retrieval"`
	Flush      FlushMemoryConfig      `yaml:"flush"`
	Episodic   EpisodicMemoryConfig   `yaml:"episodic"`
	Reflection ReflectionMemoryConfig `yaml:"reflection"`
	Promotion  PromotionMemoryConfig  `yaml:"promotion"`
	Write      WriteMemoryConfig      `yaml:"write"`
}

// PinnedMemoryConfig controls always-in-context memory limits.
type PinnedMemoryConfig struct {
	Enabled      *bool `yaml:"enabled"`
	MaxChars     int   `yaml:"maxChars"`
	MaxItemChars int   `yaml:"maxItemChars"`
}

// RetrievalMemoryConfig toggles retrieval-backed memory.
type RetrievalMemoryConfig struct {
	Enabled *bool `yaml:"enabled"`
}

// FlushMemoryConfig controls memory extraction during session flush.
type FlushMemoryConfig struct {
	Enabled         *bool         `yaml:"enabled"`
	MaxCalls        int           `yaml:"maxCalls"`
	MaxOutputTokens int64         `yaml:"maxOutputTokens"`
	Timeout         time.Duration `yaml:"timeout"`
}

// EpisodicMemoryConfig controls background episodic memory extraction.
type EpisodicMemoryConfig struct {
	Enabled         *bool         `yaml:"enabled"`
	Interval        time.Duration `yaml:"interval"`
	IdleAfter       time.Duration `yaml:"idleAfter"`
	MinMessages     int           `yaml:"minMessages"`
	WindowSize      int           `yaml:"windowSize"`
	MaxWindows      int           `yaml:"maxWindows"`
	MaxWindowChars  int           `yaml:"maxWindowChars"`
	MaxWindowTokens int           `yaml:"maxWindowTokens"`
	MaxRetries      int           `yaml:"maxRetries"`
}

// ReflectionMemoryConfig controls promotion from episodic memories into reflections.
type ReflectionMemoryConfig struct {
	Enabled      *bool         `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	Limit        int           `yaml:"limit"`
	RelatedLimit int           `yaml:"relatedLimit"`
}

// PromotionMemoryConfig controls memory lifecycle promotion.
type PromotionMemoryConfig struct {
	Enabled            *bool         `yaml:"enabled"`
	Interval           time.Duration `yaml:"interval"`
	Limit              int           `yaml:"limit"`
	EvaluatedRetention time.Duration `yaml:"evaluatedRetention"`
}

// WriteMemoryConfig toggles model-initiated memory writes.
type WriteMemoryConfig struct {
	Enabled *bool `yaml:"enabled"`
}

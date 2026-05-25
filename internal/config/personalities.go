package config

// PersonalityConfig applies per-personality instructions, model, memory, and tool overrides.
type PersonalityConfig struct {
	Soul          string                  `yaml:"soul"`
	Instruct      string                  `yaml:"instruct"`
	State         string                  `yaml:"state"`
	Memory        PersonalityMemoryConfig `yaml:"memory"`
	Tools         PersonalityToolsConfig  `yaml:"tools"`
	Model         MainModelConfig         `yaml:"model"`
	MaxIterations int                     `yaml:"maxIterations"`
}

// PersonalityMemoryConfig overrides memory features for a personality.
type PersonalityMemoryConfig struct {
	Pinned     *bool `yaml:"pinned"`
	Retrieval  *bool `yaml:"retrieval"`
	Write      *bool `yaml:"write"`
	Episodic   *bool `yaml:"episodic"`
	Reflection *bool `yaml:"reflection"`
	Promotion  *bool `yaml:"promotion"`
	Flush      *bool `yaml:"flush"`
}

// PersonalityToolsConfig overrides tool capabilities for a personality.
type PersonalityToolsConfig struct {
	Filesystem *bool  `yaml:"fs"`
	Network    *bool  `yaml:"net"`
	Exec       *bool  `yaml:"exec"`
	Memory     string `yaml:"mem"`
}

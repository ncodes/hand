package config

// TraceConfig controls trace collection backends.
type TraceConfig struct {
	Enabled  bool                `yaml:"enabled"`
	Disk     TraceDiskConfig     `yaml:"disk"`
	Database TraceDatabaseConfig `yaml:"database"`
}

// TraceDiskConfig controls JSONL trace writing.
type TraceDiskConfig struct {
	Enabled *bool  `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

// TraceDatabaseConfig controls database-backed trace persistence.
type TraceDatabaseConfig struct {
	Enabled             *bool `yaml:"enabled"`
	MaxEventsPerSession int   `yaml:"maxEventsPerSession"`
}

package config

import (
	"os"
	"regexp"
	"sync"

	"github.com/joho/godotenv"

	appcredential "github.com/wandxy/hand/internal/credential"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

// Config is the root runtime configuration for Hand.
type Config struct {
	Name          string                       `yaml:"name"`
	Platform      string                       `yaml:"platform"`
	Models        ModelsConfig                 `yaml:"models"`
	RPC           RPCConfig                    `yaml:"rpc"`
	Gateway       GatewayConfig                `yaml:"gateway"`
	FS            FSConfig                     `yaml:"fs"`
	Exec          ExecConfig                   `yaml:"exec"`
	Storage       StorageConfig                `yaml:"storage"`
	Session       SessionConfig                `yaml:"session"`
	Search        SearchConfig                 `yaml:"search"`
	Memory        MemoryConfig                 `yaml:"memory"`
	Reranker      RerankerConfig               `yaml:"reranker"`
	Compaction    CompactionConfig             `yaml:"compaction"`
	Cap           CapConfig                    `yaml:"cap"`
	Log           LogConfig                    `yaml:"log"`
	Debug         DebugConfig                  `yaml:"debug"`
	Trace         TraceConfig                  `yaml:"trace"`
	TUI           TUIConfig                    `yaml:"tui"`
	Web           WebConfig                    `yaml:"web"`
	Safety        SafetyConfig                 `yaml:"safety"`
	Rules         RulesConfig                  `yaml:"rules"`
	Personalities map[string]PersonalityConfig `yaml:"personalities"`
}

var (
	globalConfig               *Config
	configMu                   sync.RWMutex
	loadDotEnv                 = godotenv.Load
	getwd                      = os.Getwd
	modelRegistry              = modelprovider.DefaultRegistry()
	loadStoredProviderToken    = appcredential.LoadStoredProviderCredential
	refreshStoredProviderToken = appcredential.RefreshStoredProviderCredential
	getSubscriptionProvider    = appcredential.GetSubscriptionProvider
)

const (
	personalityNamePattern     = `[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`
	personalityStateShared     = "shared"
	personalityStateIsolated   = "isolated"
	personalityStateReadonly   = "readonly"
	personalityToolMemoryNone  = "none"
	personalityToolMemoryRead  = "read"
	personalityToolMemoryWrite = "write"
)

var validPersonalityName = regexp.MustCompile(`^` + personalityNamePattern + `$`)

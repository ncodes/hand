package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
)

type Config struct {
	Name          string                       `yaml:"name"`
	Platform      string                       `yaml:"platform"`
	Models        ModelsConfig                 `yaml:"models"`
	RPC           RPCConfig                    `yaml:"rpc"`
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

type MainModelConfig struct {
	Name          string `yaml:"name"`
	Provider      string `yaml:"provider"`
	APIMode       string `yaml:"apiMode"`
	BaseURL       string `yaml:"baseUrl"`
	ContextLength int    `yaml:"contextLength"`
	Stream        *bool  `yaml:"stream"`
}

type SummaryModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	APIMode  string `yaml:"apiMode"`
	BaseURL  string `yaml:"baseUrl"`
}

type EmbeddingModelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"baseUrl"`
}

type RPCConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

type FSConfig struct {
	NoProfileAccess bool     `yaml:"noProfileAccess"`
	Roots           []string `yaml:"roots"`
}

type ExecConfig struct {
	Allow []string `yaml:"allow"`
	Ask   []string `yaml:"ask"`
	Deny  []string `yaml:"deny"`
}

type StorageConfig struct {
	Backend string `yaml:"backend"`
}

type SessionConfig struct {
	MaxIterations     int           `yaml:"maxIterations"`
	Instruct          string        `yaml:"instruct"`
	DefaultIdleExpiry time.Duration `yaml:"defaultIdleExpiry"`
	ArchiveRetention  time.Duration `yaml:"archiveRetention"`
}

type SearchConfig struct {
	EnableRerank *bool              `yaml:"enableRerank"`
	Vector       SearchVectorConfig `yaml:"vector"`
}

type SearchVectorConfig struct {
	Enabled          bool `yaml:"enabled"`
	Required         bool `yaml:"required"`
	RebuildBatchSize int  `yaml:"rebuildBatchSize"`
}

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

type PinnedMemoryConfig struct {
	Enabled      *bool `yaml:"enabled"`
	MaxChars     int   `yaml:"maxChars"`
	MaxItemChars int   `yaml:"maxItemChars"`
}

type RetrievalMemoryConfig struct {
	Enabled *bool `yaml:"enabled"`
}

type FlushMemoryConfig struct {
	Enabled         *bool         `yaml:"enabled"`
	MaxCalls        int           `yaml:"maxCalls"`
	MaxOutputTokens int64         `yaml:"maxOutputTokens"`
	Timeout         time.Duration `yaml:"timeout"`
}

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

type ReflectionMemoryConfig struct {
	Enabled      *bool         `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	Limit        int           `yaml:"limit"`
	RelatedLimit int           `yaml:"relatedLimit"`
}

type PromotionMemoryConfig struct {
	Enabled  *bool         `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
	Limit    int           `yaml:"limit"`
}

type WriteMemoryConfig struct {
	Enabled *bool `yaml:"enabled"`
}

type RerankerConfig struct {
	Enabled               *bool                             `yaml:"enabled"`
	Type                  string                            `yaml:"type"`
	Model                 string                            `yaml:"model"`
	MaxCandidates         int                               `yaml:"maxCandidates"`
	MaxCandidateTextChars int                               `yaml:"maxCandidateTextChars"`
	MaxOutputTokens       int                               `yaml:"maxOutputTokens"`
	Overrides             map[string]RerankerOverrideConfig `yaml:"overrides"`
}

type RerankerOverrideConfig struct {
	Type                  string `yaml:"type,omitempty"`
	Model                 string `yaml:"model,omitempty"`
	MaxCandidates         *int   `yaml:"maxCandidates,omitempty"`
	MaxCandidateTextChars *int   `yaml:"maxCandidateTextChars,omitempty"`
	MaxOutputTokens       *int   `yaml:"maxOutputTokens,omitempty"`
}

type RerankerEffectiveConfig struct {
	Type                     string
	Model                    string
	MaxCandidates            int
	MaxCandidatesSet         bool
	MaxCandidateTextChars    int
	MaxCandidateTextCharsSet bool
	MaxOutputTokens          int
}

type CompactionConfig struct {
	Enabled           *bool   `yaml:"enabled"`
	TriggerPercent    float64 `yaml:"triggerPercent"`
	WarnPercent       float64 `yaml:"warnPercent"`
	RecentSessionTail *int    `yaml:"recentSessionTail"`
}

type CapConfig struct {
	Filesystem *bool `yaml:"fs"`
	Network    *bool `yaml:"net"`
	Exec       *bool `yaml:"exec"`
	Memory     *bool `yaml:"mem"`
	Browser    *bool `yaml:"browser"`
}

func (c *Config) CompactionRecentSessionTailEffective() int {
	if c == nil || c.Compaction.RecentSessionTail == nil {
		return constants.RecentSessionTail
	}

	if *c.Compaction.RecentSessionTail < 0 {
		return 0
	}

	return *c.Compaction.RecentSessionTail
}

type LogConfig struct {
	Level   string `yaml:"level"`
	NoColor bool   `yaml:"noColor"`
}

type DebugConfig struct {
	Requests bool `yaml:"requests"`
}

type TraceConfig struct {
	Enabled  bool                `yaml:"enabled"`
	Disk     TraceDiskConfig     `yaml:"disk"`
	Database TraceDatabaseConfig `yaml:"database"`
}

type TraceDiskConfig struct {
	Enabled *bool  `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

type TraceDatabaseConfig struct {
	Enabled             *bool `yaml:"enabled"`
	MaxEventsPerSession int   `yaml:"maxEventsPerSession"`
}

type TUIConfig struct {
	ThinkingComposer *bool `yaml:"thinkingComposer"`
}

type SafetyConfig struct {
	Input  *bool `yaml:"input"`
	Output *bool `yaml:"output"`
	PII    *bool `yaml:"pii"`
}

type WebConfig struct {
	Provider                     string        `yaml:"provider"`
	APIKey                       string        `yaml:"apiKey"`
	BaseURL                      string        `yaml:"baseUrl"`
	MaxCharPerResult             int           `yaml:"maxCharPerResult"`
	MaxExtractCharPerResult      int           `yaml:"maxExtractCharPerResult"`
	MaxExtractResponseBytes      int           `yaml:"maxExtractResponseBytes"`
	CacheTTL                     time.Duration `yaml:"cacheTTL"`
	BlockedDomainsEnabled        bool          `yaml:"-"`
	BlockedDomains               []string      `yaml:"-"`
	BlockedDomainFiles           []string      `yaml:"-"`
	NativeAllowedHosts           []string      `yaml:"-"`
	NativeBlockedHosts           []string      `yaml:"-"`
	NativeAllowedHostFiles       []string      `yaml:"-"`
	NativeBlockedHostFiles       []string      `yaml:"-"`
	ExtractMinSummarizeChars     int           `yaml:"extractMinSummarizeChars"`
	ExtractMaxSummaryChars       int           `yaml:"extractMaxSummaryChars"`
	ExtractMaxSummaryChunkChars  int           `yaml:"extractMaxSummaryChunkChars"`
	ExtractRefusalThresholdChars int           `yaml:"extractRefusalThresholdChars"`
}

type RulesConfig struct {
	Files []string `yaml:"files"`
}

type PersonalityConfig struct {
	Soul          string                  `yaml:"soul"`
	Instruct      string                  `yaml:"instruct"`
	State         string                  `yaml:"state"`
	Memory        PersonalityMemoryConfig `yaml:"memory"`
	Tools         PersonalityToolsConfig  `yaml:"tools"`
	Model         MainModelConfig         `yaml:"model"`
	MaxIterations int                     `yaml:"maxIterations"`
}

type PersonalityMemoryConfig struct {
	Pinned     *bool `yaml:"pinned"`
	Retrieval  *bool `yaml:"retrieval"`
	Write      *bool `yaml:"write"`
	Episodic   *bool `yaml:"episodic"`
	Reflection *bool `yaml:"reflection"`
	Promotion  *bool `yaml:"promotion"`
	Flush      *bool `yaml:"flush"`
}

type PersonalityToolsConfig struct {
	Filesystem *bool  `yaml:"fs"`
	Network    *bool  `yaml:"net"`
	Exec       *bool  `yaml:"exec"`
	Memory     string `yaml:"mem"`
}

func (c *WebConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain WebConfig
	var raw struct {
		plain          `yaml:",inline"`
		BlockedDomains struct {
			Enabled bool     `yaml:"enabled"`
			Domains []string `yaml:"domains"`
			Files   []string `yaml:"files"`
		} `yaml:"blockedDomains"`
		Native struct {
			AllowedHosts     []string `yaml:"allowedHosts"`
			BlockedHosts     []string `yaml:"blockedHosts"`
			AllowedHostFiles []string `yaml:"allowedHostFiles"`
			BlockedHostFiles []string `yaml:"blockedHostFiles"`
		} `yaml:"native"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	*c = WebConfig(raw.plain)
	c.BlockedDomainsEnabled = raw.BlockedDomains.Enabled
	c.BlockedDomains = raw.BlockedDomains.Domains
	c.BlockedDomainFiles = raw.BlockedDomains.Files
	c.NativeAllowedHosts = raw.Native.AllowedHosts
	c.NativeBlockedHosts = raw.Native.BlockedHosts
	c.NativeAllowedHostFiles = raw.Native.AllowedHostFiles
	c.NativeBlockedHostFiles = raw.Native.BlockedHostFiles

	return nil
}

type ModelAuth struct {
	Provider string
	APIKey   string
	BaseURL  string
}

type ModelMetadata struct {
	Exists        bool
	ContextLength int
}

var (
	globalConfig            *Config
	configMu                sync.RWMutex
	loadDotEnv              = godotenv.Load
	getwd                   = os.Getwd
	httpClient              = &http.Client{Timeout: 5 * time.Second}
	modelDocsBaseURL        = "https://developers.openai.com/api/docs/models"
	resolveModelMeta        = fetchModelMetadataFromProvider
	providerDefaultBaseURLs = map[string]map[string]string{
		constants.ModelProviderOpenRouter: {
			constants.DefaultModelAPIModeCompletions: constants.DefaultOpenRouterBaseURL,
			"responses":                              constants.DefaultOpenRouterResponsesBaseURL,
			"embeddings":                             constants.DefaultOpenRouterEmbeddingsBaseURL,
		},
		constants.ModelProviderOpenAI: {
			constants.DefaultModelAPIModeCompletions: constants.DefaultOpenAIBaseURL,
			"responses":                              constants.DefaultOpenAIBaseURL,
			"embeddings":                             constants.DefaultOpenAIEmbeddingsBaseURL,
		},
	}
)

var contextWindowPatternOAI = regexp.MustCompile(`([0-9][0-9,]*)(?:\s|<!--[^>]*-->)+context window`)

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

// DefaultConfig is the canonical baseline for new Hand configuration.
var DefaultConfig = Config{
	Models: ModelsConfig{
		Main: MainModelConfig{
			Name:          constants.DefaultProfileModel,
			Provider:      constants.ModelProviderOpenRouter,
			Stream:        new(constants.DefaultProfileModelStream),
			ContextLength: constants.DefaultContextLength,
			APIMode:       constants.DefaultModelAPIModeCompletions,
			BaseURL:       constants.DefaultOpenRouterBaseURL,
		},
		Summary: SummaryModelConfig{
			Name: constants.DefaultProfileSummaryModel,
		},
		Embedding: EmbeddingModelConfig{
			Name: constants.DefaultProfileEmbeddingModel,
		},
		Verify:     new(constants.DefaultProfileModelVerify),
		MaxRetries: new(constants.DefaultModelMaxRetries),
	},
	Session: SessionConfig{
		MaxIterations:     constants.DefaultMaxIterations,
		DefaultIdleExpiry: constants.DefaultSessionIdleExpiry,
		ArchiveRetention:  constants.DefaultArchiveRetention,
	},
	RPC: RPCConfig{
		Address: constants.DefaultRPCAddress,
		Port:    constants.DefaultRPCPort,
	},
	FS: FSConfig{
		NoProfileAccess: true,
	},
	Log: LogConfig{
		Level: constants.DefaultProfileLogLevel,
	},
	Debug: DebugConfig{
		Requests: constants.DefaultProfileDebugRequests,
	},
	Trace: TraceConfig{
		Enabled: constants.DefaultProfileTraceEnabled,
		Disk: TraceDiskConfig{
			Enabled: new(constants.DefaultProfileTraceDiskEnabled),
		},
		Database: TraceDatabaseConfig{
			Enabled:             new(constants.DefaultProfileTraceDatabaseEnabled),
			MaxEventsPerSession: constants.DefaultTraceMaxEventsPerSession,
		},
	},
	TUI: TUIConfig{
		ThinkingComposer: new(constants.DefaultTUIThinkingComposerEnabled),
	},
	Safety: SafetyConfig{
		Input:  new(constants.DefaultSafetyInputEnabled),
		Output: new(constants.DefaultSafetyOutputEnabled),
		PII:    new(constants.DefaultSafetyPIIEnabled),
	},
	Web: WebConfig{
		Provider:                     constants.DefaultProfileWebProvider,
		MaxCharPerResult:             constants.DefaultWebMaxCharPerResult,
		MaxExtractCharPerResult:      constants.DefaultWebMaxExtractCharPerResult,
		MaxExtractResponseBytes:      constants.DefaultWebMaxExtractResponseBytes,
		CacheTTL:                     constants.DefaultProfileWebCacheTTL,
		BlockedDomainsEnabled:        constants.DefaultProfileWebBlockedDomainsEnabled,
		ExtractMinSummarizeChars:     constants.DefaultWebExtractMinSummarizeChars,
		ExtractMaxSummaryChars:       constants.DefaultWebExtractMaxSummaryChars,
		ExtractMaxSummaryChunkChars:  constants.DefaultWebExtractMaxSummaryChunkChars,
		ExtractRefusalThresholdChars: constants.DefaultWebExtractRefusalThresholdChars,
	},
	Platform: constants.DefaultPlatform,
	Cap: CapConfig{
		Filesystem: new(constants.DefaultProfileCapabilityFilesystem),
		Network:    new(constants.DefaultProfileCapabilityNetwork),
		Exec:       new(constants.DefaultProfileCapabilityExec),
		Memory:     new(constants.DefaultProfileCapabilityMemory),
		Browser:    new(constants.DefaultProfileCapabilityBrowser),
	},
	Storage: StorageConfig{
		Backend: constants.DefaultStorageBackend,
	},
	Search: SearchConfig{
		EnableRerank: new(constants.DefaultProfileSearchEnableRerank),
		Vector: SearchVectorConfig{
			Enabled:          constants.DefaultProfileSearchVectorEnabled,
			Required:         constants.DefaultProfileSearchVectorRequired,
			RebuildBatchSize: constants.DefaultVectorStoreRebuildBatchSize,
		},
	},
	Reranker: RerankerConfig{
		Enabled:               new(constants.DefaultProfileRerankerEnabled),
		Type:                  constants.RerankerDeterministic,
		MaxCandidates:         constants.DefaultProfileRerankerMaxCandidates,
		MaxCandidateTextChars: constants.DefaultProfileRerankerMaxCandidateTextChars,
		MaxOutputTokens:       constants.DefaultProfileRerankerMaxOutputTokens,
		Overrides: map[string]RerankerOverrideConfig{
			"memory_episodic_extraction": {Type: constants.RerankerLLM},
			"memory_promotion":           {Type: constants.RerankerLLM},
			"memory_reflection":          {Type: constants.RerankerLLM},
		},
	},
	Compaction: CompactionConfig{
		Enabled:           new(constants.DefaultProfileCompactionEnabled),
		TriggerPercent:    constants.DefaultCompactionTrigger,
		WarnPercent:       constants.DefaultCompactionWarn,
		RecentSessionTail: new(constants.RecentSessionTail),
	},
	Memory: MemoryConfig{
		Enabled:  new(constants.DefaultProfileMemoryEnabled),
		Provider: constants.MemoryProviderDefault,
		Pinned: PinnedMemoryConfig{
			Enabled:      new(constants.DefaultProfileMemoryPinnedEnabled),
			MaxChars:     constants.DefaultProfileMemoryPinnedMaxChars,
			MaxItemChars: constants.DefaultProfileMemoryPinnedMaxItemChars,
		},
		Retrieval: RetrievalMemoryConfig{
			Enabled: new(constants.DefaultProfileMemoryRetrievalEnabled),
		},
		Flush: FlushMemoryConfig{
			Enabled:         new(constants.DefaultProfileMemoryFlushEnabled),
			MaxCalls:        constants.DefaultProfileMemoryFlushMaxCalls,
			MaxOutputTokens: constants.DefaultProfileMemoryFlushMaxOutputTokens,
			Timeout:         constants.DefaultProfileMemoryFlushTimeout,
		},
		Episodic: EpisodicMemoryConfig{
			Enabled:         new(constants.DefaultProfileMemoryEpisodicEnabled),
			Interval:        constants.DefaultProfileMemoryEpisodicInterval,
			IdleAfter:       constants.DefaultProfileMemoryEpisodicIdleAfter,
			MinMessages:     constants.DefaultProfileMemoryEpisodicMinMessages,
			WindowSize:      constants.DefaultProfileMemoryEpisodicWindowSize,
			MaxWindows:      constants.DefaultProfileMemoryEpisodicMaxWindows,
			MaxWindowChars:  constants.DefaultProfileMemoryEpisodicMaxWindowChars,
			MaxWindowTokens: constants.DefaultProfileMemoryEpisodicMaxWindowTokens,
			MaxRetries:      constants.DefaultProfileMemoryEpisodicMaxRetries,
		},
		Reflection: ReflectionMemoryConfig{
			Enabled:      new(constants.DefaultProfileMemoryReflectionEnabled),
			Interval:     constants.DefaultProfileMemoryReflectionInterval,
			Limit:        constants.DefaultProfileMemoryReflectionLimit,
			RelatedLimit: constants.DefaultProfileMemoryReflectionRelatedLimit,
		},
		Promotion: PromotionMemoryConfig{
			Enabled:  new(constants.DefaultProfileMemoryPromotionEnabled),
			Interval: constants.DefaultProfileMemoryPromotionInterval,
			Limit:    constants.DefaultProfileMemoryPromotionLimit,
		},
		Write: WriteMemoryConfig{
			Enabled: new(constants.DefaultProfileMemoryWriteEnabled),
		},
	},
}

func PreloadEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = ".env"
	}

	if err := loadDotEnv(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load env file %q: %w", path, err)
	}

	return nil
}

func Load(envPath, configPath string) (*Config, error) {
	if err := PreloadEnvFile(envPath); err != nil {
		return nil, err
	}

	cfg, err := loadConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	applyEnvOverrides(cfg)
	requestedContextLength := cfg.Models.Main.ContextLength
	cfg.Normalize()
	applyProviderModelMetadata(context.Background(), cfg, requestedContextLength)

	return cfg, nil
}

func Get() *Config {
	configMu.RLock()
	defer configMu.RUnlock()

	if globalConfig == nil {
		return NewDefaultConfig()
	}

	return globalConfig
}

// NewDefaultConfig returns an independent default config instance.
func NewDefaultConfig() *Config {
	cfg := cloneConfig(DefaultConfig)
	cfg.FS.Roots = getDefaultFSRoots()

	return &cfg
}

// ToYAML returns cfg encoded as a YAML config file.
func (c *Config) ToYAML() ([]byte, error) {
	if c == nil {
		return nil, errors.New("config is required")
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return data, nil
}

// SaveYAML writes cfg to path without overwriting an existing file.
func SaveYAML(path string, cfg *Config) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("config path is required")
	}

	data, err := cfg.ToYAML()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("config file already exists: %s", path)
		}

		return fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

func cloneConfig(cfg Config) Config {
	cfg.Models.Verify = cloneBoolPtr(cfg.Models.Verify)
	cfg.Models.MaxRetries = cloneIntPtr(cfg.Models.MaxRetries)
	cfg.Models.Main.Stream = cloneBoolPtr(cfg.Models.Main.Stream)
	cfg.Search.EnableRerank = cloneBoolPtr(cfg.Search.EnableRerank)
	cfg.Memory.Enabled = cloneBoolPtr(cfg.Memory.Enabled)
	cfg.Memory.Pinned.Enabled = cloneBoolPtr(cfg.Memory.Pinned.Enabled)
	cfg.Memory.Retrieval.Enabled = cloneBoolPtr(cfg.Memory.Retrieval.Enabled)
	cfg.Memory.Flush.Enabled = cloneBoolPtr(cfg.Memory.Flush.Enabled)
	cfg.Memory.Episodic.Enabled = cloneBoolPtr(cfg.Memory.Episodic.Enabled)
	cfg.Memory.Reflection.Enabled = cloneBoolPtr(cfg.Memory.Reflection.Enabled)
	cfg.Memory.Promotion.Enabled = cloneBoolPtr(cfg.Memory.Promotion.Enabled)
	cfg.Memory.Write.Enabled = cloneBoolPtr(cfg.Memory.Write.Enabled)
	cfg.Reranker.Enabled = cloneBoolPtr(cfg.Reranker.Enabled)
	cfg.Reranker.Overrides = cloneRerankerOverrides(cfg.Reranker.Overrides)
	cfg.Compaction.Enabled = cloneBoolPtr(cfg.Compaction.Enabled)
	cfg.Compaction.RecentSessionTail = cloneIntPtr(cfg.Compaction.RecentSessionTail)
	cfg.Cap.Filesystem = cloneBoolPtr(cfg.Cap.Filesystem)
	cfg.Cap.Network = cloneBoolPtr(cfg.Cap.Network)
	cfg.Cap.Exec = cloneBoolPtr(cfg.Cap.Exec)
	cfg.Cap.Memory = cloneBoolPtr(cfg.Cap.Memory)
	cfg.Cap.Browser = cloneBoolPtr(cfg.Cap.Browser)
	cfg.Trace.Disk.Enabled = cloneBoolPtr(cfg.Trace.Disk.Enabled)
	cfg.Trace.Database.Enabled = cloneBoolPtr(cfg.Trace.Database.Enabled)
	cfg.TUI.ThinkingComposer = cloneBoolPtr(cfg.TUI.ThinkingComposer)
	cfg.Safety.Input = cloneBoolPtr(cfg.Safety.Input)
	cfg.Safety.Output = cloneBoolPtr(cfg.Safety.Output)
	cfg.Safety.PII = cloneBoolPtr(cfg.Safety.PII)
	cfg.FS.Roots = slices.Clone(cfg.FS.Roots)
	cfg.Exec.Allow = slices.Clone(cfg.Exec.Allow)
	cfg.Exec.Ask = slices.Clone(cfg.Exec.Ask)
	cfg.Exec.Deny = slices.Clone(cfg.Exec.Deny)
	cfg.Web.BlockedDomains = slices.Clone(cfg.Web.BlockedDomains)
	cfg.Web.BlockedDomainFiles = slices.Clone(cfg.Web.BlockedDomainFiles)
	cfg.Web.NativeAllowedHosts = slices.Clone(cfg.Web.NativeAllowedHosts)
	cfg.Web.NativeBlockedHosts = slices.Clone(cfg.Web.NativeBlockedHosts)
	cfg.Web.NativeAllowedHostFiles = slices.Clone(cfg.Web.NativeAllowedHostFiles)
	cfg.Web.NativeBlockedHostFiles = slices.Clone(cfg.Web.NativeBlockedHostFiles)
	cfg.Rules.Files = slices.Clone(cfg.Rules.Files)
	cfg.Personalities = clonePersonalityConfigs(cfg.Personalities)

	return cfg
}

func cloneRerankerOverrides(overrides map[string]RerankerOverrideConfig) map[string]RerankerOverrideConfig {
	if len(overrides) == 0 {
		return nil
	}

	cloned := make(map[string]RerankerOverrideConfig, len(overrides))
	maps.Copy(cloned, overrides)
	for useCase, override := range cloned {
		override.MaxCandidates = cloneIntPtr(override.MaxCandidates)
		override.MaxCandidateTextChars = cloneIntPtr(override.MaxCandidateTextChars)
		override.MaxOutputTokens = cloneIntPtr(override.MaxOutputTokens)
		cloned[useCase] = override
	}

	return cloned
}

func clonePersonalityConfigs(values map[string]PersonalityConfig) map[string]PersonalityConfig {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]PersonalityConfig, len(values))
	for name, personality := range values {
		personality.Memory.Pinned = cloneBoolPtr(personality.Memory.Pinned)
		personality.Memory.Retrieval = cloneBoolPtr(personality.Memory.Retrieval)
		personality.Memory.Write = cloneBoolPtr(personality.Memory.Write)
		personality.Memory.Episodic = cloneBoolPtr(personality.Memory.Episodic)
		personality.Memory.Reflection = cloneBoolPtr(personality.Memory.Reflection)
		personality.Memory.Promotion = cloneBoolPtr(personality.Memory.Promotion)
		personality.Memory.Flush = cloneBoolPtr(personality.Memory.Flush)
		personality.Tools.Filesystem = cloneBoolPtr(personality.Tools.Filesystem)
		personality.Tools.Network = cloneBoolPtr(personality.Tools.Network)
		personality.Tools.Exec = cloneBoolPtr(personality.Tools.Exec)
		personality.Model.Stream = cloneBoolPtr(personality.Model.Stream)
		cloned[name] = personality
	}

	return cloned
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}

	return new(*value)
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}

	return new(*value)
}

func Set(cfg *Config) {
	configMu.Lock()
	defer configMu.Unlock()
	globalConfig = cfg
}

func loadConfigFile(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "config.yaml"
	}
	baseDir := filepath.Dir(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewDefaultConfig(), nil
		}

		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	cfg := cloneConfig(DefaultConfig)
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	cfg.resolvePaths(baseDir)

	return &cfg, nil
}

func (c *Config) resolvePaths(baseDir string) {
	if c == nil {
		return
	}

	c.FS.Roots = getPathsFromBase(c.FS.Roots, getWorkingDirectory())
	c.Web.BlockedDomainFiles = getPathsFromBase(c.Web.BlockedDomainFiles, baseDir)
	c.Web.NativeAllowedHostFiles = getPathsFromBase(c.Web.NativeAllowedHostFiles, baseDir)
	c.Web.NativeBlockedHostFiles = getPathsFromBase(c.Web.NativeBlockedHostFiles, baseDir)
	c.resolvePersonalitySoulPaths(baseDir)
}

func AddFilesystemRoots(cfg *Config, roots ...string) {
	if cfg == nil {
		return
	}

	cfg.FS.Roots = normalizeFSRoots(append(cfg.FS.Roots, roots...))
}

func (c *Config) resolvePersonalitySoulPaths(baseDir string) {
	if c == nil || len(c.Personalities) == 0 {
		return
	}

	resolved := make(map[string]PersonalityConfig, len(c.Personalities))
	for name, personality := range c.Personalities {
		personality.Soul = resolvePersonalitySoulPath(personality.Soul, baseDir)
		resolved[name] = personality
	}
	c.Personalities = resolved
}

func resolvePersonalitySoulPath(path string, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}

	profileHome := strings.TrimSpace(datadir.HomeDir())
	if profileHome != "" {
		profilePath := filepath.Join(profileHome, path)
		if _, err := os.Stat(profilePath); err == nil {
			return profilePath
		}
	}

	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return path
	}

	return filepath.Join(baseDir, path)
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if value := strings.TrimSpace(os.Getenv("HAND_NAME")); value != "" {
		cfg.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL")); value != "" {
		cfg.Models.Main.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY")); value != "" {
		cfg.Models.Summary.Name = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MODEL_STREAM"); ok {
		cfg.Models.Main.Stream = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_CONTEXT_LENGTH")); value != "" {
		if contextLength, err := strconv.Atoi(value); err == nil {
			cfg.Models.Main.ContextLength = contextLength
		}
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_MODELS_VERIFY"))); value != "" {
		cfg.Models.Verify = new(value == "1" || value == "true" || value == "yes")
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_MAX_RETRIES")); value != "" {
		if retries, err := strconv.Atoi(value); err == nil {
			cfg.Models.MaxRetries = &retries
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_PROVIDER")); value != "" {
		cfg.Models.Main.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_EMBEDDING_PROVIDER")); value != "" {
		cfg.Models.Embedding.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_EMBEDDING_MODEL")); value != "" {
		cfg.Models.Embedding.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_KEY")); value != "" {
		cfg.Models.Key = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_OPENAI_API_KEY")); value != "" {
		cfg.Models.OpenAIAPIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_OPENROUTER_API_KEY")); value != "" {
		cfg.Models.OpenRouterAPIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_BASE_URL")); value != "" {
		cfg.Models.Main.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_PROVIDER")); value != "" {
		cfg.Models.Summary.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_BASE_URL")); value != "" {
		cfg.Models.Summary.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_API_MODE")); value != "" {
		cfg.Models.Main.APIMode = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_API_MODE")); value != "" {
		cfg.Models.Summary.APIMode = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RPC_ADDRESS")); value != "" {
		cfg.RPC.Address = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RPC_PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.RPC.Port = port
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_MAX_ITERATIONS")); value != "" {
		if maxIterations, err := strconv.Atoi(value); err == nil {
			cfg.Session.MaxIterations = maxIterations
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_LOG_LEVEL")); value != "" {
		cfg.Log.Level = value
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_LOG_NO_COLOR"))); value != "" {
		cfg.Log.NoColor = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_DEBUG_REQUESTS"))); value != "" {
		cfg.Debug.Requests = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_INPUT"); ok {
		cfg.Safety.Input = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_OUTPUT"); ok {
		cfg.Safety.Output = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_PII"); ok {
		cfg.Safety.PII = new(value)
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_TRACE_ENABLED"))); value != "" {
		cfg.Trace.Enabled = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("HAND_TRACE_DISK_ENABLED"); ok {
		cfg.Trace.Disk.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_TRACE_DISK_DIR")); value != "" {
		cfg.Trace.Disk.Dir = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_TRACE_DATABASE_ENABLED"); ok {
		cfg.Trace.Database.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION")); value != "" {
		if maxEvents, err := strconv.Atoi(value); err == nil {
			cfg.Trace.Database.MaxEventsPerSession = maxEvents
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_TUI_THINKING_COMPOSER"); ok {
		cfg.TUI.ThinkingComposer = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_PROVIDER")); value != "" {
		cfg.Web.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_API_KEY")); value != "" {
		cfg.Web.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BASE_URL")); value != "" {
		cfg.Web.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_EXTRACT_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_EXTRACT_RESPONSE_BYTES")); value != "" {
		if bytes, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractResponseBytes = bytes
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_CACHE_TTL")); value != "" {
		cfg.Web.CacheTTL = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_WEB_BLOCKED_DOMAINS_ENABLED"); ok {
		cfg.Web.BlockedDomainsEnabled = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BLOCKED_DOMAINS")); value != "" {
		cfg.Web.BlockedDomains = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BLOCKED_DOMAIN_FILES")); value != "" {
		cfg.Web.BlockedDomainFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_ALLOWED_HOSTS")); value != "" {
		cfg.Web.NativeAllowedHosts = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_BLOCKED_HOSTS")); value != "" {
		cfg.Web.NativeBlockedHosts = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_ALLOWED_HOST_FILES")); value != "" {
		cfg.Web.NativeAllowedHostFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_BLOCKED_HOST_FILES")); value != "" {
		cfg.Web.NativeBlockedHostFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MIN_SUMMARIZE_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMinSummarizeChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MAX_SUMMARY_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChunkChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractRefusalThresholdChars = chars
		}
	}
	if cfg.Web.Provider == "" {
		switch {
		case strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_KEY")) != "" || strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_URL")) != "":
			cfg.Web.Provider = constants.WebProviderFirecrawl
		case strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderParallel
		case strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderTavily
		case strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderExa
		}
	}
	if cfg.Web.APIKey == "" {
		switch strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) {
		case constants.WebProviderFirecrawl:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_KEY"))
		case constants.WebProviderParallel:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY"))
		case constants.WebProviderTavily:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY"))
		case constants.WebProviderExa:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY"))
		}
	}
	if cfg.Web.BaseURL == "" && strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) == constants.WebProviderFirecrawl {
		cfg.Web.BaseURL = strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_URL"))
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RULES_FILES")); value != "" {
		cfg.Rules.Files = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_INSTRUCT")); value != "" {
		cfg.Session.Instruct = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_PLATFORM")); value != "" {
		cfg.Platform = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_FS_ROOTS")); value != "" {
		cfg.FS.Roots = splitAndTrimCSV(value)
	}

	if value, ok := parseOptionalBoolEnv("HAND_CAP_FS"); ok {
		cfg.Cap.Filesystem = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_NET"); ok {
		cfg.Cap.Network = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_EXEC"); ok {
		cfg.Cap.Exec = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_MEM"); ok {
		cfg.Cap.Memory = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_BROWSER"); ok {
		cfg.Cap.Browser = new(value)
	}

	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_ALLOW")); value != "" {
		cfg.Exec.Allow = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_ASK")); value != "" {
		cfg.Exec.Ask = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_DENY")); value != "" {
		cfg.Exec.Deny = splitAndTrimCSV(value)
	}

	if value := strings.TrimSpace(os.Getenv("HAND_STORAGE_BACKEND")); value != "" {
		cfg.Storage.Backend = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_DEFAULT_IDLE_EXPIRY")); value != "" {
		cfg.Session.DefaultIdleExpiry = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_ARCHIVE_RETENTION")); value != "" {
		cfg.Session.ArchiveRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_VECTOR_ENABLED"); ok {
		cfg.Search.Vector.Enabled = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_ENABLED"); ok {
		cfg.Memory.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROVIDER")); value != "" {
		cfg.Memory.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_BACKEND")); value != "" {
		cfg.Memory.Backend = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_PINNED_ENABLED"); ok {
		cfg.Memory.Pinned.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_RETRIEVAL_ENABLED"); ok {
		cfg.Memory.Retrieval.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_FLUSH_ENABLED"); ok {
		cfg.Memory.Flush.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_MAX_CALLS")); value != "" {
		if maxCalls, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Flush.MaxCalls = maxCalls
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_MAX_OUTPUT_TOKENS")); value != "" {
		if maxOutputTokens, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.Memory.Flush.MaxOutputTokens = maxOutputTokens
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_TIMEOUT")); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			cfg.Memory.Flush.Timeout = timeout
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PINNED_MAX_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxChars = maxChars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PINNED_MAX_ITEM_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxItemChars = maxChars
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_EPISODIC_ENABLED"); ok {
		cfg.Memory.Episodic.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_INTERVAL")); value != "" {
		cfg.Memory.Episodic.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_IDLE_AFTER")); value != "" {
		cfg.Memory.Episodic.IdleAfter = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MIN_MESSAGES")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MinMessages = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_WINDOW_SIZE")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.WindowSize = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOWS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindows = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowChars = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowTokens = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_RETRIES")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxRetries = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_REFLECTION_ENABLED"); ok {
		cfg.Memory.Reflection.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_INTERVAL")); value != "" {
		cfg.Memory.Reflection.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.Limit = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_RELATED_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.RelatedLimit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_PROMOTION_ENABLED"); ok {
		cfg.Memory.Promotion.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROMOTION_INTERVAL")); value != "" {
		cfg.Memory.Promotion.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROMOTION_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Promotion.Limit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_WRITE_ENABLED"); ok {
		cfg.Memory.Write.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_VECTOR_REQUIRED"); ok {
		cfg.Search.Vector.Required = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE")); value != "" {
		if batchSize, err := strconv.Atoi(value); err == nil {
			cfg.Search.Vector.RebuildBatchSize = batchSize
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_RERANKER_ENABLED"); ok {
		cfg.Reranker.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_ENABLE_RERANK"); ok {
		cfg.Search.EnableRerank = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_TYPE")); value != "" {
		cfg.Reranker.Type = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MODEL")); value != "" {
		cfg.Reranker.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_CANDIDATES")); value != "" {
		if maxCandidates, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidates = maxCandidates
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidateTextChars = maxChars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_OUTPUT_TOKENS")); value != "" {
		if maxTokens, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxOutputTokens = maxTokens
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_OVERRIDES")); value != "" {
		var overrides map[string]RerankerOverrideConfig
		if err := json.Unmarshal([]byte(value), &overrides); err == nil {
			cfg.Reranker.Overrides = overrides
		}
	}

	if value, ok := parseOptionalBoolEnv("HAND_COMPACTION_ENABLED"); ok {
		cfg.Compaction.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_COMPACTION_TRIGGER_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.TriggerPercent = percent
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_COMPACTION_WARN_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.WarnPercent = percent
		}
	}
}

// normalizeFields applies trimming and defaults except default model base URL resolution.
func (c *Config) normalizeFields() {
	if c == nil {
		return
	}

	c.Name = strings.TrimSpace(c.Name)
	c.Models.Main.Name = strings.TrimSpace(c.Models.Main.Name)
	c.Models.Summary.Name = strings.TrimSpace(c.Models.Summary.Name)
	c.Models.Main.Provider = strings.TrimSpace(strings.ToLower(c.Models.Main.Provider))
	c.Models.Embedding.Provider = strings.TrimSpace(strings.ToLower(c.Models.Embedding.Provider))
	c.Models.Embedding.Name = strings.TrimSpace(c.Models.Embedding.Name)
	c.Models.Key = strings.TrimSpace(c.Models.Key)
	c.Models.OpenAIAPIKey = strings.TrimSpace(c.Models.OpenAIAPIKey)
	c.Models.OpenRouterAPIKey = strings.TrimSpace(c.Models.OpenRouterAPIKey)
	c.Models.Main.BaseURL = strings.TrimSpace(c.Models.Main.BaseURL)
	c.Models.Summary.Provider = strings.TrimSpace(strings.ToLower(c.Models.Summary.Provider))
	c.Models.Summary.BaseURL = strings.TrimSpace(c.Models.Summary.BaseURL)
	c.Models.Main.APIMode = strings.TrimSpace(strings.ToLower(c.Models.Main.APIMode))
	c.Models.Summary.APIMode = strings.TrimSpace(strings.ToLower(c.Models.Summary.APIMode))
	c.Log.Level = strings.TrimSpace(strings.ToLower(c.Log.Level))
	c.Trace.Disk.Dir = strings.TrimSpace(c.Trace.Disk.Dir)
	c.Web.Provider = strings.TrimSpace(strings.ToLower(c.Web.Provider))
	c.Web.APIKey = strings.TrimSpace(c.Web.APIKey)
	c.Web.BaseURL = strings.TrimSpace(c.Web.BaseURL)
	c.Web.BlockedDomains = dedupeAndTrim(c.Web.BlockedDomains)
	c.Web.BlockedDomainFiles = dedupeAndTrim(c.Web.BlockedDomainFiles)
	c.Web.NativeAllowedHosts = dedupeAndTrim(c.Web.NativeAllowedHosts)
	c.Web.NativeBlockedHosts = dedupeAndTrim(c.Web.NativeBlockedHosts)
	c.Web.NativeAllowedHostFiles = dedupeAndTrim(c.Web.NativeAllowedHostFiles)
	c.Web.NativeBlockedHostFiles = dedupeAndTrim(c.Web.NativeBlockedHostFiles)
	c.Rules.Files = normalizeRulePaths(c.Rules.Files)
	c.Session.Instruct = strings.TrimSpace(c.Session.Instruct)
	c.Platform = strings.TrimSpace(strings.ToLower(c.Platform))
	c.FS.Roots = normalizeFSRoots(c.FS.Roots)
	c.Exec.Allow = dedupeAndTrim(c.Exec.Allow)
	c.Exec.Ask = dedupeAndTrim(c.Exec.Ask)
	c.Exec.Deny = dedupeAndTrim(c.Exec.Deny)
	c.Storage.Backend = strings.TrimSpace(strings.ToLower(c.Storage.Backend))
	c.Memory.Provider = strings.TrimSpace(strings.ToLower(c.Memory.Provider))
	c.Memory.Backend = strings.TrimSpace(strings.ToLower(c.Memory.Backend))
	c.Reranker.Type = strings.TrimSpace(strings.ToLower(c.Reranker.Type))
	c.Reranker.Model = strings.TrimSpace(c.Reranker.Model)
	c.Reranker.Overrides = normalizeRerankerOverrides(c.Reranker.Overrides)
	c.normalizePersonalities()

	if c.Models.Main.Name == "" {
		c.Models.Main.Name = constants.DefaultModel
	}
	if c.Models.Main.Stream == nil {
		c.Models.Main.Stream = new(constants.DefaultProfileModelStream)
	}
	if c.Models.Verify == nil {
		c.Models.Verify = new(constants.DefaultProfileModelVerify)
	}
	if c.Models.MaxRetries == nil {
		c.Models.MaxRetries = new(constants.DefaultModelMaxRetries)
	}
	if c.Models.Main.ContextLength <= 0 {
		c.Models.Main.ContextLength = constants.DefaultContextLength
	}

	if c.Models.Main.Provider == "" {
		c.Models.Main.Provider = constants.DefaultModelProvider
	}

	if c.Models.Main.APIMode == "" {
		c.Models.Main.APIMode = constants.DefaultModelAPIModeCompletions
	}

	if c.Log.Level == "" {
		c.Log.Level = constants.DefaultLogLevel
	}
	if c.RPC.Address == "" {
		c.RPC.Address = constants.DefaultRPCAddress
	}

	if c.RPC.Port == 0 {
		c.RPC.Port = constants.DefaultRPCPort
	}
	if c.Session.MaxIterations == 0 {
		c.Session.MaxIterations = constants.DefaultMaxIterations
	}
	if c.Trace.Disk.Dir == "" {
		c.Trace.Disk.Dir = datadir.DebugTraceDir()
	}
	if c.Trace.Disk.Enabled == nil {
		c.Trace.Disk.Enabled = new(constants.DefaultProfileTraceDiskEnabled)
	}
	if c.Trace.Database.Enabled == nil {
		c.Trace.Database.Enabled = new(constants.DefaultProfileTraceDatabaseEnabled)
	}
	if c.Trace.Database.MaxEventsPerSession <= 0 {
		c.Trace.Database.MaxEventsPerSession = constants.DefaultTraceMaxEventsPerSession
	}
	if c.TUI.ThinkingComposer == nil {
		c.TUI.ThinkingComposer = new(constants.DefaultTUIThinkingComposerEnabled)
	}
	if c.Safety.Input == nil {
		c.Safety.Input = new(constants.DefaultSafetyInputEnabled)
	}
	if c.Safety.Output == nil {
		c.Safety.Output = new(constants.DefaultSafetyOutputEnabled)
	}
	if c.Safety.PII == nil {
		c.Safety.PII = new(constants.DefaultSafetyPIIEnabled)
	}
	if c.Platform == "" {
		c.Platform = constants.DefaultPlatform
	}
	if c.Web.MaxCharPerResult <= 0 {
		c.Web.MaxCharPerResult = constants.DefaultWebMaxCharPerResult
	}
	if c.Web.MaxExtractCharPerResult <= 0 {
		c.Web.MaxExtractCharPerResult = constants.DefaultWebMaxExtractCharPerResult
	}
	if c.Web.MaxExtractResponseBytes <= 0 {
		c.Web.MaxExtractResponseBytes = constants.DefaultWebMaxExtractResponseBytes
	}
	if c.Web.CacheTTL < 0 {
		c.Web.CacheTTL = constants.DefaultWebCacheTTL
	}
	if c.Web.ExtractMinSummarizeChars <= 0 {
		c.Web.ExtractMinSummarizeChars = constants.DefaultWebExtractMinSummarizeChars
	}
	if c.Web.ExtractMaxSummaryChars <= 0 {
		c.Web.ExtractMaxSummaryChars = constants.DefaultWebExtractMaxSummaryChars
	}
	if c.Web.ExtractMaxSummaryChunkChars <= 0 {
		c.Web.ExtractMaxSummaryChunkChars = constants.DefaultWebExtractMaxSummaryChunkChars
	}
	if c.Web.ExtractRefusalThresholdChars <= 0 {
		c.Web.ExtractRefusalThresholdChars = constants.DefaultWebExtractRefusalThresholdChars
	}

	if c.Cap.Filesystem == nil {
		c.Cap.Filesystem = new(constants.DefaultProfileCapabilityFilesystem)
	}
	if c.Cap.Network == nil {
		c.Cap.Network = new(constants.DefaultProfileCapabilityNetwork)
	}
	if c.Cap.Exec == nil {
		c.Cap.Exec = new(constants.DefaultProfileCapabilityExec)
	}
	if c.Cap.Memory == nil {
		c.Cap.Memory = new(constants.DefaultProfileCapabilityMemory)
	}
	if c.Cap.Browser == nil {
		c.Cap.Browser = new(constants.DefaultProfileCapabilityBrowser)
	}

	if len(c.FS.Roots) == 0 {
		c.FS.Roots = getDefaultFSRoots()
	}

	if c.Storage.Backend == "" {
		c.Storage.Backend = constants.DefaultStorageBackend
	}

	if c.Session.DefaultIdleExpiry <= 0 {
		c.Session.DefaultIdleExpiry = constants.DefaultSessionIdleExpiry
	}
	if c.Session.ArchiveRetention <= 0 {
		c.Session.ArchiveRetention = constants.DefaultArchiveRetention
	}
	if c.Compaction.Enabled == nil {
		c.Compaction.Enabled = new(constants.DefaultProfileCompactionEnabled)
	}
	if c.Compaction.TriggerPercent <= 0 {
		c.Compaction.TriggerPercent = constants.DefaultCompactionTrigger
	}
	if c.Compaction.WarnPercent <= 0 {
		c.Compaction.WarnPercent = constants.DefaultCompactionWarn
	}
	if c.Compaction.RecentSessionTail == nil {
		c.Compaction.RecentSessionTail = new(constants.RecentSessionTail)
	}
	if c.Memory.Enabled == nil {
		c.Memory.Enabled = new(constants.DefaultProfileMemoryEnabled)
	}
	if c.Memory.Provider == "" {
		c.Memory.Provider = constants.MemoryProviderDefault
	}
	if c.Memory.Pinned.Enabled == nil {
		c.Memory.Pinned.Enabled = new(constants.DefaultProfileMemoryPinnedEnabled)
	}
	if c.Memory.Retrieval.Enabled == nil {
		c.Memory.Retrieval.Enabled = new(constants.DefaultProfileMemoryRetrievalEnabled)
	}
	if c.Memory.Flush.Enabled == nil {
		c.Memory.Flush.Enabled = new(constants.DefaultProfileMemoryFlushEnabled)
	}
	if c.Memory.Flush.MaxCalls <= 0 {
		c.Memory.Flush.MaxCalls = constants.DefaultProfileMemoryFlushMaxCalls
	}
	if c.Memory.Flush.MaxOutputTokens <= 0 {
		c.Memory.Flush.MaxOutputTokens = constants.DefaultProfileMemoryFlushMaxOutputTokens
	}
	if c.Memory.Flush.Timeout <= 0 {
		c.Memory.Flush.Timeout = constants.DefaultProfileMemoryFlushTimeout
	}
	if c.Memory.Episodic.Enabled == nil {
		c.Memory.Episodic.Enabled = new(constants.DefaultMemoryEpisodicEnabled)
	}
	if c.Memory.Reflection.Enabled == nil {
		c.Memory.Reflection.Enabled = new(constants.DefaultMemoryReflectionEnabled)
	}
	if c.Memory.Promotion.Enabled == nil {
		c.Memory.Promotion.Enabled = new(constants.DefaultProfileMemoryPromotionEnabled)
	}
	if c.Memory.Write.Enabled == nil {
		c.Memory.Write.Enabled = new(constants.DefaultProfileMemoryWriteEnabled)
	}

}

func normalizeRerankerOverrides(overrides map[string]RerankerOverrideConfig) map[string]RerankerOverrideConfig {
	if len(overrides) == 0 {
		return nil
	}

	normalized := make(map[string]RerankerOverrideConfig, len(overrides))
	for key, override := range overrides {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}

		override.Type = strings.TrimSpace(strings.ToLower(override.Type))
		override.Model = strings.TrimSpace(override.Model)
		normalized[key] = override
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func (c *Config) normalizePersonalities() {
	if c == nil || len(c.Personalities) == 0 {
		return
	}

	normalized := make(map[string]PersonalityConfig, len(c.Personalities))
	for name, personality := range c.Personalities {
		name = strings.ToLower(strings.TrimSpace(name))
		personality.Soul = strings.TrimSpace(personality.Soul)
		personality.Instruct = strings.TrimSpace(personality.Instruct)
		personality.State = strings.TrimSpace(strings.ToLower(personality.State))
		if personality.State == "" {
			personality.State = personalityStateShared
		}
		personality.Tools.Memory = strings.TrimSpace(strings.ToLower(personality.Tools.Memory))
		personality.Model.Name = strings.TrimSpace(personality.Model.Name)
		personality.Model.Provider = strings.TrimSpace(strings.ToLower(personality.Model.Provider))
		personality.Model.APIMode = strings.TrimSpace(strings.ToLower(personality.Model.APIMode))
		personality.Model.BaseURL = strings.TrimSpace(personality.Model.BaseURL)
		normalized[name] = personality
	}
	c.Personalities = normalized
}

func (c *Config) applyDefaultModelBaseURL() {
	if c == nil {
		return
	}

	mapped := getDefaultBaseURLForProvider(c.Models.Main.Provider, c.Models.Main.APIMode)
	if mapped == "" {
		return
	}
	if c.Models.Main.BaseURL == "" || isProviderDefaultBaseURL(c.Models.Main.BaseURL) {
		c.Models.Main.BaseURL = mapped
	}
}

func isProviderDefaultBaseURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	for _, modes := range providerDefaultBaseURLs {
		for _, baseURL := range modes {
			if value == strings.TrimSpace(baseURL) {
				return true
			}
		}
	}

	return false
}

// Normalize trims fields, applies defaults, and resolves default model base URL when unset.
func (c *Config) Normalize() {
	if c == nil {
		return
	}

	c.normalizeFields()
	c.applyDefaultModelBaseURL()
}

func getDefaultBaseURLForProvider(provider, apiMode string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	apiMode = strings.TrimSpace(strings.ToLower(apiMode))
	if apiMode == "" {
		apiMode = constants.DefaultModelAPIModeCompletions
	}

	modes, ok := providerDefaultBaseURLs[provider]
	if !ok {
		return ""
	}

	u, ok := modes[apiMode]
	if !ok {
		return ""
	}

	return u
}

func (c *Config) VerifyEnabled() bool {
	if c == nil {
		return true
	}

	return getBoolValueDefault(c.Models.Verify, true)
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

func (c *Config) SummaryModelAPIModeEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.Models.Summary.APIMode != "" {
		return c.Models.Summary.APIMode
	}

	return c.Models.Main.APIMode
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
	sumMode := c.SummaryModelAPIModeEffective()
	mainMode := c.Models.Main.APIMode

	if sum == main && sumMode == mainMode {
		return c.Models.Main.BaseURL
	}

	if u := strings.TrimSpace(c.Models.Summary.BaseURL); u != "" {
		return u
	}

	return getDefaultBaseURLForProvider(sum, sumMode)
}

func (c *Config) ResolveSummaryModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	prov := c.SummaryProviderEffective()
	auth := ModelAuth{
		Provider: prov,
		BaseURL:  c.summaryModelBaseURLEffective(),
	}

	auth.APIKey = c.resolveAPIKeyForProvider(prov)
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

// ModelAuthEqual reports whether two auth values describe the same provider, endpoint, and key.
func ModelAuthEqual(a, b ModelAuth) bool {
	return strings.TrimSpace(strings.ToLower(a.Provider)) == strings.TrimSpace(strings.ToLower(b.Provider)) &&
		strings.TrimSpace(a.BaseURL) == strings.TrimSpace(b.BaseURL) &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey)
}

func splitAndTrimCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}

	return values
}

func dedupeAndTrim(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}

	return out
}

func normalizeFSRoots(values []string) []string {
	values = dedupeAndTrim(values)
	if len(values) == 0 {
		return nil
	}

	roots := make([]string, 0, len(values))
	for _, value := range values {
		if filepath.IsAbs(value) {
			roots = append(roots, filepath.Clean(value))
			continue
		}

		cwd, err := getwd()
		if err != nil {
			roots = append(roots, filepath.Clean(value))
			continue
		}
		roots = append(roots, filepath.Clean(filepath.Join(cwd, value)))
	}

	return dedupeAndTrim(roots)
}

func getPathsFromBase(values []string, baseDir string) []string {
	values = dedupeAndTrim(values)
	if len(values) == 0 {
		return nil
	}

	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return values
	}

	resolved := make([]string, 0, len(values))
	for _, value := range values {
		if filepath.IsAbs(value) {
			resolved = append(resolved, value)
			continue
		}
		resolved = append(resolved, filepath.Join(baseDir, value))
	}

	return resolved
}

func getWorkingDirectory() string {
	cwd, err := getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func getDefaultFSRoots() []string {
	cwd, err := getwd()
	if err != nil {
		return []string{"."}
	}
	return []string{filepath.Clean(cwd)}
}

func parseOptionalBoolEnv(key string) (bool, bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return false, false
	}
	return value == "1" || value == "true" || value == "yes", true
}

func parseDurationOrZero(value string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func getBoolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func getBoolValueDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is required")
	}

	if err := c.validatePersonalityNames(); err != nil {
		return err
	}

	c.Normalize()

	if err := c.validatePersonalities(); err != nil {
		return err
	}

	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required; set HAND_NAME, provide it in config, or use --name")
	}

	if !isValidModelSlug(c.Models.Main.Name) {
		return errors.New("model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
	}

	if c.Models.Summary.Name != "" && !isValidModelSlug(c.Models.Summary.Name) {
		return errors.New("summary model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
	}

	if _, ok := providerDefaultBaseURLs[strings.TrimSpace(strings.ToLower(c.Models.Main.Provider))]; !ok {
		return errors.New("model provider must be one of: openai, openrouter")
	}

	if c.Models.Summary.Provider != "" {
		if _, ok := providerDefaultBaseURLs[c.Models.Summary.Provider]; !ok {
			return errors.New("summary model provider must be one of: openai, openrouter")
		}
	}

	if err := c.validateRerankerSettings(); err != nil {
		return err
	}

	if err := c.validateSearchVectorSettings(); err != nil {
		return err
	}

	auth, err := c.ResolveModelAuth()
	if err != nil {
		return err
	}

	summaryAuth, err := c.ResolveSummaryModelAuth()
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.RPC.Address) == "" {
		return errors.New("rpc address is required; set HAND_RPC_ADDRESS, provide it in config, or use --rpc.address")
	}

	if c.RPC.Port < 0 {
		return errors.New("rpc port must be non-negative; set HAND_RPC_PORT, provide it in config, or use --rpc.port")
	}

	if c.Session.MaxIterations <= 0 {
		return errors.New("max iterations must be greater than zero; set HAND_SESSION_MAX_ITERATIONS, provide it in config, " +
			"or use --max-iterations")
	}
	if c.ModelMaxRetriesEffective() < 0 {
		return errors.New("model max retries must be greater than or equal to zero; use --model.max-retries")
	}

	switch c.Models.Main.APIMode {
	case constants.DefaultModelAPIModeCompletions:
	case "responses":
	default:
		return errors.New("model api mode must be one of: completions, responses; use --model.api-mode")
	}

	if c.Models.Summary.APIMode != "" {
		switch c.Models.Summary.APIMode {
		case constants.DefaultModelAPIModeCompletions:
		case "responses":
		default:
			return errors.New("summary model api mode must be one of: completions, responses; " +
				"use --model.summary-api-mode")
		}
	}

	if c.Storage.Backend != "memory" && c.Storage.Backend != "sqlite" {
		return errors.New("storage backend must be one of: memory, sqlite")
	}
	if c.Memory.Backend != "" && c.Memory.Backend != "memory" && c.Memory.Backend != "sqlite" {
		return errors.New("memory backend must be one of: memory, sqlite")
	}
	if c.Compaction.TriggerPercent >= 1 {
		return errors.New("compaction trigger percent must be greater than zero and less than one")
	}
	if c.Compaction.WarnPercent >= 1 {
		return errors.New("compaction warn percent must be greater than zero and less than one")
	}
	if c.Compaction.WarnPercent < c.Compaction.TriggerPercent {
		return errors.New("compaction warn percent must be greater than or equal to compaction trigger percent")
	}
	if c.Compaction.RecentSessionTail != nil && *c.Compaction.RecentSessionTail < 0 {
		return errors.New("compaction recent session tail must be greater than or equal to zero")
	}

	if c.VerifyEnabled() {
		verifySlots := []modelVerifySlot{{field: "models.main.name", slug: c.Models.Main.Name}}
		if c.Models.Summary.Name != "" && c.Models.Summary.Name != c.Models.Main.Name {
			verifySlots = append(verifySlots, modelVerifySlot{field: "models.summary.name", slug: c.Models.Summary.Name})
		}

		for _, slot := range verifySlots {
			slotAuth := auth
			if slot.field == "models.summary.name" {
				slotAuth = summaryAuth
			}
			verifyCfg := *c
			verifyCfg.Models.Main.Name = slot.slug
			meta, err := resolveModelMeta(context.Background(), &verifyCfg, slotAuth)
			if err != nil {
				return fmt.Errorf("%s: %w", slot.field, err)
			}
			if !meta.Exists {
				return fmt.Errorf("%s: %w", slot.field, newUnknownModelError(auth.Provider, slot.slug))
			}
		}
	}

	switch strings.TrimSpace(strings.ToLower(c.Log.Level)) {
	case "", "debug", "info", "warn", "error":
		return nil
	default:
		return errors.New("log level must be one of debug, info, warn, or error; use --log.level")
	}
}

func (c *Config) validatePersonalityNames() error {
	if c == nil || len(c.Personalities) == 0 {
		return nil
	}

	seen := make(map[string]string, len(c.Personalities))
	names := make([]string, 0, len(c.Personalities))
	for name := range c.Personalities {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if !validPersonalityName.MatchString(trimmed) {
			return fmt.Errorf("invalid personality name %q: must match %s", trimmed, personalityNamePattern)
		}

		normalized := strings.ToLower(trimmed)
		if existing, ok := seen[normalized]; ok {
			return fmt.Errorf("duplicate personality name %q conflicts with %q", trimmed, existing)
		}
		seen[normalized] = trimmed
	}

	return nil
}

func (c *Config) validatePersonalities() error {
	if c == nil {
		return nil
	}

	for name, personality := range c.Personalities {
		if err := validatePersonalityConfig(name, personality); err != nil {
			return err
		}
	}

	return nil
}

func validatePersonalityConfig(name string, personality PersonalityConfig) error {
	switch personality.State {
	case personalityStateShared, personalityStateIsolated, personalityStateReadonly:
	default:
		return fmt.Errorf("personalities.%s.state must be one of: shared, isolated, readonly", name)
	}

	switch personality.Tools.Memory {
	case "", personalityToolMemoryNone, personalityToolMemoryRead, personalityToolMemoryWrite:
	default:
		return fmt.Errorf("personalities.%s.tools.mem must be one of: none, read, write", name)
	}

	if personality.MaxIterations < 0 {
		return fmt.Errorf("personalities.%s.maxIterations must be non-negative", name)
	}

	if personality.Model.Name != "" && !isValidModelSlug(personality.Model.Name) {
		return fmt.Errorf("personalities.%s.model.name must use the format <owner>/<name>", name)
	}
	if personality.Model.Provider != "" {
		if _, ok := providerDefaultBaseURLs[personality.Model.Provider]; !ok {
			return fmt.Errorf("personalities.%s.model.provider must be one of: openai, openrouter", name)
		}
	}
	switch personality.Model.APIMode {
	case "", constants.DefaultModelAPIModeCompletions, constants.DefaultModelAPIModeResponses:
	default:
		return fmt.Errorf("personalities.%s.model.apiMode must be one of: completions, responses", name)
	}

	return nil
}

func (c *Config) validateSearchVectorSettings() error {
	if !c.Search.Vector.Enabled {
		return nil
	}
	provider := c.ModelEmbeddingProviderEffective()
	if _, ok := providerDefaultBaseURLs[provider]; !ok {
		return errors.New("embedding provider must be one of: openai, openrouter")
	}
	if c.Models.Embedding.Name == "" {
		return errors.New("embedding model is required")
	}
	if c.Search.Vector.RebuildBatchSize < 0 {
		return errors.New("vector rebuild batch size must be non-negative")
	}
	auth, err := c.ResolveEmbeddingModelAuth()
	if err != nil {
		return err
	}
	if err := c.validateEmbeddingModelExists(context.Background(), auth); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateRerankerSettings() error {
	if err := validateRerankerType(c.RerankerEffective()); err != nil {
		return err
	}
	if c.Reranker.MaxCandidates < 0 {
		return errors.New("reranker max candidates must be non-negative")
	}
	if c.Reranker.MaxCandidateTextChars < 0 {
		return errors.New("reranker max candidate text chars must be non-negative")
	}
	if c.Reranker.MaxOutputTokens < 0 {
		return errors.New("reranker max output tokens must be non-negative")
	}
	for useCase, override := range c.Reranker.Overrides {
		if err := c.validateRerankerOverride(useCase, override); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) validateRerankerOverride(useCase string, override RerankerOverrideConfig) error {
	useCase = strings.TrimSpace(useCase)
	if useCase == "" {
		return errors.New("reranker override use case is required")
	}
	if strings.TrimSpace(override.Type) != "" {
		if err := validateRerankerType(override.Type); err != nil {
			return fmt.Errorf("reranker override %q: %w", useCase, err)
		}
	}
	if override.MaxCandidates != nil && *override.MaxCandidates < 0 {
		return fmt.Errorf("reranker override %q max candidates must be non-negative", useCase)
	}
	if override.MaxCandidateTextChars != nil && *override.MaxCandidateTextChars < 0 {
		return fmt.Errorf("reranker override %q max candidate text chars must be non-negative", useCase)
	}
	if override.MaxOutputTokens != nil && *override.MaxOutputTokens < 0 {
		return fmt.Errorf("reranker override %q max output tokens must be non-negative", useCase)
	}

	return nil
}

func validateRerankerType(rerankerType string) error {
	switch strings.TrimSpace(strings.ToLower(rerankerType)) {
	case constants.RerankerDeterministic, constants.RerankerNoop, constants.RerankerLLM:
		return nil
	default:
		return errors.New("reranker type must be one of: deterministic, noop, llm")
	}
}

func (c *Config) validateEmbeddingModelExists(ctx context.Context, auth ModelAuth) error {
	if !c.VerifyEnabled() {
		return nil
	}

	var (
		meta ModelMetadata
		err  error
	)
	switch strings.TrimSpace(strings.ToLower(auth.Provider)) {
	case "openrouter":
		meta, err = fetchOpenRouterModelEndpoints(
			ctx,
			getDefaultBaseURLForProvider("openrouter", constants.DefaultModelAPIModeCompletions),
			c.Models.Embedding.Name,
			auth.APIKey,
		)
	case "openai":
		meta, err = fetchOpenAIModelExists(ctx, c.Models.Embedding.Name)
	default:
		return fmt.Errorf("models.embedding.name: unsupported model provider %q", auth.Provider)
	}
	if err != nil {
		return fmt.Errorf("models.embedding.name: %w", err)
	}
	if !meta.Exists {
		return fmt.Errorf("models.embedding.name: %w", newUnknownModelError(auth.Provider, c.Models.Embedding.Name))
	}

	return nil
}

func (c *Config) ResolveEmbeddingModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	provider := c.ModelEmbeddingProviderEffective()
	if _, ok := providerDefaultBaseURLs[provider]; !ok {
		return ModelAuth{}, errors.New("embedding provider must be one of: openai, openrouter")
	}

	auth := ModelAuth{
		Provider: provider,
		BaseURL:  getDefaultBaseURLForProvider(provider, "embeddings"),
		APIKey:   c.resolveAPIKeyForProvider(provider),
	}
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("embedding API key is required")
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
		BaseURL:  c.Models.Main.BaseURL,
	}

	auth.APIKey = c.resolveAPIKeyForProvider(c.Models.Main.Provider)
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

func (c *Config) resolveAPIKeyForProvider(provider string) string {
	switch provider {
	case "openrouter":
		return getFirstNonEmpty(c.Models.OpenRouterAPIKey, c.Models.Key)
	case "openai":
		return getFirstNonEmpty(c.Models.OpenAIAPIKey, c.Models.Key)
	default:
		return c.Models.Key
	}
}

func getFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeRulePaths(files []string) []string {
	normalized := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))

	for _, file := range files {
		path := strings.TrimSpace(file)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}

	return normalized
}

// modelVerifySlot pairs a config field label (YAML keys) with the slug sent to resolveModelMeta.
type modelVerifySlot struct {
	field string
	slug  string
}

func isValidModelSlug(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	owner, name, ok := strings.Cut(value, "/")
	if !ok {
		return false
	}

	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	return owner != "" && name != "" && !strings.Contains(name, "/")
}

func applyProviderModelMetadata(ctx context.Context, cfg *Config, requestedContextLength int) {
	if cfg == nil {
		return
	}
	if !cfg.VerifyEnabled() {
		return
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		return
	}

	meta, err := resolveModelMeta(ctx, cfg, auth)
	if err != nil || !meta.Exists || meta.ContextLength <= 0 {
		return
	}

	if requestedContextLength <= 0 || requestedContextLength > meta.ContextLength {
		cfg.Models.Main.ContextLength = meta.ContextLength
	}
}

func fetchModelMetadataFromProvider(ctx context.Context, cfg *Config, auth ModelAuth) (ModelMetadata, error) {
	if cfg == nil {
		return ModelMetadata{}, nil
	}

	return fetchModelMetadataForSlug(ctx, auth, cfg.Models.Main.Name)
}

func fetchModelMetadataForSlug(ctx context.Context, auth ModelAuth, slug string) (ModelMetadata, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ModelMetadata{}, nil
	}

	switch strings.TrimSpace(strings.ToLower(auth.Provider)) {
	case "openrouter":
		return fetchOpenRouterModelMetadata(ctx, auth.BaseURL, slug, auth.APIKey)
	case "openai":
		return fetchOpenAIModelMetadata(ctx, slug)
	default:
		return ModelMetadata{}, fmt.Errorf("unsupported model provider %q", auth.Provider)
	}
}

func fetchOpenRouterModelMetadata(ctx context.Context, baseURL, model, apiKey string) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = getDefaultBaseURLForProvider("openrouter", constants.DefaultModelAPIModeCompletions)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return ModelMetadata{}, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openrouter model %q: "+
			"openrouter models lookup returned %s", model, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ModelMetadata{}, err
	}

	type openRouterModel struct {
		ID            string `json:"id"`
		ContextLength int    `json:"context_length"`
	}

	var wrapped struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return ModelMetadata{}, err
	}

	for _, item := range wrapped.Data {
		if strings.TrimSpace(item.ID) == model {
			return ModelMetadata{
				Exists:        true,
				ContextLength: item.ContextLength,
			}, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenRouterModelEndpoints(ctx context.Context, baseURL, model, apiKey string) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = getDefaultBaseURLForProvider("openrouter", constants.DefaultModelAPIModeCompletions)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models/"+getOpenRouterModelPath(model)+"/endpoints",
		nil,
	)
	if err != nil {
		return ModelMetadata{}, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ModelMetadata{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openrouter model %q: "+
			"openrouter model endpoints lookup returned %s", model, resp.Status)
	}

	return ModelMetadata{Exists: true}, nil
}

func getOpenRouterModelPath(model string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(model), "/"), "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
}

func fetchOpenAIModelMetadata(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range getOpenAIModelDocSlugs(model) {
		meta, err := fetchOpenAIModelMetadataPage(ctx, candidate, true)
		if err != nil {
			return ModelMetadata{}, err
		}
		if meta.Exists {
			return meta, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenAIModelExists(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range getOpenAIModelDocSlugs(model) {
		meta, err := fetchOpenAIModelMetadataPage(ctx, candidate, false)
		if err != nil {
			return ModelMetadata{}, err
		}
		if meta.Exists {
			return meta, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenAIModelMetadataPage(ctx context.Context, model string, requireContextWindow bool) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(modelDocsBaseURL, "/")+"/"+model, nil)
	if err != nil {
		return ModelMetadata{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ModelMetadata{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openai model %q: openai model docs lookup returned %s", model, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ModelMetadata{}, err
	}
	if isOpenAIModelDocsPageNotFound(body) {
		return ModelMetadata{}, nil
	}

	match := contextWindowPatternOAI.FindStringSubmatch(string(body))
	if len(match) != 2 {
		if !requireContextWindow {
			return ModelMetadata{Exists: true}, nil
		}

		return ModelMetadata{}, nil
	}

	contextLength, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return ModelMetadata{}, err
	}

	return ModelMetadata{
		Exists:        true,
		ContextLength: contextLength,
	}, nil
}

func isOpenAIModelDocsPageNotFound(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "<title>Page not found | OpenAI API</title>") ||
		strings.Contains(text, `name="title" content="Page not found | OpenAI API"`)
}

func newUnknownModelError(provider, model string) error {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "openrouter":
		return fmt.Errorf("model %q is not available on openrouter", model)
	default:
		return fmt.Errorf("model %q is not available on openai", model)
	}
}

func getOpenAIModelDocSlugs(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}

	if prefix, suffix, ok := strings.Cut(model, "/"); ok && strings.EqualFold(prefix, "openai") {
		model = strings.TrimSpace(suffix)
	}

	candidates := []string{model}
	if base := trimOpenAISnapshotSuffix(model); base != model {
		candidates = append(candidates, base)
	}

	return dedupeAndTrim(candidates)
}

func trimOpenAISnapshotSuffix(model string) string {
	parts := strings.Split(strings.TrimSpace(model), "-")
	if len(parts) < 4 {
		return model
	}

	last := len(parts) - 1
	if len(parts[last-2]) != 4 || len(parts[last-1]) != 2 || len(parts[last]) != 2 {
		return model
	}

	if _, err := strconv.Atoi(parts[last-2]); err != nil {
		return model
	}
	if _, err := strconv.Atoi(parts[last-1]); err != nil {
		return model
	}
	if _, err := strconv.Atoi(parts[last]); err != nil {
		return model
	}

	return strings.Join(parts[:last-2], "-")
}

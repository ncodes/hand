package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	Name       string           `yaml:"name"`
	Platform   string           `yaml:"platform"`
	Models     ModelsConfig     `yaml:"models"`
	RPC        RPCConfig        `yaml:"rpc"`
	FS         FSConfig         `yaml:"fs"`
	Exec       ExecConfig       `yaml:"exec"`
	Storage    StorageConfig    `yaml:"storage"`
	Session    SessionConfig    `yaml:"session"`
	Search     SearchConfig     `yaml:"search"`
	Memory     MemoryConfig     `yaml:"memory"`
	Reranker   RerankerConfig   `yaml:"reranker"`
	Compaction CompactionConfig `yaml:"compaction"`
	Cap        CapConfig        `yaml:"cap"`
	Log        LogConfig        `yaml:"log"`
	Debug      DebugConfig      `yaml:"debug"`
	Web        WebConfig        `yaml:"web"`
	Rules      RulesConfig      `yaml:"rules"`
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
	Roots []string `yaml:"roots"`
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
	Enabled  *bool              `yaml:"enabled"`
	Provider string             `yaml:"provider"`
	Backend  string             `yaml:"backend"`
	Pinned   PinnedMemoryConfig `yaml:"pinned"`
}

type PinnedMemoryConfig struct {
	Enabled      *bool `yaml:"enabled"`
	MaxChars     int   `yaml:"maxChars"`
	MaxItemChars int   `yaml:"maxItemChars"`
}

type RerankerConfig struct {
	Enabled               *bool  `yaml:"enabled"`
	Type                  string `yaml:"type"`
	Model                 string `yaml:"model"`
	MaxCandidates         int    `yaml:"maxCandidates"`
	MaxCandidateTextChars int    `yaml:"maxCandidateTextChars"`
	MaxOutputTokens       int    `yaml:"maxOutputTokens"`
}

type CompactionConfig struct {
	Enabled        *bool   `yaml:"enabled"`
	TriggerPercent float64 `yaml:"triggerPercent"`
	WarnPercent    float64 `yaml:"warnPercent"`
}

type CapConfig struct {
	Filesystem *bool `yaml:"fs"`
	Network    *bool `yaml:"net"`
	Exec       *bool `yaml:"exec"`
	Memory     *bool `yaml:"mem"`
	Browser    *bool `yaml:"browser"`
}

type LogConfig struct {
	Level   string `yaml:"level"`
	NoColor bool   `yaml:"noColor"`
}

type DebugConfig struct {
	Requests bool   `yaml:"requests"`
	Traces   bool   `yaml:"traces"`
	TraceDir string `yaml:"traceDir"`
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
	resolveModelMeta        = resolveModelMetadataFromProvider
	providerDefaultBaseURLs = map[string]map[string]string{
		constants.ModelProviderOpenRouter: {
			DefaultModelAPIMode: "https://openrouter.ai/api/v1",
			"responses":         "https://openrouter.ai/api/v1/responses",
			"embeddings":        "https://openrouter.ai/api/v1/embeddings",
		},
		constants.ModelProviderOpenAI: {
			DefaultModelAPIMode: "https://api.openai.com/v1",
			"responses":         "https://api.openai.com/v1",
			"embeddings":        "https://api.openai.com/v1/embeddings",
		},
	}
)

var contextWindowPatternOAI = regexp.MustCompile(`([0-9][0-9,]*)(?:\s|<!--[^>]*-->)+context window`)

const (
	webProviderFirecrawl = "firecrawl"
	webProviderParallel  = "parallel"
	webProviderTavily    = "tavily"
	webProviderExa       = "exa"

	defaultModel                                         = constants.DefaultModel
	defaultContextLength                                 = constants.DefaultContextLength
	defaultModelProvider                                 = constants.DefaultModelProvider
	DefaultModelAPIMode                                  = constants.DefaultModelAPIMode
	DefaultMaxIterations                                 = constants.DefaultMaxIterations
	DefaultWebMaxCharPerResult                           = constants.DefaultWebMaxCharPerResult
	DefaultWebMaxExtractCharPerResult                    = constants.DefaultWebMaxExtractCharPerResult
	DefaultWebMaxExtractResponseBytes                    = constants.DefaultWebMaxExtractResponseBytes
	DefaultWebCacheTTL                     time.Duration = constants.DefaultWebCacheTTL
	DefaultWebExtractMinSummarizeChars                   = constants.DefaultWebExtractMinSummarizeChars
	DefaultWebExtractMaxSummaryChars                     = constants.DefaultWebExtractMaxSummaryChars
	DefaultWebExtractMaxSummaryChunkChars                = constants.DefaultWebExtractMaxSummaryChunkChars
	DefaultWebExtractRefusalThresholdChars               = constants.DefaultWebExtractRefusalThresholdChars
	DefaultModelMaxRetries                               = constants.DefaultModelMaxRetries
	defaultMaxIterations                                 = constants.DefaultMaxIterations
)

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
		return &Config{
			Models: ModelsConfig{
				Main: MainModelConfig{
					Name:          defaultModel,
					Stream:        new(true),
					ContextLength: defaultContextLength,
					APIMode:       DefaultModelAPIMode,
				},
				Verify:     new(true),
				MaxRetries: new(DefaultModelMaxRetries),
			},
			Session: SessionConfig{
				MaxIterations:     defaultMaxIterations,
				DefaultIdleExpiry: constants.DefaultSessionIdleExpiry,
				ArchiveRetention:  constants.DefaultArchiveRetention,
			},
			Log: LogConfig{
				Level: constants.DefaultLogLevel,
			},
			Debug: DebugConfig{
				TraceDir: datadir.DebugTraceDir(),
			},
			Web: WebConfig{
				MaxCharPerResult:             DefaultWebMaxCharPerResult,
				MaxExtractCharPerResult:      DefaultWebMaxExtractCharPerResult,
				MaxExtractResponseBytes:      DefaultWebMaxExtractResponseBytes,
				CacheTTL:                     DefaultWebCacheTTL,
				ExtractMinSummarizeChars:     DefaultWebExtractMinSummarizeChars,
				ExtractMaxSummaryChars:       DefaultWebExtractMaxSummaryChars,
				ExtractMaxSummaryChunkChars:  DefaultWebExtractMaxSummaryChunkChars,
				ExtractRefusalThresholdChars: DefaultWebExtractRefusalThresholdChars,
			},
			Platform: constants.DefaultPlatform,
			Cap: CapConfig{
				Filesystem: new(true),
				Network:    new(true),
				Exec:       new(true),
				Memory:     new(true),
				Browser:    new(false),
			},
			FS: FSConfig{
				Roots: defaultFSRoots(),
			},
			Storage: StorageConfig{
				Backend: constants.DefaultStorageBackend,
			},
			Compaction: CompactionConfig{
				Enabled:        new(true),
				TriggerPercent: constants.DefaultCompactionTrigger,
				WarnPercent:    constants.DefaultCompactionWarn,
			},
			Memory: MemoryConfig{
				Enabled:  new(true),
				Provider: constants.MemoryProviderDefault,
			},
		}
	}

	return globalConfig
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
			return &Config{}, nil
		}

		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var cfg Config
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

	c.FS.Roots = resolvePathsFromBase(c.FS.Roots, baseDir)
	c.Web.BlockedDomainFiles = resolvePathsFromBase(c.Web.BlockedDomainFiles, baseDir)
	c.Web.NativeAllowedHostFiles = resolvePathsFromBase(c.Web.NativeAllowedHostFiles, baseDir)
	c.Web.NativeBlockedHostFiles = resolvePathsFromBase(c.Web.NativeBlockedHostFiles, baseDir)
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
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_DEBUG_TRACES"))); value != "" {
		cfg.Debug.Traces = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(os.Getenv("HAND_DEBUG_TRACE_DIR")); value != "" {
		cfg.Debug.TraceDir = value
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
			cfg.Web.Provider = webProviderFirecrawl
		case strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY")) != "":
			cfg.Web.Provider = webProviderParallel
		case strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY")) != "":
			cfg.Web.Provider = webProviderTavily
		case strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY")) != "":
			cfg.Web.Provider = webProviderExa
		}
	}
	if cfg.Web.APIKey == "" {
		switch strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) {
		case webProviderFirecrawl:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_KEY"))
		case webProviderParallel:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY"))
		case webProviderTavily:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY"))
		case webProviderExa:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY"))
		}
	}
	if cfg.Web.BaseURL == "" && strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) == webProviderFirecrawl {
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
	c.Debug.TraceDir = strings.TrimSpace(c.Debug.TraceDir)
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

	if c.Models.Main.Name == "" {
		c.Models.Main.Name = defaultModel
	}
	if c.Models.Main.Stream == nil {
		c.Models.Main.Stream = new(true)
	}
	if c.Models.Verify == nil {
		c.Models.Verify = new(true)
	}
	if c.Models.MaxRetries == nil {
		c.Models.MaxRetries = new(DefaultModelMaxRetries)
	}
	if c.Models.Main.ContextLength <= 0 {
		c.Models.Main.ContextLength = defaultContextLength
	}

	if c.Models.Main.Provider == "" {
		c.Models.Main.Provider = defaultModelProvider
	}

	if c.Models.Main.APIMode == "" {
		c.Models.Main.APIMode = DefaultModelAPIMode
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
		c.Session.MaxIterations = defaultMaxIterations
	}
	if c.Debug.TraceDir == "" {
		c.Debug.TraceDir = datadir.DebugTraceDir()
	}
	if c.Platform == "" {
		c.Platform = constants.DefaultPlatform
	}
	if c.Web.MaxCharPerResult <= 0 {
		c.Web.MaxCharPerResult = DefaultWebMaxCharPerResult
	}
	if c.Web.MaxExtractCharPerResult <= 0 {
		c.Web.MaxExtractCharPerResult = DefaultWebMaxExtractCharPerResult
	}
	if c.Web.MaxExtractResponseBytes <= 0 {
		c.Web.MaxExtractResponseBytes = DefaultWebMaxExtractResponseBytes
	}
	if c.Web.CacheTTL < 0 {
		c.Web.CacheTTL = DefaultWebCacheTTL
	}
	if c.Web.ExtractMinSummarizeChars <= 0 {
		c.Web.ExtractMinSummarizeChars = DefaultWebExtractMinSummarizeChars
	}
	if c.Web.ExtractMaxSummaryChars <= 0 {
		c.Web.ExtractMaxSummaryChars = DefaultWebExtractMaxSummaryChars
	}
	if c.Web.ExtractMaxSummaryChunkChars <= 0 {
		c.Web.ExtractMaxSummaryChunkChars = DefaultWebExtractMaxSummaryChunkChars
	}
	if c.Web.ExtractRefusalThresholdChars <= 0 {
		c.Web.ExtractRefusalThresholdChars = DefaultWebExtractRefusalThresholdChars
	}

	if c.Cap.Filesystem == nil {
		c.Cap.Filesystem = new(true)
	}
	if c.Cap.Network == nil {
		c.Cap.Network = new(true)
	}
	if c.Cap.Exec == nil {
		c.Cap.Exec = new(true)
	}
	if c.Cap.Memory == nil {
		c.Cap.Memory = new(true)
	}
	if c.Cap.Browser == nil {
		c.Cap.Browser = new(false)
	}

	if len(c.FS.Roots) == 0 {
		c.FS.Roots = defaultFSRoots()
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
		c.Compaction.Enabled = new(true)
	}
	if c.Compaction.TriggerPercent <= 0 {
		c.Compaction.TriggerPercent = constants.DefaultCompactionTrigger
	}
	if c.Compaction.WarnPercent <= 0 {
		c.Compaction.WarnPercent = constants.DefaultCompactionWarn
	}
	if c.Memory.Enabled == nil {
		c.Memory.Enabled = new(true)
	}
	if c.Memory.Provider == "" {
		c.Memory.Provider = constants.MemoryProviderDefault
	}
	if c.Memory.Pinned.Enabled == nil {
		c.Memory.Pinned.Enabled = new(true)
	}

}

func (c *Config) applyDefaultModelBaseURL() {
	if c == nil || c.Models.Main.BaseURL != "" {
		return
	}

	if mapped := defaultBaseURLForProvider(c.Models.Main.Provider, c.Models.Main.APIMode); mapped != "" {
		c.Models.Main.BaseURL = mapped
	}
}

// Normalize trims fields, applies defaults, and resolves default model base URL when unset.
func (c *Config) Normalize() {
	if c == nil {
		return
	}

	c.normalizeFields()
	c.applyDefaultModelBaseURL()
}

func defaultBaseURLForProvider(provider, apiMode string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	apiMode = strings.TrimSpace(strings.ToLower(apiMode))
	if apiMode == "" {
		apiMode = DefaultModelAPIMode
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

	return boolValueDefault(c.Models.Verify, true)
}

func (c *Config) StreamEnabled() bool {
	if c == nil {
		return true
	}

	return boolValueDefault(c.Models.Main.Stream, true)
}

func (c *Config) ModelMaxRetriesEffective() int {
	if c == nil {
		return DefaultModelMaxRetries
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
	return boolValueDefault(c.Memory.Enabled, true)
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

	return defaultBaseURLForProvider(sum, sumMode)
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

func resolvePathsFromBase(values []string, baseDir string) []string {
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

func defaultFSRoots() []string {
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

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func boolValueDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is required")
	}

	c.Normalize()

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

	if c.RPC.Port <= 0 {
		return errors.New("rpc port must be greater than zero; set HAND_RPC_PORT, provide it in config, or use --rpc.port")
	}

	if c.Session.MaxIterations <= 0 {
		return errors.New("max iterations must be greater than zero; set HAND_SESSION_MAX_ITERATIONS, provide it in config, " +
			"or use --max-iterations")
	}
	if c.ModelMaxRetriesEffective() < 0 {
		return errors.New("model max retries must be greater than or equal to zero; use --model.max-retries")
	}

	switch c.Models.Main.APIMode {
	case DefaultModelAPIMode:
	case "responses":
	default:
		return errors.New("model api mode must be one of: completions, responses; use --model.api-mode")
	}

	if c.Models.Summary.APIMode != "" {
		switch c.Models.Summary.APIMode {
		case DefaultModelAPIMode:
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
				return fmt.Errorf("%s: %w", slot.field, unknownModelError(auth.Provider, slot.slug))
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
	switch c.RerankerEffective() {
	case constants.RerankerDeterministic, constants.RerankerNoop, constants.RerankerLLM:
	default:
		return errors.New("reranker type must be one of: deterministic, noop, llm")
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
	if c.RerankerEffective() == constants.RerankerLLM {
		if c.RerankerModelEffective() == "" {
			return errors.New("reranker model is required")
		}
	}

	return nil
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
			defaultBaseURLForProvider("openrouter", DefaultModelAPIMode),
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
		return fmt.Errorf("models.embedding.name: %w", unknownModelError(auth.Provider, c.Models.Embedding.Name))
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
		BaseURL:  defaultBaseURLForProvider(provider, "embeddings"),
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
		return firstNonEmpty(c.Models.OpenRouterAPIKey, c.Models.Key)
	case "openai":
		return firstNonEmpty(c.Models.OpenAIAPIKey, c.Models.Key)
	default:
		return c.Models.Key
	}
}

func firstNonEmpty(values ...string) string {
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

func resolveModelMetadataFromProvider(ctx context.Context, cfg *Config, auth ModelAuth) (ModelMetadata, error) {
	if cfg == nil {
		return ModelMetadata{}, nil
	}

	return resolveModelMetadataForSlug(ctx, auth, cfg.Models.Main.Name)
}

func resolveModelMetadataForSlug(ctx context.Context, auth ModelAuth, slug string) (ModelMetadata, error) {
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
		baseURL = defaultBaseURLForProvider("openrouter", DefaultModelAPIMode)
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
		baseURL = defaultBaseURLForProvider("openrouter", DefaultModelAPIMode)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models/"+openRouterModelPath(model)+"/endpoints",
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

func openRouterModelPath(model string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(model), "/"), "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
}

func fetchOpenAIModelMetadata(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range openAIModelDocSlugs(model) {
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
	for _, candidate := range openAIModelDocSlugs(model) {
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

func unknownModelError(provider, model string) error {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "openrouter":
		return fmt.Errorf("model %q is not available on openrouter", model)
	default:
		return fmt.Errorf("model %q is not available on openai", model)
	}
}

func openAIModelDocSlugs(model string) []string {
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

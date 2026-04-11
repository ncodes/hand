package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/wandxy/hand/internal/datadir"
)

type Config struct {
	Name                       string
	Model                      string
	SummaryModel               string
	Stream                     *bool
	ContextLength              int
	VerifyModel                *bool
	ModelProvider              string
	ModelKey                   string
	OpenAIAPIKey               string
	OpenRouterAPIKey           string
	ModelBaseURL               string
	SummaryProvider            string
	SummaryModelBaseURL        string
	SummaryModelAPIMode        string
	ModelAPIMode               string
	RPCAddress                 string
	RPCPort                    int
	MaxIterations              int
	LogLevel                   string
	LogNoColor                 bool
	DebugRequests              bool
	DebugTraces                bool
	DebugTraceDir              string
	WebProvider                string
	WebAPIKey                  string
	WebBaseURL                 string
	WebMaxCharPerResult        int
	WebMaxExtractCharPerResult int
	WebMaxExtractResponseBytes int
	RulesFiles                 []string
	Instruct                   string
	Platform                   string
	CapFilesystem              *bool
	CapNetwork                 *bool
	CapExec                    *bool
	CapMemory                  *bool
	CapBrowser                 *bool
	FSRoots                    []string
	ExecAllow                  []string
	ExecAsk                    []string
	ExecDeny                   []string
	StorageBackend             string
	SessionDefaultIdleExpiry   time.Duration
	SessionArchiveRetention    time.Duration
	CompactionEnabled          *bool
	CompactionTriggerPercent   float64
	CompactionWarnPercent      float64
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
		"openrouter": {
			DefaultModelAPIMode: "https://openrouter.ai/api/v1",
			"responses":         "https://openrouter.ai/api/v1/responses",
		},
		"openai": {
			DefaultModelAPIMode: "",
			"responses":         "",
		},
	}
)

var contextWindowPatternOAI = regexp.MustCompile(`([0-9][0-9,]*)(?:\s|<!--[^>]*-->)+context window`)

const (
	defaultModel                      = "openai/gpt-4o-mini"
	defaultContextLength              = 128000
	defaultModelProvider              = "openrouter"
	DefaultModelAPIMode               = "completions"
	DefaultMaxIterations              = 90
	DefaultWebMaxCharPerResult        = 1200
	DefaultWebMaxExtractCharPerResult = 50000
	DefaultWebMaxExtractResponseBytes = 2 * 1024 * 1024
	defaultMaxIterations              = DefaultMaxIterations
)

type fileConfig struct {
	Name          string `yaml:"name"`
	Instruct      string `yaml:"instruct"`
	Platform      string `yaml:"platform"`
	MaxIterations int    `yaml:"maxIterations"`

	Model struct {
		Name             string `yaml:"name"`
		SummaryModel     string `yaml:"summaryModel"`
		Stream           *bool  `yaml:"stream"`
		ContextLength    int    `yaml:"contextLength"`
		VerifyModel      *bool  `yaml:"verifyModel"`
		Provider         string `yaml:"provider"`
		Key              string `yaml:"key"`
		OpenAIAPIKey     string `yaml:"openaiApiKey"`
		OpenRouterAPIKey string `yaml:"openrouterApiKey"`
		BaseURL          string `yaml:"baseUrl"`
		SummaryProvider  string `yaml:"summaryProvider"`
		SummaryBaseURL   string `yaml:"summaryBaseUrl"`
		SummaryAPIMode   string `yaml:"summaryApiMode"`
		APIMode          string `yaml:"apiMode"`
	} `yaml:"model"`

	Log struct {
		Level   string `yaml:"level"`
		NoColor bool   `yaml:"noColor"`
	} `yaml:"log"`

	Debug struct {
		Requests bool   `yaml:"requests"`
		Traces   bool   `yaml:"traces"`
		TraceDir string `yaml:"traceDir"`
	} `yaml:"debug"`

	Web struct {
		Provider                string `yaml:"provider"`
		APIKey                  string `yaml:"apiKey"`
		BaseURL                 string `yaml:"baseUrl"`
		MaxCharPerResult        int    `yaml:"maxCharPerResult"`
		MaxExtractCharPerResult int    `yaml:"maxExtractCharPerResult"`
		MaxExtractResponseBytes int    `yaml:"maxExtractResponseBytes"`
	} `yaml:"web"`

	RPC struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
	} `yaml:"rpc"`

	FS struct {
		Roots []string `yaml:"roots"`
	} `yaml:"fs"`

	Exec struct {
		Allow []string `yaml:"allow"`
		Ask   []string `yaml:"ask"`
		Deny  []string `yaml:"deny"`
	} `yaml:"exec"`

	Storage struct {
		Backend string `yaml:"backend"`
	} `yaml:"storage"`

	Session struct {
		DefaultIdleExpiry string `yaml:"defaultIdleExpiry"`
		ArchiveRetention  string `yaml:"archiveRetention"`
	} `yaml:"session"`

	Compaction struct {
		Enabled        *bool   `yaml:"enabled"`
		TriggerPercent float64 `yaml:"triggerPercent"`
		WarnPercent    float64 `yaml:"warnPercent"`
	} `yaml:"compaction"`

	Cap struct {
		Filesystem *bool `yaml:"fs"`
		Network    *bool `yaml:"net"`
		Exec       *bool `yaml:"exec"`
		Memory     *bool `yaml:"mem"`
		Browser    *bool `yaml:"browser"`
	} `yaml:"cap"`

	Rules struct {
		Files []string `yaml:"files"`
	} `yaml:"rules"`
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
	requestedContextLength := cfg.ContextLength
	cfg.Normalize()
	applyProviderModelMetadata(context.Background(), cfg, requestedContextLength)

	return cfg, nil
}

func Get() *Config {
	configMu.RLock()
	defer configMu.RUnlock()

	if globalConfig == nil {
		return &Config{
			Model:                    defaultModel,
			Stream:                   new(true),
			ContextLength:            defaultContextLength,
			VerifyModel:              new(true),
			ModelAPIMode:             DefaultModelAPIMode,
			MaxIterations:            defaultMaxIterations,
			LogLevel:                 "info",
			DebugTraceDir:            datadir.DebugTraceDir(),
			Platform:                 "cli",
			CapFilesystem:            new(true),
			CapNetwork:               new(true),
			CapExec:                  new(true),
			CapMemory:                new(true),
			CapBrowser:               new(false),
			FSRoots:                  defaultFSRoots(),
			StorageBackend:           "sqlite",
			SessionDefaultIdleExpiry: 24 * time.Hour,
			SessionArchiveRetention:  30 * 24 * time.Hour,
			CompactionEnabled:        new(true),
			CompactionTriggerPercent: 0.85,
			CompactionWarnPercent:    0.95,
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

	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	return &Config{
		Name:                       raw.Name,
		Model:                      raw.Model.Name,
		SummaryModel:               raw.Model.SummaryModel,
		Stream:                     raw.Model.Stream,
		ContextLength:              raw.Model.ContextLength,
		VerifyModel:                raw.Model.VerifyModel,
		ModelProvider:              raw.Model.Provider,
		ModelKey:                   raw.Model.Key,
		OpenAIAPIKey:               raw.Model.OpenAIAPIKey,
		OpenRouterAPIKey:           raw.Model.OpenRouterAPIKey,
		ModelBaseURL:               raw.Model.BaseURL,
		SummaryProvider:            raw.Model.SummaryProvider,
		SummaryModelBaseURL:        raw.Model.SummaryBaseURL,
		SummaryModelAPIMode:        raw.Model.SummaryAPIMode,
		ModelAPIMode:               raw.Model.APIMode,
		RPCAddress:                 raw.RPC.Address,
		RPCPort:                    raw.RPC.Port,
		MaxIterations:              raw.MaxIterations,
		LogLevel:                   raw.Log.Level,
		LogNoColor:                 raw.Log.NoColor,
		DebugRequests:              raw.Debug.Requests,
		DebugTraces:                raw.Debug.Traces,
		DebugTraceDir:              raw.Debug.TraceDir,
		WebProvider:                raw.Web.Provider,
		WebAPIKey:                  raw.Web.APIKey,
		WebBaseURL:                 raw.Web.BaseURL,
		WebMaxCharPerResult:        raw.Web.MaxCharPerResult,
		WebMaxExtractCharPerResult: raw.Web.MaxExtractCharPerResult,
		WebMaxExtractResponseBytes: raw.Web.MaxExtractResponseBytes,
		RulesFiles:                 raw.Rules.Files,
		Instruct:                   raw.Instruct,
		Platform:                   raw.Platform,
		CapFilesystem:              raw.Cap.Filesystem,
		CapNetwork:                 raw.Cap.Network,
		CapExec:                    raw.Cap.Exec,
		CapMemory:                  raw.Cap.Memory,
		CapBrowser:                 raw.Cap.Browser,
		FSRoots:                    resolvePathsFromBase(raw.FS.Roots, baseDir),
		ExecAllow:                  raw.Exec.Allow,
		ExecAsk:                    raw.Exec.Ask,
		ExecDeny:                   raw.Exec.Deny,
		StorageBackend:             raw.Storage.Backend,
		SessionDefaultIdleExpiry:   parseDurationOrZero(raw.Session.DefaultIdleExpiry),
		SessionArchiveRetention:    parseDurationOrZero(raw.Session.ArchiveRetention),
		CompactionEnabled:          raw.Compaction.Enabled,
		CompactionTriggerPercent:   raw.Compaction.TriggerPercent,
		CompactionWarnPercent:      raw.Compaction.WarnPercent,
	}, nil
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if value := strings.TrimSpace(os.Getenv("NAME")); value != "" {
		cfg.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL")); value != "" {
		cfg.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_SUMMARY")); value != "" {
		cfg.SummaryModel = value
	}
	if value, ok := parseOptionalBoolEnv("MODEL_STREAM"); ok {
		cfg.Stream = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_CONTEXT_LENGTH")); value != "" {
		if contextLength, err := strconv.Atoi(value); err == nil {
			cfg.ContextLength = contextLength
		}
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("MODEL_VERIFY_MODEL"))); value != "" {
		cfg.VerifyModel = new(value == "1" || value == "true" || value == "yes")
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_PROVIDER")); value != "" {
		cfg.ModelProvider = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_KEY")); value != "" {
		cfg.ModelKey = value
	}
	if value := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); value != "" {
		cfg.OpenAIAPIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); value != "" {
		cfg.OpenRouterAPIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_BASE_URL")); value != "" {
		cfg.ModelBaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_SUMMARY_PROVIDER")); value != "" {
		cfg.SummaryProvider = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_SUMMARY_BASE_URL")); value != "" {
		cfg.SummaryModelBaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_API_MODE")); value != "" {
		cfg.ModelAPIMode = value
	}
	if value := strings.TrimSpace(os.Getenv("MODEL_SUMMARY_API_MODE")); value != "" {
		cfg.SummaryModelAPIMode = value
	}
	if value := strings.TrimSpace(os.Getenv("RPC_ADDRESS")); value != "" {
		cfg.RPCAddress = value
	}
	if value := strings.TrimSpace(os.Getenv("RPC_PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.RPCPort = port
		}
	}
	if value := strings.TrimSpace(os.Getenv("MAX_ITERATIONS")); value != "" {
		if maxIterations, err := strconv.Atoi(value); err == nil {
			cfg.MaxIterations = maxIterations
		}
	}
	if value := strings.TrimSpace(os.Getenv("LOG_LEVEL")); value != "" {
		cfg.LogLevel = value
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("LOG_NO_COLOR"))); value != "" {
		cfg.LogNoColor = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_REQUESTS"))); value != "" {
		cfg.DebugRequests = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_TRACES"))); value != "" {
		cfg.DebugTraces = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(os.Getenv("DEBUG_TRACE_DIR")); value != "" {
		cfg.DebugTraceDir = value
	}
	if value := strings.TrimSpace(os.Getenv("WEB_PROVIDER")); value != "" {
		cfg.WebProvider = value
	}
	if value := strings.TrimSpace(os.Getenv("WEB_API_KEY")); value != "" {
		cfg.WebAPIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("WEB_BASE_URL")); value != "" {
		cfg.WebBaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("WEB_MAX_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.WebMaxCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("WEB_MAX_EXTRACT_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.WebMaxExtractCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("WEB_MAX_EXTRACT_RESPONSE_BYTES")); value != "" {
		if bytes, err := strconv.Atoi(value); err == nil {
			cfg.WebMaxExtractResponseBytes = bytes
		}
	}
	if cfg.WebProvider == "" {
		switch {
		case strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY")) != "" || strings.TrimSpace(os.Getenv("FIRECRAWL_API_URL")) != "":
			cfg.WebProvider = "firecrawl"
		case strings.TrimSpace(os.Getenv("PARALLEL_API_KEY")) != "":
			cfg.WebProvider = "parallel"
		case strings.TrimSpace(os.Getenv("TAVILY_API_KEY")) != "":
			cfg.WebProvider = "tavily"
		case strings.TrimSpace(os.Getenv("EXA_API_KEY")) != "":
			cfg.WebProvider = "exa"
		}
	}
	if cfg.WebAPIKey == "" {
		switch strings.TrimSpace(strings.ToLower(cfg.WebProvider)) {
		case "firecrawl":
			cfg.WebAPIKey = strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
		case "parallel":
			cfg.WebAPIKey = strings.TrimSpace(os.Getenv("PARALLEL_API_KEY"))
		case "tavily":
			cfg.WebAPIKey = strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
		case "exa":
			cfg.WebAPIKey = strings.TrimSpace(os.Getenv("EXA_API_KEY"))
		}
	}
	if cfg.WebBaseURL == "" && strings.TrimSpace(strings.ToLower(cfg.WebProvider)) == "firecrawl" {
		cfg.WebBaseURL = strings.TrimSpace(os.Getenv("FIRECRAWL_API_URL"))
	}
	if value := strings.TrimSpace(os.Getenv("RULES_FILES")); value != "" {
		cfg.RulesFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("INSTRUCT")); value != "" {
		cfg.Instruct = value
	}
	if value := strings.TrimSpace(os.Getenv("PLATFORM")); value != "" {
		cfg.Platform = value
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_FS_ROOTS")); value != "" {
		cfg.FSRoots = splitAndTrimCSV(value)
	}

	if value, ok := parseOptionalBoolEnv("AGENT_CAP_FS"); ok {
		cfg.CapFilesystem = new(value)
	}
	if value, ok := parseOptionalBoolEnv("AGENT_CAP_NET"); ok {
		cfg.CapNetwork = new(value)
	}
	if value, ok := parseOptionalBoolEnv("AGENT_CAP_EXEC"); ok {
		cfg.CapExec = new(value)
	}
	if value, ok := parseOptionalBoolEnv("AGENT_CAP_MEM"); ok {
		cfg.CapMemory = new(value)
	}
	if value, ok := parseOptionalBoolEnv("AGENT_CAP_BROWSER"); ok {
		cfg.CapBrowser = new(value)
	}

	if value := strings.TrimSpace(os.Getenv("AGENT_EXEC_ALLOW")); value != "" {
		cfg.ExecAllow = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_EXEC_ASK")); value != "" {
		cfg.ExecAsk = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_EXEC_DENY")); value != "" {
		cfg.ExecDeny = splitAndTrimCSV(value)
	}

	if value := strings.TrimSpace(os.Getenv("AGENT_STORAGE_BACKEND")); value != "" {
		cfg.StorageBackend = value
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_SESSION_DEFAULT_IDLE_EXPIRY")); value != "" {
		cfg.SessionDefaultIdleExpiry = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_SESSION_ARCHIVE_RETENTION")); value != "" {
		cfg.SessionArchiveRetention = parseDurationOrZero(value)
	}

	if value, ok := parseOptionalBoolEnv("AGENT_COMPACTION_ENABLED"); ok {
		cfg.CompactionEnabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_COMPACTION_TRIGGER_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.CompactionTriggerPercent = percent
		}
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_COMPACTION_WARN_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.CompactionWarnPercent = percent
		}
	}
}

// normalizeFields applies trimming and defaults except default model base URL resolution.
func (c *Config) normalizeFields() {
	if c == nil {
		return
	}

	c.Name = strings.TrimSpace(c.Name)
	c.Model = strings.TrimSpace(c.Model)
	c.SummaryModel = strings.TrimSpace(c.SummaryModel)
	c.ModelProvider = strings.TrimSpace(strings.ToLower(c.ModelProvider))
	c.ModelKey = strings.TrimSpace(c.ModelKey)
	c.OpenAIAPIKey = strings.TrimSpace(c.OpenAIAPIKey)
	c.OpenRouterAPIKey = strings.TrimSpace(c.OpenRouterAPIKey)
	c.ModelBaseURL = strings.TrimSpace(c.ModelBaseURL)
	c.SummaryProvider = strings.TrimSpace(strings.ToLower(c.SummaryProvider))
	c.SummaryModelBaseURL = strings.TrimSpace(c.SummaryModelBaseURL)
	c.ModelAPIMode = strings.TrimSpace(strings.ToLower(c.ModelAPIMode))
	c.SummaryModelAPIMode = strings.TrimSpace(strings.ToLower(c.SummaryModelAPIMode))
	c.LogLevel = strings.TrimSpace(strings.ToLower(c.LogLevel))
	c.DebugTraceDir = strings.TrimSpace(c.DebugTraceDir)
	c.WebProvider = strings.TrimSpace(strings.ToLower(c.WebProvider))
	c.WebAPIKey = strings.TrimSpace(c.WebAPIKey)
	c.WebBaseURL = strings.TrimSpace(c.WebBaseURL)
	c.RulesFiles = normalizeRulePaths(c.RulesFiles)
	c.Instruct = strings.TrimSpace(c.Instruct)
	c.Platform = strings.TrimSpace(strings.ToLower(c.Platform))
	c.FSRoots = normalizeFSRoots(c.FSRoots)
	c.ExecAllow = dedupeAndTrim(c.ExecAllow)
	c.ExecAsk = dedupeAndTrim(c.ExecAsk)
	c.ExecDeny = dedupeAndTrim(c.ExecDeny)
	c.StorageBackend = strings.TrimSpace(strings.ToLower(c.StorageBackend))

	if c.Model == "" {
		c.Model = defaultModel
	}
	if c.Stream == nil {
		c.Stream = new(true)
	}
	if c.VerifyModel == nil {
		c.VerifyModel = new(true)
	}
	if c.ContextLength <= 0 {
		c.ContextLength = defaultContextLength
	}

	if c.ModelProvider == "" {
		c.ModelProvider = defaultModelProvider
	}

	if c.ModelAPIMode == "" {
		c.ModelAPIMode = DefaultModelAPIMode
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.RPCAddress == "" {
		c.RPCAddress = "127.0.0.1"
	}

	if c.RPCPort == 0 {
		c.RPCPort = 50051
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = defaultMaxIterations
	}
	if c.DebugTraceDir == "" {
		c.DebugTraceDir = datadir.DebugTraceDir()
	}
	if c.Platform == "" {
		c.Platform = "cli"
	}
	if c.WebMaxCharPerResult <= 0 {
		c.WebMaxCharPerResult = DefaultWebMaxCharPerResult
	}
	if c.WebMaxExtractCharPerResult <= 0 {
		c.WebMaxExtractCharPerResult = DefaultWebMaxExtractCharPerResult
	}
	if c.WebMaxExtractResponseBytes <= 0 {
		c.WebMaxExtractResponseBytes = DefaultWebMaxExtractResponseBytes
	}

	if c.CapFilesystem == nil {
		c.CapFilesystem = new(true)
	}
	if c.CapNetwork == nil {
		c.CapNetwork = new(true)
	}
	if c.CapExec == nil {
		c.CapExec = new(true)
	}
	if c.CapMemory == nil {
		c.CapMemory = new(true)
	}
	if c.CapBrowser == nil {
		c.CapBrowser = new(false)
	}

	if len(c.FSRoots) == 0 {
		c.FSRoots = defaultFSRoots()
	}

	if c.StorageBackend == "" {
		c.StorageBackend = "sqlite"
	}

	if c.SessionDefaultIdleExpiry <= 0 {
		c.SessionDefaultIdleExpiry = 24 * time.Hour
	}
	if c.SessionArchiveRetention <= 0 {
		c.SessionArchiveRetention = 30 * 24 * time.Hour
	}
	if c.CompactionEnabled == nil {
		c.CompactionEnabled = new(true)
	}
	if c.CompactionTriggerPercent <= 0 {
		c.CompactionTriggerPercent = 0.85
	}
	if c.CompactionWarnPercent <= 0 {
		c.CompactionWarnPercent = 0.95
	}
}

func (c *Config) applyDefaultModelBaseURL() {
	if c == nil || c.ModelBaseURL != "" {
		return
	}

	if mapped := defaultBaseURLForProvider(c.ModelProvider, c.ModelAPIMode); mapped != "" {
		c.ModelBaseURL = mapped
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

func (c *Config) VerifyModelEnabled() bool {
	if c == nil {
		return true
	}

	return boolValueDefault(c.VerifyModel, true)
}

func (c *Config) StreamEnabled() bool {
	if c == nil {
		return true
	}

	return boolValueDefault(c.Stream, true)
}

func (c *Config) SummaryModelEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.SummaryModel != "" {
		return c.SummaryModel
	}

	return c.Model
}

func (c *Config) SummaryProviderEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.SummaryProvider != "" {
		return c.SummaryProvider
	}

	return c.ModelProvider
}

func (c *Config) SummaryModelAPIModeEffective() string {
	if c == nil {
		return ""
	}

	c.normalizeFields()
	if c.SummaryModelAPIMode != "" {
		return c.SummaryModelAPIMode
	}

	return c.ModelAPIMode
}

func (c *Config) summaryModelBaseURLEffective() string {
	main := c.ModelProvider
	sum := c.SummaryProviderEffective()
	sumMode := c.SummaryModelAPIModeEffective()
	mainMode := c.ModelAPIMode

	if sum == main && sumMode == mainMode {
		return c.ModelBaseURL
	}

	if u := strings.TrimSpace(c.SummaryModelBaseURL); u != "" {
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
		return ModelAuth{}, errors.New("model key is required; set MODEL_KEY, provide it in config, or use --model.key")
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
		return errors.New("name is required; set NAME, provide it in config, or use --name")
	}

	if !isValidModelSlug(c.Model) {
		return errors.New("model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
	}

	if c.SummaryModel != "" && !isValidModelSlug(c.SummaryModel) {
		return errors.New("summary model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
	}

	if _, ok := providerDefaultBaseURLs[strings.TrimSpace(strings.ToLower(c.ModelProvider))]; !ok {
		return errors.New("model provider must be one of: openai, openrouter")
	}

	if c.SummaryProvider != "" {
		if _, ok := providerDefaultBaseURLs[c.SummaryProvider]; !ok {
			return errors.New("summary model provider must be one of: openai, openrouter")
		}
	}

	auth, err := c.ResolveModelAuth()
	if err != nil {
		return err
	}

	summaryAuth, err := c.ResolveSummaryModelAuth()
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.RPCAddress) == "" {
		return errors.New("rpc address is required; set RPC_ADDRESS, provide it in config, or use --rpc.address")
	}

	if c.RPCPort <= 0 {
		return errors.New("rpc port must be greater than zero; set RPC_PORT, provide it in config, or use --rpc.port")
	}

	if c.MaxIterations <= 0 {
		return errors.New("max iterations must be greater than zero; set MAX_ITERATIONS, provide it in config, " +
			"or use --max-iterations")
	}

	switch c.ModelAPIMode {
	case DefaultModelAPIMode:
	case "responses":
	default:
		return errors.New("model api mode must be one of: completions, responses; use --model.api-mode")
	}

	if c.SummaryModelAPIMode != "" {
		switch c.SummaryModelAPIMode {
		case DefaultModelAPIMode:
		case "responses":
		default:
			return errors.New("summary model api mode must be one of: completions, responses; " +
				"use --model.summary-api-mode")
		}
	}

	if c.StorageBackend != "memory" && c.StorageBackend != "sqlite" {
		return errors.New("storage backend must be one of: memory, sqlite")
	}

	if c.CompactionTriggerPercent >= 1 {
		return errors.New("compaction trigger percent must be greater than zero and less than one")
	}
	if c.CompactionWarnPercent >= 1 {
		return errors.New("compaction warn percent must be greater than zero and less than one")
	}
	if c.CompactionWarnPercent < c.CompactionTriggerPercent {
		return errors.New("compaction warn percent must be greater than or equal to compaction trigger percent")
	}

	if c.VerifyModelEnabled() {
		verifySlots := []modelVerifySlot{{field: "model.name", slug: c.Model}}
		if c.SummaryModel != "" && c.SummaryModel != c.Model {
			verifySlots = append(verifySlots, modelVerifySlot{field: "model.summaryModel", slug: c.SummaryModel})
		}

		for _, slot := range verifySlots {
			slotAuth := auth
			if slot.field == "model.summaryModel" {
				slotAuth = summaryAuth
			}
			verifyCfg := *c
			verifyCfg.Model = slot.slug
			meta, err := resolveModelMeta(context.Background(), &verifyCfg, slotAuth)
			if err != nil {
				return fmt.Errorf("%s: %w", slot.field, err)
			}
			if !meta.Exists {
				return fmt.Errorf("%s: %w", slot.field, unknownModelError(auth.Provider, slot.slug))
			}
		}
	}

	switch strings.TrimSpace(strings.ToLower(c.LogLevel)) {
	case "", "debug", "info", "warn", "error":
		return nil
	default:
		return errors.New("log level must be one of debug, info, warn, or error; use --log.level")
	}
}

func (c *Config) ResolveModelAuth() (ModelAuth, error) {
	if c == nil {
		return ModelAuth{}, errors.New("config is required")
	}

	c.Normalize()

	auth := ModelAuth{
		Provider: c.ModelProvider,
		BaseURL:  c.ModelBaseURL,
	}

	auth.APIKey = c.resolveAPIKeyForProvider(c.ModelProvider)
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

func (c *Config) resolveAPIKeyForProvider(provider string) string {
	switch provider {
	case "openrouter":
		return firstNonEmpty(c.OpenRouterAPIKey, c.ModelKey)
	case "openai":
		return firstNonEmpty(c.OpenAIAPIKey, c.ModelKey)
	default:
		return c.ModelKey
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
	if !cfg.VerifyModelEnabled() {
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
		cfg.ContextLength = meta.ContextLength
	}
}

func resolveModelMetadataFromProvider(ctx context.Context, cfg *Config, auth ModelAuth) (ModelMetadata, error) {
	if cfg == nil {
		return ModelMetadata{}, nil
	}

	return resolveModelMetadataForSlug(ctx, auth, cfg.Model)
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

func fetchOpenAIModelMetadata(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range openAIModelDocCandidates(model) {
		meta, err := fetchOpenAIModelMetadataCandidate(ctx, candidate)
		if err != nil {
			return ModelMetadata{}, err
		}
		if meta.Exists {
			return meta, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenAIModelMetadataCandidate(ctx context.Context, model string) (ModelMetadata, error) {
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

	match := contextWindowPatternOAI.FindStringSubmatch(string(body))
	if len(match) != 2 {
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

func unknownModelError(provider, model string) error {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "openrouter":
		return fmt.Errorf("model %q is not available on openrouter", model)
	default:
		return fmt.Errorf("model %q is not available on openai", model)
	}
}

func openAIModelDocCandidates(model string) []string {
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

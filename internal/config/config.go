package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/workspace"
)

type Config struct {
	Name             string
	Model            string
	ModelRouter      string
	ModelKey         string
	OpenAIAPIKey     string
	OpenRouterAPIKey string
	ModelBaseURL     string
	ModelAPIMode     string
	RPCAddress       string
	RPCPort          int
	MaxIterations    int
	LogLevel         string
	LogNoColor       bool
	DebugRequests    bool
	DebugTraces      bool
	DebugTraceDir    string
	RulesFiles       []string
	Instruct         string
	Platform         string
	CapFilesystem    *bool
	CapNetwork       *bool
	CapExec          *bool
	CapMemory        *bool
	CapBrowser       *bool
}

type ModelAuth struct {
	Router  string
	APIKey  string
	BaseURL string
}

var (
	globalConfig     *Config
	configMu         sync.RWMutex
	loadDotEnv       = godotenv.Load
	supportedRouters = map[string]string{
		"openrouter": "https://openrouter.ai/api/v1",
		"none":       "",
	}
)

const (
	defaultModel         = "openai/gpt-4o-mini"
	defaultModelRouter   = "openrouter"
	DefaultModelAPIMode  = "chat-completions"
	DefaultMaxIterations = 90
	defaultMaxIterations = DefaultMaxIterations
)

type fileConfig struct {
	Name  string `yaml:"name"`
	Model struct {
		Name             string `yaml:"name"`
		Router           string `yaml:"router"`
		Key              string `yaml:"key"`
		OpenAIAPIKey     string `yaml:"openaiApiKey"`
		OpenRouterAPIKey string `yaml:"openrouterApiKey"`
		BaseURL          string `yaml:"baseUrl"`
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

	RPC struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
	} `yaml:"rpc"`

	Agent struct {
		MaxIterations int    `yaml:"maxIterations"`
		Instruct      string `yaml:"instruct"`
		Cap           struct {
			Filesystem *bool `yaml:"fs"`
			Network    *bool `yaml:"net"`
			Exec       *bool `yaml:"exec"`
			Memory     *bool `yaml:"mem"`
			Browser    *bool `yaml:"browser"`
		} `yaml:"cap"`
	} `yaml:"agent"`

	Platform string `yaml:"platform"`

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
	cfg.Normalize()

	return cfg, nil
}

func Get() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalConfig == nil {
		return &Config{
			Model:         defaultModel,
			ModelAPIMode:  DefaultModelAPIMode,
			MaxIterations: defaultMaxIterations,
			LogLevel:      "info",
			DebugTraceDir: datadir.DebugTraceDir(),
			Platform:      "cli",
			CapFilesystem: new(true),
			CapNetwork:    new(true),
			CapExec:       new(true),
			CapMemory:     new(true),
			CapBrowser:    new(false),
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
		Name:             raw.Name,
		Model:            raw.Model.Name,
		ModelRouter:      raw.Model.Router,
		ModelKey:         raw.Model.Key,
		OpenAIAPIKey:     raw.Model.OpenAIAPIKey,
		OpenRouterAPIKey: raw.Model.OpenRouterAPIKey,
		ModelBaseURL:     raw.Model.BaseURL,
		ModelAPIMode:     raw.Model.APIMode,
		RPCAddress:       raw.RPC.Address,
		RPCPort:          raw.RPC.Port,
		MaxIterations:    raw.Agent.MaxIterations,
		LogLevel:         raw.Log.Level,
		LogNoColor:       raw.Log.NoColor,
		DebugRequests:    raw.Debug.Requests,
		DebugTraces:      raw.Debug.Traces,
		DebugTraceDir:    raw.Debug.TraceDir,
		RulesFiles:       raw.Rules.Files,
		Instruct:         raw.Agent.Instruct,
		Platform:         raw.Platform,
		CapFilesystem:    raw.Agent.Cap.Filesystem,
		CapNetwork:       raw.Agent.Cap.Network,
		CapExec:          raw.Agent.Cap.Exec,
		CapMemory:        raw.Agent.Cap.Memory,
		CapBrowser:       raw.Agent.Cap.Browser,
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
	if value := strings.TrimSpace(os.Getenv("MODEL_ROUTER")); value != "" {
		cfg.ModelRouter = value
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
	if value := strings.TrimSpace(os.Getenv("MODEL_API_MODE")); value != "" {
		cfg.ModelAPIMode = value
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
	if value := strings.TrimSpace(os.Getenv("RULES_FILES")); value != "" {
		cfg.RulesFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("INSTRUCT")); value != "" {
		cfg.Instruct = value
	}
	if value := strings.TrimSpace(os.Getenv("PLATFORM")); value != "" {
		cfg.Platform = value
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
}

func (c *Config) Normalize() {
	if c == nil {
		return
	}

	c.Name = strings.TrimSpace(c.Name)
	c.Model = strings.TrimSpace(c.Model)
	c.ModelRouter = strings.TrimSpace(strings.ToLower(c.ModelRouter))
	c.ModelKey = strings.TrimSpace(c.ModelKey)
	c.OpenAIAPIKey = strings.TrimSpace(c.OpenAIAPIKey)
	c.OpenRouterAPIKey = strings.TrimSpace(c.OpenRouterAPIKey)
	c.ModelBaseURL = strings.TrimSpace(c.ModelBaseURL)
	c.ModelAPIMode = strings.TrimSpace(strings.ToLower(c.ModelAPIMode))
	c.LogLevel = strings.TrimSpace(strings.ToLower(c.LogLevel))
	c.DebugTraceDir = strings.TrimSpace(c.DebugTraceDir)
	c.RulesFiles = workspace.NormalizeRulePaths(c.RulesFiles)
	c.Instruct = strings.TrimSpace(c.Instruct)
	c.Platform = strings.TrimSpace(strings.ToLower(c.Platform))

	if c.Model == "" {
		c.Model = defaultModel
	}

	if c.ModelRouter == "" {
		c.ModelRouter = defaultModelRouter
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

	if c.ModelBaseURL == "" {
		if mappedBaseURL, ok := supportedRouters[c.ModelRouter]; ok {
			c.ModelBaseURL = mappedBaseURL
		}
	}
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

func parseOptionalBoolEnv(key string) (bool, bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return false, false
	}
	return value == "1" || value == "true" || value == "yes", true
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
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

	if strings.TrimSpace(c.Model) == "" {
		return errors.New("model is required; set MODEL, provide it in config, or use --model")
	}

	if router := strings.TrimSpace(strings.ToLower(c.ModelRouter)); router != "" {
		if _, ok := supportedRouters[router]; !ok {
			return errors.New(`model router must be one of: none, openrouter`)
		}
	}
	if _, err := c.ResolveModelAuth(); err != nil {
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
	case DefaultModelAPIMode, "responses":
	default:
		return errors.New("model api mode must be one of: chat-completions, responses; use --model.api-mode")
	}
	if c.ModelAPIMode == "responses" && c.ModelRouter == "openrouter" {
		return errors.New("model api mode 'responses' is only supported with model router 'none'; " +
			"use --model.router 'none' or --model.api-mode 'chat-completions'")
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
		Router:  c.ModelRouter,
		BaseURL: c.ModelBaseURL,
	}

	switch c.ModelRouter {
	case "openrouter":
		auth.APIKey = firstNonEmpty(c.OpenRouterAPIKey, c.ModelKey)
	case "none":
		auth.APIKey = firstNonEmpty(c.OpenAIAPIKey, c.ModelKey)
	default:
		auth.APIKey = c.ModelKey
	}

	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuth{}, errors.New("model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	}

	return auth, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

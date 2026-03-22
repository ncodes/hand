package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Name             string
	Model            string
	ModelRouter      string
	ModelKey         string
	OpenAIAPIKey     string
	OpenRouterAPIKey string
	ModelBaseURL     string
	LogLevel         string
	LogNoColor       bool
	DebugRequests    bool
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
	defaultModel       = "openai/gpt-4o-mini"
	defaultModelRouter = "openrouter"
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
	} `yaml:"model"`
	Log struct {
		Level   string `yaml:"level"`
		NoColor bool   `yaml:"noColor"`
	} `yaml:"log"`
	Debug struct {
		Requests bool `yaml:"requests"`
	} `yaml:"debug"`
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
			Model:    defaultModel,
			LogLevel: "info",
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
		LogLevel:         raw.Log.Level,
		LogNoColor:       raw.Log.NoColor,
		DebugRequests:    raw.Debug.Requests,
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
	if value := strings.TrimSpace(os.Getenv("LOG_LEVEL")); value != "" {
		cfg.LogLevel = value
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("LOG_NO_COLOR"))); value != "" {
		cfg.LogNoColor = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_REQUESTS"))); value != "" {
		cfg.DebugRequests = value == "1" || value == "true" || value == "yes"
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
	c.LogLevel = strings.TrimSpace(strings.ToLower(c.LogLevel))

	if c.Model == "" {
		c.Model = defaultModel
	}

	if c.ModelRouter == "" {
		c.ModelRouter = defaultModelRouter
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	if c.ModelBaseURL == "" {
		if mappedBaseURL, ok := supportedRouters[c.ModelRouter]; ok {
			c.ModelBaseURL = mappedBaseURL
		}
	}
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

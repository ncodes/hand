package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	Model        string
	ModelRouter  string
	ModelKey     string
	ModelBaseURL string
	LogLevel     string
	LogNoColor   bool
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

func (c *Config) Normalize() {
	if c == nil {
		return
	}

	c.Model = strings.TrimSpace(c.Model)
	c.ModelRouter = strings.TrimSpace(strings.ToLower(c.ModelRouter))
	c.ModelKey = strings.TrimSpace(c.ModelKey)
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

func (c Config) Validate() error {
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("model is required; set MODEL, provide it in config, or use --model")
	}

	if router := strings.TrimSpace(strings.ToLower(c.ModelRouter)); router != "" {
		if _, ok := supportedRouters[router]; !ok {
			return errors.New(`model router must be one of: none, openrouter`)
		}
	}
	if strings.TrimSpace(c.ModelKey) == "" {
		return errors.New("model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	}

	switch strings.TrimSpace(strings.ToLower(c.LogLevel)) {
	case "", "debug", "info", "warn", "error":
		return nil
	default:
		return errors.New("log level must be one of debug, info, warn, or error; use --log.level")
	}
}

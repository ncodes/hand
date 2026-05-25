package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wandxy/hand/internal/datadir"
)

// PreloadEnvFile loads environment variables from an optional env file before config resolution.
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

// Load reads configuration from disk and applies environment overrides.
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
	applyRegistryModelMetadata(cfg, requestedContextLength)

	return cfg, nil
}

// Get returns a configuration value addressed by path.
func Get() *Config {
	configMu.RLock()
	defer configMu.RUnlock()

	if globalConfig == nil {
		return NewDefaultConfig()
	}

	return globalConfig
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

// Set updates a configuration value addressed by path.
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

// AddFilesystemRoots appends filesystem roots to cfg after normalizing them.
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

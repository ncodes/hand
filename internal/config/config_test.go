package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreloadEnvFile_LoadsValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
LOG_LEVEL=warn
LOG_NO_COLOR=true
`), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "env-agent", os.Getenv("NAME"))
	require.Equal(t, "env-model", os.Getenv("MODEL"))
	require.Equal(t, "openrouter", os.Getenv("MODEL_ROUTER"))
	require.Equal(t, "env-key", os.Getenv("MODEL_KEY"))
	require.Equal(t, "https://env.example/v1", os.Getenv("MODEL_BASE_URL"))
	require.Equal(t, "warn", os.Getenv("LOG_LEVEL"))
	require.Equal(t, "true", os.Getenv("LOG_NO_COLOR"))
}

func TestPreloadEnvFile_DoesNotOverrideShellEnv(t *testing.T) {
	clearEnvKeys(t, "MODEL_KEY")
	t.Setenv("MODEL_KEY", "shell-key")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("MODEL_KEY=env-key\n"), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "shell-key", os.Getenv("MODEL_KEY"))
}

func TestPreloadEnvFile_ReturnsErrorForUnreadablePath(t *testing.T) {
	dir := t.TempDir()
	require.EqualError(t, PreloadEnvFile(dir), `failed to load env file "`+dir+`": read `+dir+`: is a directory`)
}

func TestPreloadEnvFile_UsesDefaultPathWhenEmpty(t *testing.T) {
	originalLoadDotEnv := loadDotEnv
	t.Cleanup(func() {
		loadDotEnv = originalLoadDotEnv
	})

	calledWith := ""
	loadDotEnv = func(filenames ...string) error {
		if len(filenames) > 0 {
			calledWith = filenames[0]
		}
		return nil
	}

	require.NoError(t, PreloadEnvFile(""))
	require.Equal(t, ".env", calledWith)
}

func TestPreloadEnvFile_IgnoresMissingFile(t *testing.T) {
	originalLoadDotEnv := loadDotEnv
	t.Cleanup(func() {
		loadDotEnv = originalLoadDotEnv
	})

	loadDotEnv = func(...string) error {
		return os.ErrNotExist
	}

	require.NoError(t, PreloadEnvFile("missing.env"))
}

func TestGet_ReturnsDefaultsWhenConfigIsUnset(t *testing.T) {
	original := Get()
	Set(nil)
	t.Cleanup(func() {
		Set(original)
	})

	cfg := Get()
	require.Empty(t, cfg.Name)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, "info", cfg.LogLevel)
	require.Empty(t, cfg.ModelRouter)
	require.Empty(t, cfg.ModelBaseURL)
}

func TestSet_StoresConfigGlobally(t *testing.T) {
	original := Get()
	t.Cleanup(func() {
		Set(original)
	})

	cfg := &Config{Name: "Test Agent", Model: "test-model", ModelRouter: "none", ModelKey: "test-key", LogLevel: "debug"}
	Set(cfg)
	require.Same(t, cfg, Get())
}

func TestConfig_ValidateRequiresKey(t *testing.T) {
	cfg := &Config{
		Name:     "test-agent",
		Model:    defaultModel,
		LogLevel: "info",
	}
	require.EqualError(t, cfg.Validate(), "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_ValidateNormalizesFields(t *testing.T) {
	cfg := &Config{
		Name:        "  Test Agent  ",
		Model:       "  test-model  ",
		ModelRouter: " OpenRouter ",
		ModelKey:    "  test-key  ",
		LogLevel:    " WARN ",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, supportedRouters["openrouter"], cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
}

func TestConfig_ValidateRequiresName(t *testing.T) {
	err := (&Config{Model: defaultModel, ModelKey: "test-key", LogLevel: "info"}).Validate()
	require.EqualError(t, err, "name is required; set NAME, provide it in config, or use --name")
}

func TestConfig_ValidateDefaultsModelWhenEmpty(t *testing.T) {
	cfg := &Config{Name: "test-agent", ModelKey: "test-key", LogLevel: "info"}

	require.NoError(t, cfg.Validate())
	require.Equal(t, defaultModel, cfg.Model)
}

func TestConfig_ValidateRejectsUnsupportedRouter(t *testing.T) {
	err := (&Config{
		Name:         "test-agent",
		Model:        defaultModel,
		ModelRouter:  "anthropic",
		ModelKey:     "test-key",
		ModelBaseURL: supportedRouters[defaultModelRouter],
		LogLevel:     "info",
	}).Validate()
	require.EqualError(t, err, "model router must be one of: none, openrouter")
}

func TestConfig_ValidateRejectsInvalidLogLevel(t *testing.T) {
	err := (&Config{
		Name:        "test-agent",
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		LogLevel:    "trace",
	}).Validate()
	require.EqualError(t, err, "log level must be one of debug, info, warn, or error; use --log.level")
}

func TestConfig_ValidateAllowsEmptyRouterAndLogLevel(t *testing.T) {
	err := (&Config{
		Name:     "test-agent",
		Model:    defaultModel,
		ModelKey: "test-key",
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_NormalizeDefaultsRouterWhenEmpty(t *testing.T) {
	cfg := &Config{
		Model:    defaultModel,
		LogLevel: "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_NormalizeIgnoresNilReceiver(t *testing.T) {
	var cfg *Config
	cfg.Normalize()
}

func TestConfig_NormalizeDefaultsModelAndLogLevel(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.Name)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestConfig_NormalizeUsesMappedBaseURLWhenRouterWasExplicitlySet(t *testing.T) {
	cfg := &Config{
		Model:       defaultModel,
		ModelRouter: defaultModelRouter,
		LogLevel:    "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_NormalizeLeavesBaseURLUnsetForNoneRouter(t *testing.T) {
	cfg := &Config{
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		LogLevel:    "info",
	}
	cfg.Normalize()
	require.Equal(t, "none", cfg.ModelRouter)
	require.Equal(t, "", cfg.ModelBaseURL)
}

func TestConfig_NormalizeTrimsAndLowercasesFields(t *testing.T) {
	cfg := &Config{
		Name:         "  Test Agent  ",
		Model:        "  test-model  ",
		ModelRouter:  " OpenRouter ",
		ModelKey:     "  test-key  ",
		ModelBaseURL: "  https://example.com/v1  ",
		LogLevel:     " WARN ",
	}
	cfg.Normalize()
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, "https://example.com/v1", cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
}

func clearEnvKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		original, ok := os.LookupEnv(key)
		if ok {
			t.Cleanup(func() {
				_ = os.Setenv(key, original)
			})
		} else {
			t.Cleanup(func() {
				_ = os.Unsetenv(key)
			})
		}
		_ = os.Unsetenv(key)
	}
}

package authcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcredential "github.com/wandxy/hand/internal/model/credential"
	"github.com/wandxy/hand/internal/profile"
)

func TestCommand_LoginStoresAPIKeyWithoutPrintingSecret(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })

	err := NewCommand().Run(context.Background(), []string{"auth", "login", "openai", "--api-key", "sk-secret-value"})
	require.NoError(t, err)
	require.NotContains(t, output.String(), "sk-secret-value")
	require.Contains(t, output.String(), "openai credential stored")

	credential, ok, err := modelcredential.NewFileStore(filepath.Join(home, "auth.json")).Get("openai")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, modelcredential.TypeAPIKey, credential.Type)
	require.Equal(t, "sk-secret-value", credential.Key)
}

func TestCommand_LoginStoresOAuthTokenWithExpiry(t *testing.T) {
	home := setAuthTestProfile(t)
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	err := NewCommand().Run(context.Background(), []string{
		"auth", "login", "github-copilot",
		"--token", "token-secret",
		"--refresh-token", "refresh-secret",
		"--expires-at", expiresAt,
		"--scope", "read",
		"--scope", "write",
	})
	require.NoError(t, err)

	credential, ok, err := modelcredential.NewFileStore(filepath.Join(home, "auth.json")).Get("github-copilot")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, modelcredential.TypeOAuth, credential.Type)
	require.Equal(t, "token-secret", credential.Token)
	require.Equal(t, "refresh-secret", credential.Refresh)
	require.Equal(t, []string{"read", "write"}, credential.Scopes)
	require.NotNil(t, credential.ExpiresAt)
}

func TestCommand_LoginValidatesCredentialFlags(t *testing.T) {
	setAuthTestProfile(t)

	err := NewCommand().Run(context.Background(), []string{"auth", "login", "openai"})
	require.EqualError(t, err, "credential is required; pass --api-key or --token, or use a provider with subscription login")

	err = NewCommand().Run(context.Background(), []string{
		"auth", "login", "openai", "--api-key", "key", "--token", "token",
	})
	require.EqualError(t, err, "use either --api-key or --token, not both")

	err = NewCommand().Run(context.Background(), []string{
		"auth", "login", "openai", "--token", "token", "--expires-at", "not-time",
	})
	require.ErrorContains(t, err, "parse --expires-at")

	err = NewCommand().Run(context.Background(), []string{"auth", "login"})
	require.EqualError(t, err, "provider is required")
}

func TestCommand_LoginUsesSubscriptionProviderWhenNoCredentialFlags(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restoreOutput := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restoreOutput) })

	previousProvider := getSubscriptionProvider
	getSubscriptionProvider = func(provider string) (modelcredential.SubscriptionProvider, bool) {
		require.Equal(t, "openai", provider)
		return fakeSubscriptionProvider{
			login: func(options modelcredential.LoginOptions) {
				require.Equal(t, "openai", options.Provider)
				require.NotNil(t, options.Input)
				require.NotNil(t, options.Output)
			},
		}, true
	}
	t.Cleanup(func() { getSubscriptionProvider = previousProvider })

	err := NewCommand().Run(context.Background(), []string{"auth", "login", "openai"})
	require.NoError(t, err)
	require.NotContains(t, output.String(), "subscription-secret")

	credential, ok, err := modelcredential.NewFileStore(filepath.Join(home, "auth.json")).Get("openai")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, modelcredential.TypeOAuth, credential.Type)
	require.Equal(t, "subscription-secret", credential.Token)
}

func TestCommand_LoginReturnsOutputError(t *testing.T) {
	setAuthTestProfile(t)
	restore := SetOutput(errorWriter{})
	t.Cleanup(func() { SetOutput(restore) })

	err := NewCommand().Run(context.Background(), []string{"auth", "login", "openai", "--api-key", "key"})
	require.EqualError(t, err, "write failed")
}

func TestCommand_StatusReportsStoredEnvironmentAndConfigSources(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })
	t.Setenv("ANTHROPIC_API_KEY", "env-secret")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`
models:
  providers:
    openrouter:
      apiKey: config-secret
`), 0o600))
	store := modelcredential.NewFileStore(filepath.Join(home, "auth.json"))
	require.NoError(t, store.Set("openai", modelcredential.StoredCredential{Type: modelcredential.TypeAPIKey, Key: "stored-secret"}))

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "openai", "anthropic", "openrouter"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "openai: stored api_key")
	require.Contains(t, output.String(), "anthropic: environment")
	require.Contains(t, output.String(), "openrouter: provider-config")
	require.NotContains(t, output.String(), "stored-secret")
	require.NotContains(t, output.String(), "env-secret")
	require.NotContains(t, output.String(), "config-secret")
}

func TestCommand_StatusReportsStoredOAuthExpiryStates(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })

	expired := time.Now().Add(-time.Hour)
	fresh := time.Now().Add(time.Hour)
	store := modelcredential.NewFileStore(filepath.Join(home, "auth.json"))
	require.NoError(t, store.Set("openai", modelcredential.StoredCredential{
		Type:      modelcredential.TypeOAuth,
		Token:     "old-token",
		ExpiresAt: &expired,
	}))
	require.NoError(t, store.Set("anthropic", modelcredential.StoredCredential{
		Type:      modelcredential.TypeOAuth,
		Token:     "fresh-token",
		ExpiresAt: &fresh,
	}))

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "openai", "anthropic"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "openai: stored oauth expired")
	require.Contains(t, output.String(), "anthropic: stored oauth refreshable")
	require.NotContains(t, output.String(), "old-token")
	require.NotContains(t, output.String(), "fresh-token")
}

func TestCommand_StatusReportsCustomProviderEnvSource(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })
	t.Setenv("CUSTOM_PROVIDER_KEY", "custom-secret")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`
models:
  providers:
    custom:
      apiKeyEnv:
        - CUSTOM_PROVIDER_KEY
`), 0o600))

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "custom"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "custom: environment")
	require.NotContains(t, output.String(), "custom-secret")
}

func TestCommand_StatusReportsAllKnownProviders(t *testing.T) {
	setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })

	err := NewCommand().Run(context.Background(), []string{"auth", "status"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "anthropic: missing")
	require.Contains(t, output.String(), "github-copilot: missing")
	require.Contains(t, output.String(), "openai: missing")
	require.Contains(t, output.String(), "openrouter: missing")
}

func TestCommand_StatusReportsConfigAndStoredProvidersWithoutArgs(t *testing.T) {
	home := setAuthTestProfile(t)
	var output bytes.Buffer
	restore := SetOutput(&output)
	t.Cleanup(func() { SetOutput(restore) })
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`
models:
  providers:
    custom-config:
      apiKey: config-secret
`), 0o600))
	store := modelcredential.NewFileStore(filepath.Join(home, "auth.json"))
	require.NoError(t, store.Set("custom-stored", modelcredential.StoredCredential{
		Type: modelcredential.TypeAPIKey,
		Key:  "stored-secret",
	}))

	err := NewCommand().Run(context.Background(), []string{"auth", "status"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "custom-config: provider-config")
	require.Contains(t, output.String(), "custom-stored: stored api_key")
	require.NotContains(t, output.String(), "config-secret")
	require.NotContains(t, output.String(), "stored-secret")
}

func TestCommand_StatusReturnsCredentialStoreParseError(t *testing.T) {
	home := setAuthTestProfile(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "auth.json"), []byte("{"), 0o600))

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "openai"})
	require.ErrorContains(t, err, "parse credential store")
}

func TestCommand_StatusReturnsOutputError(t *testing.T) {
	setAuthTestProfile(t)
	restore := SetOutput(errorWriter{})
	t.Cleanup(func() { SetOutput(restore) })

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "openai"})
	require.EqualError(t, err, "write failed")
}

func TestCommand_LogoutRemovesStoredCredential(t *testing.T) {
	home := setAuthTestProfile(t)
	store := modelcredential.NewFileStore(filepath.Join(home, "auth.json"))
	require.NoError(t, store.Set("openai", modelcredential.StoredCredential{Type: modelcredential.TypeAPIKey, Key: "stored-secret"}))

	err := NewCommand().Run(context.Background(), []string{"auth", "logout", "openai"})
	require.NoError(t, err)

	_, ok, err := store.Get("openai")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCommand_LogoutValidatesProviderArg(t *testing.T) {
	setAuthTestProfile(t)

	err := NewCommand().Run(context.Background(), []string{"auth", "logout"})
	require.EqualError(t, err, "provider is required")
}

func TestCommand_LogoutReturnsOutputError(t *testing.T) {
	home := setAuthTestProfile(t)
	store := modelcredential.NewFileStore(filepath.Join(home, "auth.json"))
	require.NoError(t, store.Set("openai", modelcredential.StoredCredential{Type: modelcredential.TypeAPIKey, Key: "stored-secret"}))
	restore := SetOutput(errorWriter{})
	t.Cleanup(func() { SetOutput(restore) })

	err := NewCommand().Run(context.Background(), []string{"auth", "logout", "openai"})
	require.EqualError(t, err, "write failed")
}

func TestCommand_ShowsHelpWithoutSubcommand(t *testing.T) {
	setAuthTestProfile(t)

	err := NewCommand().Run(context.Background(), []string{"auth"})
	require.NoError(t, err)
}

func TestLoadAuthConfig_ReturnsConfigLoadError(t *testing.T) {
	home := setAuthTestProfile(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte("models: ["), 0o600))

	_, err := loadAuthConfig(nil)
	require.ErrorContains(t, err, "failed to parse config file")
}

func TestSetOutput_NilDiscardsOutput(t *testing.T) {
	previous := SetOutput(nil)
	t.Cleanup(func() { SetOutput(previous) })

	require.Equal(t, io.Discard, authOutput)
}

func TestFormatAuthStatus_ReturnsUnknownSourceValue(t *testing.T) {
	status := modelcredential.Status{
		Configured: true,
		Source:     modelcredential.CredentialSource("runtime"),
	}

	require.Equal(t, "runtime", formatAuthStatus(status))
}

func TestGetFirstEnvValue_SkipsBlankAndMissingKeys(t *testing.T) {
	value, key := getFirstEnvValue([]string{" ", "MISSING_AUTH_TEST_KEY"})

	require.Empty(t, value)
	require.Empty(t, key)
}

func setAuthTestProfile(t *testing.T) string {
	t.Helper()

	original := profile.Active()
	home := t.TempDir()
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: home}))
	t.Cleanup(func() {
		profile.SetActive(original)
	})
	return home
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type fakeSubscriptionProvider struct {
	login func(modelcredential.LoginOptions)
}

func (p fakeSubscriptionProvider) Login(
	_ context.Context,
	options modelcredential.LoginOptions,
) (modelcredential.StoredCredential, error) {
	if p.login != nil {
		p.login(options)
	}
	return modelcredential.StoredCredential{
		Type:  modelcredential.TypeOAuth,
		Token: "subscription-secret",
	}, nil
}

func (fakeSubscriptionProvider) Refresh(
	context.Context,
	modelcredential.StoredCredential,
) (modelcredential.StoredCredential, error) {
	return modelcredential.StoredCredential{}, nil
}

func (fakeSubscriptionProvider) AuthHeaders(
	context.Context,
	modelcredential.StoredCredential,
) (map[string]string, error) {
	return nil, nil
}

package authcmd

import (
	"bytes"
	"context"
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
	require.EqualError(t, err, "credential is required; pass --api-key or --token")

	err = NewCommand().Run(context.Background(), []string{
		"auth", "login", "openai", "--api-key", "key", "--token", "token",
	})
	require.EqualError(t, err, "use either --api-key or --token, not both")

	err = NewCommand().Run(context.Background(), []string{
		"auth", "login", "openai", "--token", "token", "--expires-at", "not-time",
	})
	require.ErrorContains(t, err, "parse --expires-at")
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

func TestCommand_StatusReturnsCredentialStoreParseError(t *testing.T) {
	home := setAuthTestProfile(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "auth.json"), []byte("{"), 0o600))

	err := NewCommand().Run(context.Background(), []string{"auth", "status", "openai"})
	require.ErrorContains(t, err, "parse credential store")
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

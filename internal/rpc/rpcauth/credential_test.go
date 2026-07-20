package rpcauth

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialPath_ReturnsProfileScopedPath(t *testing.T) {
	require.Equal(t, filepath.Join("/profile", credentialFilename), CredentialPath(" /profile "))
}

func TestLoadOrCreate_PersistsAndReusesProtectedCredential(t *testing.T) {
	home := t.TempDir()
	created, err := LoadOrCreate(home)
	require.NoError(t, err)
	require.Len(t, created, credentialBytes)

	loaded, err := LoadOrCreate(home)
	require.NoError(t, err)
	require.Equal(t, created, loaded)
	require.Equal(t, os.FileMode(0o600), mustFileMode(t, CredentialPath(home)))
}

func TestRotate_ReplacesCredential(t *testing.T) {
	home := t.TempDir()
	before, err := LoadOrCreate(home)
	require.NoError(t, err)

	after, err := Rotate(home)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
	loaded, err := Load(home)
	require.NoError(t, err)
	require.Equal(t, after, loaded)
}

func TestLoadOrCreate_RejectsInvalidExistingCredential(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.Chmod(home, 0o700))
	require.NoError(t, os.WriteFile(CredentialPath(home), []byte("invalid\n"), 0o644))

	credential, err := LoadOrCreate(home)
	require.Nil(t, credential)
	require.ErrorContains(t, err, "load existing RPC owner credential")
	require.ErrorContains(t, err, "permissions are too broad")
}

func TestLoad_RejectsInvalidCredentialAndBroadPermissions(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.Chmod(home, 0o700))
	path := CredentialPath(home)
	require.NoError(t, os.WriteFile(path, []byte("invalid\n"), 0o600))
	_, err := Load(home)
	require.EqualError(t, err, "RPC owner credential is invalid")

	if runtime.GOOS == "windows" {
		return
	}
	credential := bytes.Repeat([]byte{1}, credentialBytes)
	require.NoError(t, os.WriteFile(path, []byte(encodeCredential(credential)), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))
	_, err = Load(home)
	require.EqualError(t, err, "RPC owner credential permissions are too broad")
	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, os.Chmod(home, 0o755))
	_, err = Load(home)
	require.EqualError(t, err, "RPC owner credential directory permissions are too broad")
}

func TestCredentialOperations_RejectInvalidProfileHome(t *testing.T) {
	for _, operation := range []func(string) ([]byte, error){Load, LoadOrCreate, Rotate} {
		_, err := operation("")
		require.EqualError(t, err, "profile home is required")
		_, err = operation("relative")
		require.EqualError(t, err, "profile home must be absolute")
	}
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func encodeCredential(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value) + "\n"
}

package rpcauth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	credentialFilename = "rpc-owner.key"
	credentialBytes    = 32
)

func CredentialPath(profileHome string) string {
	return filepath.Join(strings.TrimSpace(profileHome), credentialFilename)
}

func Load(profileHome string) ([]byte, error) {
	path, err := getCredentialPath(profileHome)
	if err != nil {
		return nil, err
	}

	if err := checkCredentialPermissions(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RPC owner credential: %w", err)
	}

	credential, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(credential) != credentialBytes {
		return nil, errors.New("RPC owner credential is invalid")
	}

	return credential, nil
}

func LoadOrCreate(profileHome string) ([]byte, error) {
	path, err := getCredentialPath(profileHome)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create RPC owner credential directory: %w", err)
	}
	if err := protectCredentialDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}

	credential, err := newCredential()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, loadErr := Load(profileHome)
			if loadErr == nil {
				return existing, nil
			}

			return nil, fmt.Errorf("load existing RPC owner credential: %w", loadErr)
		}
		return nil, fmt.Errorf("create RPC owner credential: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(credential) + "\n"
	_, writeErr := file.WriteString(encoded)
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return nil, errors.Join(writeErr, syncErr, closeErr)
	}
	if err := protectCredentialFile(path); err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	return credential, nil
}

func Rotate(profileHome string) ([]byte, error) {
	path, err := getCredentialPath(profileHome)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create RPC owner credential directory: %w", err)
	}
	if err := protectCredentialDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}

	credential, err := newCredential()
	if err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".rpc-owner-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary RPC owner credential: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(credential) + "\n"
	if _, err := temporary.WriteString(encoded); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := protectCredentialFile(temporaryPath); err != nil {
		return nil, err
	}
	if err := replaceCredentialFile(temporaryPath, path); err != nil {
		return nil, fmt.Errorf("replace RPC owner credential: %w", err)
	}
	if err := protectCredentialFile(path); err != nil {
		return nil, err
	}

	return credential, nil
}

func getCredentialPath(profileHome string) (string, error) {
	profileHome = strings.TrimSpace(profileHome)
	if profileHome == "" {
		return "", errors.New("profile home is required")
	}
	if !filepath.IsAbs(profileHome) {
		return "", errors.New("profile home must be absolute")
	}

	return CredentialPath(profileHome), nil
}

func newCredential() ([]byte, error) {
	credential := make([]byte, credentialBytes)
	if _, err := rand.Read(credential); err != nil {
		return nil, fmt.Errorf("generate RPC owner credential: %w", err)
	}

	return credential, nil
}

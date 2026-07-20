//go:build !windows

package rpcauth

import (
	"errors"
	"os"
	"path/filepath"
)

func protectCredentialDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func protectCredentialFile(path string) error {
	return os.Chmod(path, 0o600)
}

func checkCredentialPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("RPC owner credential permissions are too broad")
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if directory.Mode().Perm()&0o077 != 0 {
		return errors.New("RPC owner credential directory permissions are too broad")
	}

	return nil
}

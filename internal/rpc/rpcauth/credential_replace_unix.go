//go:build !windows

package rpcauth

import "os"

func replaceCredentialFile(source, destination string) error {
	return os.Rename(source, destination)
}

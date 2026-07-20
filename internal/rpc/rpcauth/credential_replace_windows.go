//go:build windows

package rpcauth

import "golang.org/x/sys/windows"

func replaceCredentialFile(source, destination string) error {
	return windows.Rename(source, destination)
}

//go:build windows

package browser

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isArtifactExportLinkUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION)
}

//go:build !windows

package browser

import (
	"errors"
	"syscall"
)

func isArtifactExportLinkUnsupported(err error) bool {
	// Linux may report EPERM both for unsupported hard links and genuine permission failures, so it remains fail-closed.
	return errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.ENOSYS)
}

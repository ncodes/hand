//go:build !windows

package browser

import (
	"fmt"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsArtifactExportLinkUnsupported_MatchesOnlyUnambiguousErrors(t *testing.T) {
	require.True(t, isArtifactExportLinkUnsupported(syscall.EOPNOTSUPP))
	require.True(t, isArtifactExportLinkUnsupported(fmt.Errorf("link export: %w", syscall.ENOSYS)))
	require.False(t, isArtifactExportLinkUnsupported(syscall.EPERM))
	require.False(t, isArtifactExportLinkUnsupported(syscall.EACCES))
}

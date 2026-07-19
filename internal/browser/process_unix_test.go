//go:build !windows

package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBrowserProcess_StopKillsDescendantsAfterLeaderExits(t *testing.T) {
	childFile := filepath.Join(t.TempDir(), "child.pid")
	command := exec.Command("sh", "-c", `sleep 30 & echo $! > "$1"`, "sh", childFile)
	process := newBrowserProcess()
	process.configure(command)
	require.NoError(t, command.Start())
	require.NoError(t, process.attach())
	require.NoError(t, command.Wait())
	rawPID, err := os.ReadFile(childFile)
	require.NoError(t, err)
	childPID, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
	require.NoError(t, syscall.Kill(childPID, 0))

	require.NoError(t, process.stop())
	require.Eventually(t, func() bool {
		err := syscall.Kill(childPID, 0)
		return errors.Is(err, syscall.ESRCH)
	}, 2*time.Second, 20*time.Millisecond)
}

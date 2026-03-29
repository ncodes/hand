//go:build windows

package native

import "os/exec"

func configureCommandProcess(cmd *exec.Cmd) {}

func terminateCommandProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

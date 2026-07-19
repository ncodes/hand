//go:build !windows

package browser

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type browserProcess struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	pgid int
}

func newBrowserProcess() *browserProcess {
	return &browserProcess{}
}

func (p *browserProcess) configure(command *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	p.cmd = command
}

func (p *browserProcess) attach() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("browser process did not start")
	}
	pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
	if err != nil {
		return err
	}
	p.pgid = pgid

	return nil
}

func (p *browserProcess) stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if p.pgid == 0 {
		err := p.cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	err := syscall.Kill(-p.pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	return err
}

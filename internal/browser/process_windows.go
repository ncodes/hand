//go:build windows

package browser

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type browserProcess struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	job windows.Handle
}

func newBrowserProcess() *browserProcess {
	return &browserProcess{}
}

func (p *browserProcess) configure(command *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	p.cmd = command
}

func (p *browserProcess) attach() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("browser process did not start")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(p.cmd.Process.Pid),
	)
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return err
	}
	p.job = job

	return nil
}

func (p *browserProcess) stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		err := windows.CloseHandle(p.job)
		p.job = 0
		return err
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	return p.cmd.Process.Kill()
}

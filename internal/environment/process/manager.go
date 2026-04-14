package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultOutputBufferBytes = 64 * 1024
const DefaultMaxTracked = 32
const DefaultStopGracePeriod = 2 * time.Second

var commandContext = exec.CommandContext
var currentGOOS = goruntime.GOOS

type Manager interface {
	Start(context.Context, StartRequest) (Info, error)
	Get(string) (Info, error)
	Read(string) (Output, error)
	Stop(context.Context, string) (Info, error)
	List() []Info
}

type DefaultManager struct {
	mu              sync.Mutex
	processes       map[string]*trackedProcess
	order           []string
	stale           map[string]struct{}
	nextID          uint64
	MaxTracked      int
	StopGracePeriod time.Duration
}

type trackedProcess struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdout  *recentBuffer
	stderr  *recentBuffer
	info    Info
	waitErr error
	done    chan struct{}
}

type recentBuffer struct {
	mu         sync.Mutex
	limit      int
	data       []byte
	truncated  bool
	totalBytes int
}

func (s *DefaultManager) Start(ctx context.Context, req StartRequest) (Info, error) {
	if s == nil {
		return Info{}, errors.New("process manager is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	command := strings.TrimSpace(req.Command)
	if command == "" {
		return Info{}, errors.New("command is required")
	}

	cmd := buildCommand(context.Background(), command, req.Args)
	configureCommand(cmd)

	cmd.Dir = strings.TrimSpace(req.CWD)
	cmd.Env = os.Environ()
	for key, value := range req.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	limit := req.OutputBufferBytes
	if limit <= 0 {
		limit = DefaultOutputBufferBytes
	}

	stdout := &recentBuffer{limit: limit}
	stderr := &recentBuffer{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return Info{}, err
	}

	processID := s.nextProcessID()
	startedAt := time.Now().UTC()
	process := &trackedProcess{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
		info: Info{
			ID:        processID,
			Command:   command,
			Args:      append([]string(nil), req.Args...),
			CWD:       strings.TrimSpace(req.CWD),
			Status:    StatusRunning,
			StartedAt: startedAt,
		},
	}

	s.mu.Lock()
	if s.processes == nil {
		s.processes = make(map[string]*trackedProcess)
	}

	if s.stale == nil {
		s.stale = make(map[string]struct{})
	}

	s.cleanupLocked()

	if limit := s.maxTracked(); limit > 0 && len(s.processes) >= limit {
		s.mu.Unlock()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return Info{}, errors.New("process manager is at capacity")
	}

	delete(s.stale, processID)

	s.processes[processID] = process
	s.order = append(s.order, processID)
	s.mu.Unlock()

	go s.wait(process)

	return process.snapshot(), nil
}

func (s *DefaultManager) Get(processID string) (Info, error) {
	process, err := s.lookup(processID)
	if err != nil {
		return Info{}, err
	}

	return process.snapshot(), nil
}

func (s *DefaultManager) Read(processID string) (Output, error) {
	process, err := s.lookup(processID)
	if err != nil {
		return Output{}, err
	}

	return process.output(), nil
}

func (s *DefaultManager) Stop(ctx context.Context, processID string) (Info, error) {
	process, err := s.lookup(processID)
	if err != nil {
		return Info{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	process.mu.Lock()
	cmd := process.cmd
	status := process.info.Status
	if cmd != nil && cmd.Process != nil && status == StatusRunning {
		process.info.Status = StatusStopped
	}
	process.mu.Unlock()

	if cmd == nil || cmd.Process == nil || status != StatusRunning {
		return process.snapshot(), nil
	}

	terminateCommandGracefully(cmd)
	select {
	case <-process.done:
		return process.snapshot(), nil
	case <-time.After(s.stopGracePeriod()):
	case <-ctx.Done():
		return process.snapshot(), ctx.Err()
	}

	terminateCommand(cmd)
	select {
	case <-process.done:
	case <-time.After(s.stopGracePeriod()):
	case <-ctx.Done():
		return process.snapshot(), ctx.Err()
	}

	return process.snapshot(), nil
}

func (s *DefaultManager) List() []Info {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	order := append([]string(nil), s.order...)
	processes := make([]*trackedProcess, 0, len(order))
	for _, processID := range order {
		if process := s.processes[processID]; process != nil {
			processes = append(processes, process)
		}
	}
	s.mu.Unlock()

	infos := make([]Info, 0, len(processes))
	for _, process := range processes {
		infos = append(infos, process.snapshot())
	}

	return infos
}

func (s *DefaultManager) wait(process *trackedProcess) {
	err := process.cmd.Wait()

	process.mu.Lock()
	process.waitErr = err
	endedAt := time.Now().UTC()
	process.info.EndedAt = &endedAt
	process.info.StdoutBytes = process.stdout.total()
	process.info.StderrBytes = process.stderr.total()
	process.info.StdoutTruncated = process.stdout.wasTruncated()
	process.info.StderrTruncated = process.stderr.wasTruncated()

	if err == nil {
		exitCode := 0
		process.info.ExitCode = &exitCode
		process.info.Status = StatusExited
		process.mu.Unlock()
		if process.done != nil {
			close(process.done)
		}
		return
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode := exitErr.ExitCode()
		process.info.ExitCode = &exitCode
		if process.info.Status == StatusStopped {
			process.mu.Unlock()
			if process.done != nil {
				close(process.done)
			}
			return
		}
		process.info.Status = StatusExited
		process.mu.Unlock()
		if process.done != nil {
			close(process.done)
		}
		return
	}

	process.info.Status = StatusFailed
	process.mu.Unlock()
	if process.done != nil {
		close(process.done)
	}
}

func (s *DefaultManager) lookup(processID string) (*trackedProcess, error) {
	if s == nil {
		return nil, errors.New("process manager is required")
	}

	processID = strings.TrimSpace(processID)
	if processID == "" {
		return nil, errors.New("process id is required")
	}

	s.mu.Lock()
	process := s.processes[processID]
	_, stale := s.stale[processID]
	if stale && process == nil {
		delete(s.stale, processID)
	}
	s.mu.Unlock()

	if process == nil {
		if stale {
			return nil, errors.New("process is no longer retained")
		}
		return nil, errors.New("process not found")
	}

	return process, nil
}

func (s *DefaultManager) nextProcessID() string {
	id := atomic.AddUint64(&s.nextID, 1)
	return "proc_" + strconv.FormatUint(id, 10)
}

func (s *DefaultManager) cleanupLocked() {
	if len(s.processes) == 0 {
		return
	}

	order := s.order[:0]
	for _, processID := range s.order {
		process := s.processes[processID]
		if process == nil {
			continue
		}
		if process.finished() {
			delete(s.processes, processID)
			if s.stale == nil {
				s.stale = make(map[string]struct{})
			}
			s.stale[processID] = struct{}{}
			continue
		}
		order = append(order, processID)
	}
	s.order = order
}

func (s *DefaultManager) maxTracked() int {
	if s == nil || s.MaxTracked <= 0 {
		return DefaultMaxTracked
	}
	return s.MaxTracked
}

func (s *DefaultManager) stopGracePeriod() time.Duration {
	if s == nil || s.StopGracePeriod <= 0 {
		return DefaultStopGracePeriod
	}
	return s.StopGracePeriod
}

func (p *trackedProcess) snapshot() Info {
	p.mu.Lock()
	defer p.mu.Unlock()

	info := p.info
	info.Args = append([]string(nil), p.info.Args...)
	info.StdoutBytes = p.stdout.total()
	info.StderrBytes = p.stderr.total()
	info.StdoutTruncated = p.stdout.wasTruncated()
	info.StderrTruncated = p.stderr.wasTruncated()
	if p.info.EndedAt != nil {
		endedAt := *p.info.EndedAt
		info.EndedAt = &endedAt
	}
	if p.info.ExitCode != nil {
		exitCode := *p.info.ExitCode
		info.ExitCode = &exitCode
	}

	return info
}

func (p *trackedProcess) output() Output {
	return Output{
		Stdout:          p.stdout.string(),
		Stderr:          p.stderr.string(),
		StdoutBytes:     p.stdout.total(),
		StderrBytes:     p.stderr.total(),
		StdoutTruncated: p.stdout.wasTruncated(),
		StderrTruncated: p.stderr.wasTruncated(),
	}
}

func (p *trackedProcess) finished() bool {
	if p == nil {
		return true
	}

	if p.done != nil {
		select {
		case <-p.done:
			return true
		default:
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.info.Status != StatusRunning
}

func (b *recentBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalBytes += len(data)
	if b.limit <= 0 {
		b.data = append(b.data, data...)
		return len(data), nil
	}

	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.truncated = true
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}

	return len(data), nil
}

func (b *recentBuffer) string() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return string(append([]byte(nil), b.data...))
}

func (b *recentBuffer) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.totalBytes
}

func (b *recentBuffer) wasTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.truncated
}

func buildCommand(ctx context.Context, command string, args []string) *exec.Cmd {
	command = strings.TrimSpace(command)
	if len(args) > 0 {
		return commandContext(ctx, command, args...)
	}

	if currentGOOS == "windows" {
		return commandContext(ctx, "cmd", "/C", command)
	}

	return commandContext(ctx, "sh", "-lc", command)
}

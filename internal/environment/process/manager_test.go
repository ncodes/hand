package process

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestManager_StartGetReadListAndExit(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), testPrintRequest("hello", 32))
	require.NoError(t, err)
	require.Equal(t, StatusRunning, info.Status)
	require.NotEmpty(t, info.ID)

	require.Eventually(t, func() bool {
		current, err := manager.Get(info.ID)
		require.NoError(t, err)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	current, err := manager.Get(info.ID)
	require.NoError(t, err)
	require.Equal(t, StatusExited, current.Status)
	require.NotNil(t, current.ExitCode)
	require.Equal(t, 0, *current.ExitCode)
	require.NotNil(t, current.EndedAt)

	output, err := manager.Read(info.ID)
	require.NoError(t, err)
	require.Equal(t, "hello", output.Stdout)
	require.Empty(t, output.Stderr)
	require.Equal(t, len("hello"), output.StdoutBytes)

	list := manager.List()
	require.Len(t, list, 1)
	require.Equal(t, info.ID, list[0].ID)
}

func TestManager_BoundsRecentOutput(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), testPrintRequest("abcdef", 3))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, err := manager.Get(info.ID)
		require.NoError(t, err)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	output, err := manager.Read(info.ID)
	require.NoError(t, err)
	require.Equal(t, "def", output.Stdout)
	require.True(t, output.StdoutTruncated)
	require.Equal(t, len("abcdef"), output.StdoutBytes)
}

func TestManager_StopMarksStopped(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), testSleepRequest())
	require.NoError(t, err)

	stopped, err := manager.Stop(context.Background(), info.ID)
	require.NoError(t, err)
	require.Equal(t, StatusStopped, stopped.Status)

	require.Eventually(t, func() bool {
		current, err := manager.Get(info.ID)
		require.NoError(t, err)
		return current.Status == StatusStopped && current.EndedAt != nil
	}, 5*time.Second, 20*time.Millisecond)
}

func TestManager_StopAcceptsNilContext(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), testSleepRequest())
	require.NoError(t, err)

	stopped, err := manager.Stop(nil, info.ID)
	require.NoError(t, err)
	require.Equal(t, StatusStopped, stopped.Status)
}

func TestManager_StartDetachesFromCallerContextAfterLaunch(t *testing.T) {
	manager := &DefaultManager{}

	ctx, cancel := context.WithCancel(context.Background())
	info, err := manager.Start(ctx, testSleepRequest())
	require.NoError(t, err)

	cancel()

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusRunning
	}, time.Second, 20*time.Millisecond)

	_, err = manager.Stop(context.Background(), info.ID)
	require.NoError(t, err)
}

func TestManager_ValidatesMissingProcessAndCommand(t *testing.T) {
	manager := &DefaultManager{}

	_, err := manager.Start(context.Background(), StartRequest{})
	require.EqualError(t, err, "command is required")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = manager.Start(canceledCtx, testPrintRequest("hello", 32))
	require.EqualError(t, err, context.Canceled.Error())

	_, err = manager.Get(" ")
	require.EqualError(t, err, "process id is required")

	_, err = manager.Read("missing")
	require.EqualError(t, err, "process not found")

	_, err = manager.Stop(context.Background(), "missing")
	require.EqualError(t, err, "process not found")
}

func TestManager_StartHandlesNilContextAndStartFailure(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(nil, testPrintRequest("hello", 32))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	_, err = manager.Start(context.Background(), StartRequest{
		Command: "command-that-does-not-exist-hand",
		Args:    []string{"arg"},
	})
	require.Error(t, err)
}

func TestManager_StartAppliesEnvOverrides(t *testing.T) {
	manager := &DefaultManager{}

	req := StartRequest{
		Command: "sh",
		Args:    []string{"-lc", "printf %s \"$HAND_PROCESS_TEST_VALUE\""},
		Env: map[string]string{
			"HAND_PROCESS_TEST_VALUE": "hello",
		},
		OutputBufferBytes: 32,
	}
	if runtime.GOOS == "windows" {
		req = StartRequest{
			Command: "cmd",
			Args:    []string{"/C", "set /p =%HAND_PROCESS_TEST_VALUE%<nul"},
			Env: map[string]string{
				"HAND_PROCESS_TEST_VALUE": "hello",
			},
			OutputBufferBytes: 32,
		}
	}

	info, err := manager.Start(context.Background(), req)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	output, err := manager.Read(info.ID)
	require.NoError(t, err)
	require.Equal(t, "hello", output.Stdout)
}

func TestManager_WaitMarksExitedForNonZeroExitCode(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), StartRequest{Command: "false"})
	if runtime.GOOS == "windows" {
		info, err = manager.Start(context.Background(), StartRequest{
			Command: "cmd",
			Args:    []string{"/C", "exit 2"},
		})
	}
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusExited && current.ExitCode != nil && *current.ExitCode != 0
	}, 5*time.Second, 20*time.Millisecond)
}

func TestManager_HandlesNilReceiver(t *testing.T) {
	var manager *DefaultManager

	_, err := manager.Start(context.Background(), StartRequest{Command: "printf", Args: []string{"hello"}})
	require.EqualError(t, err, "process manager is required")

	_, err = manager.Get("proc_1")
	require.EqualError(t, err, "process manager is required")

	_, err = manager.Read("proc_1")
	require.EqualError(t, err, "process manager is required")

	_, err = manager.Stop(context.Background(), "proc_1")
	require.EqualError(t, err, "process manager is required")

	require.Nil(t, manager.List())
}

func TestManager_StopReturnsExistingSnapshotWhenAlreadyNotRunning(t *testing.T) {
	manager := &DefaultManager{}

	info, err := manager.Start(context.Background(), testPrintRequest("hello", 32))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	stopped, err := manager.Stop(context.Background(), info.ID)
	require.NoError(t, err)
	require.Equal(t, StatusExited, stopped.Status)
}

func TestManager_StartRejectsWhenCapacityIsReachedByRunningProcess(t *testing.T) {
	manager := &DefaultManager{MaxTracked: 1}

	info, err := manager.Start(context.Background(), testSleepRequest())
	require.NoError(t, err)

	_, err = manager.Start(context.Background(), testSleepRequest())
	require.EqualError(t, err, "process manager is at capacity")

	_, stopErr := manager.Stop(context.Background(), info.ID)
	require.NoError(t, stopErr)
}

func TestManager_StartCleansUpFinishedProcessesAndMarksStaleIDs(t *testing.T) {
	manager := &DefaultManager{MaxTracked: 1}

	info, err := manager.Start(context.Background(), testPrintRequest("hello", 32))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.Status == StatusExited
	}, 5*time.Second, 20*time.Millisecond)

	next, err := manager.Start(context.Background(), testSleepRequest())
	require.NoError(t, err)

	_, err = manager.Get(info.ID)
	require.EqualError(t, err, "process is no longer retained")

	list := manager.List()
	require.Len(t, list, 1)
	require.Equal(t, next.ID, list[0].ID)

	_, stopErr := manager.Stop(context.Background(), next.ID)
	require.NoError(t, stopErr)
}

func TestManager_CleanupTreatsDoneProcessAsFinishedBeforeStatusUpdate(t *testing.T) {
	manager := &DefaultManager{
		MaxTracked: 1,
		processes: map[string]*trackedProcess{
			"proc_1": {
				done: make(chan struct{}),
				info: Info{
					ID:     "proc_1",
					Status: StatusRunning,
				},
			},
		},
		order: []string{"proc_1"},
		stale: map[string]struct{}{},
	}
	close(manager.processes["proc_1"].done)

	manager.cleanupLocked()

	require.Empty(t, manager.processes)
	require.Empty(t, manager.order)
	_, ok := manager.stale["proc_1"]
	require.True(t, ok)
}

func TestManager_CleanupInitializesStaleMapForFinishedProcess(t *testing.T) {
	manager := &DefaultManager{
		processes: map[string]*trackedProcess{
			"proc_1": {
				info: Info{
					ID:     "proc_1",
					Status: StatusExited,
				},
			},
		},
		order: []string{"proc_1"},
	}

	manager.cleanupLocked()

	require.Empty(t, manager.processes)
	require.Empty(t, manager.order)
	_, ok := manager.stale["proc_1"]
	require.True(t, ok)
}

func TestManager_CleanupHandlesEmptyAndNilTrackedEntries(t *testing.T) {
	manager := &DefaultManager{}
	manager.cleanupLocked()

	manager = &DefaultManager{
		processes: map[string]*trackedProcess{
			"proc_1": nil,
		},
		order: []string{"proc_1"},
	}
	manager.cleanupLocked()

	require.Empty(t, manager.order)
	require.Len(t, manager.processes, 1)
}

func TestManager_ListSkipsNilTrackedProcessEntries(t *testing.T) {
	manager := &DefaultManager{
		processes: map[string]*trackedProcess{
			"proc_1": nil,
		},
		order: []string{"proc_1"},
	}

	require.Empty(t, manager.List())
}

func TestManager_WaitMarksFailedWhenWaitDoesNotReturnExitError(t *testing.T) {
	manager := &DefaultManager{}
	process := &trackedProcess{
		cmd:    exec.Command("printf", "hello"),
		done:   make(chan struct{}),
		stdout: &recentBuffer{limit: 16},
		stderr: &recentBuffer{limit: 16},
		info: Info{
			ID:     "proc_1",
			Status: StatusRunning,
		},
	}

	manager.wait(process)

	info := process.snapshot()
	require.Equal(t, StatusFailed, info.Status)
	require.NotNil(t, info.EndedAt)
	require.Nil(t, info.ExitCode)
	select {
	case <-process.done:
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestManager_StopFallsBackAfterGracePeriodAndReturnsSnapshot(t *testing.T) {
	manager := &DefaultManager{
		StopGracePeriod: 10 * time.Millisecond,
		processes: map[string]*trackedProcess{
			"proc_1": {
				cmd:    &exec.Cmd{Process: &os.Process{Pid: 999999}},
				stdout: &recentBuffer{},
				stderr: &recentBuffer{},
				info: Info{
					ID:     "proc_1",
					Status: StatusRunning,
				},
				done: make(chan struct{}),
			},
		},
		order: []string{"proc_1"},
	}

	stopped, err := manager.Stop(context.Background(), "proc_1")
	require.NoError(t, err)
	require.Equal(t, StatusStopped, stopped.Status)
}

func TestManager_StopReturnsContextErrorAfterForceKillAttempt(t *testing.T) {
	manager := &DefaultManager{
		StopGracePeriod: 20 * time.Millisecond,
		processes: map[string]*trackedProcess{
			"proc_1": {
				cmd:    &exec.Cmd{Process: &os.Process{Pid: 999999}},
				stdout: &recentBuffer{},
				stderr: &recentBuffer{},
				info: Info{
					ID:     "proc_1",
					Status: StatusRunning,
				},
				done: make(chan struct{}),
			},
		},
		order: []string{"proc_1"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	stopped, err := manager.Stop(ctx, "proc_1")
	require.EqualError(t, err, context.Canceled.Error())
	require.Equal(t, StatusStopped, stopped.Status)
}

func TestManager_FinishedAndStopGracePeriodHelpers(t *testing.T) {
	var nilProcess *trackedProcess
	require.True(t, nilProcess.finished())

	process := &trackedProcess{
		info: Info{Status: StatusExited},
	}
	require.True(t, process.finished())

	manager := &DefaultManager{}
	require.Equal(t, DefaultStopGracePeriod, manager.stopGracePeriod())

	manager.StopGracePeriod = 50 * time.Millisecond
	require.Equal(t, 50*time.Millisecond, manager.stopGracePeriod())
}

func TestRecentBuffer_WriteWithoutLimit(t *testing.T) {
	buffer := &recentBuffer{}

	written, err := buffer.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, written)
	require.Equal(t, "hello", buffer.string())
	require.False(t, buffer.wasTruncated())
	require.Equal(t, 5, buffer.total())
}

func TestBuildCommand_UsesShellWhenArgsAreOmitted(t *testing.T) {
	cmd := buildCommand(context.Background(), "echo hello", nil)

	if runtime.GOOS == "windows" {
		require.Equal(t, "cmd", cmd.Path)
		require.Equal(t, []string{"cmd", "/C", "echo hello"}, cmd.Args)
		return
	}

	require.Equal(t, []string{"sh", "-lc", "echo hello"}, cmd.Args)
}

func TestBuildCommand_UsesDirectExecutionWhenArgsProvided(t *testing.T) {
	cmd := buildCommand(context.Background(), "printf", []string{"hello"})

	require.Equal(t, "printf", cmd.Args[0])
	require.Equal(t, []string{"printf", "hello"}, cmd.Args)
}

func TestBuildCommand_UsesWindowsShellWhenConfigured(t *testing.T) {
	previousGOOS := currentGOOS
	t.Cleanup(func() {
		currentGOOS = previousGOOS
	})
	currentGOOS = "windows"

	cmd := buildCommand(context.Background(), "echo hello", nil)

	require.Equal(t, []string{"cmd", "/C", "echo hello"}, cmd.Args)
}

func TestConfigureCommand_HandlesNilCommand(t *testing.T) {
	require.NotPanics(t, func() {
		configureCommand(nil)
	})
}

func TestTerminateCommand_HandlesNilCommandAndProcess(t *testing.T) {
	require.NotPanics(t, func() {
		terminateCommand(nil)
	})

	require.NotPanics(t, func() {
		terminateCommand(&exec.Cmd{})
	})
}

func TestTerminateCommandGracefully_HandlesNilCommandAndProcess(t *testing.T) {
	require.NotPanics(t, func() {
		terminateCommandGracefully(nil)
	})

	require.NotPanics(t, func() {
		terminateCommandGracefully(&exec.Cmd{})
	})
}

func TestManager_StopReturnsContextErrorWhenCanceledWhileWaiting(t *testing.T) {
	manager := &DefaultManager{StopGracePeriod: time.Second}

	info, err := manager.Start(context.Background(), testSleepRequest())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped, err := manager.Stop(ctx, info.ID)
	require.EqualError(t, err, context.Canceled.Error())
	require.Equal(t, StatusStopped, stopped.Status)

	require.Eventually(t, func() bool {
		current, getErr := manager.Get(info.ID)
		require.NoError(t, getErr)
		return current.EndedAt != nil
	}, 5*time.Second, 20*time.Millisecond)
}

func testPrintRequest(output string, bufferBytes int) StartRequest {
	if runtime.GOOS == "windows" {
		return StartRequest{
			Command:           "cmd",
			Args:              []string{"/C", "set /p =" + output + "<nul"},
			OutputBufferBytes: bufferBytes,
		}
	}

	return StartRequest{
		Command:           "printf",
		Args:              []string{output},
		OutputBufferBytes: bufferBytes,
	}
}

func testSleepRequest() StartRequest {
	if runtime.GOOS == "windows" {
		return StartRequest{
			Command: "cmd",
			Args:    []string{"/C", "ping -n 6 127.0.0.1 >nul"},
		}
	}

	return StartRequest{
		Command: "sleep",
		Args:    []string{"5"},
	}
}

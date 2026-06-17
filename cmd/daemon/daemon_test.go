package daemon

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handcli "github.com/wandxy/hand/internal/cli"
)

var errDaemonTestWrite = errors.New("write failed")

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errDaemonTestWrite
}

func TestSetOutputReturnsPreviousAndDiscardsNil(t *testing.T) {
	originalOutput := daemonOutput
	originalStartupOutput := handcli.SetDaemonOutput(io.Discard)
	t.Cleanup(func() {
		daemonOutput = originalOutput
		handcli.SetDaemonOutput(originalStartupOutput)
	})
	var output bytes.Buffer

	previous := SetOutput(&output)
	require.Same(t, originalOutput, previous)
	previous = SetOutput(nil)
	require.Same(t, &output, previous)

	_, err := daemonOutput.Write([]byte("discarded"))
	require.NoError(t, err)
	require.Empty(t, output.String())
}

func TestStatusCommandPrintsDaemonStatus(t *testing.T) {
	originalGetDaemonStatus := getDaemonStatus
	originalOutput := daemonOutput
	t.Cleanup(func() {
		getDaemonStatus = originalGetDaemonStatus
		daemonOutput = originalOutput
	})

	startedAt := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	var output bytes.Buffer
	daemonOutput = &output
	getDaemonStatus = func(context.Context) (handcli.DaemonStatus, error) {
		return handcli.DaemonStatus{
			State:     "running",
			Health:    "SERVING",
			Profile:   "work",
			PID:       1234,
			Address:   "127.0.0.1",
			Port:      50051,
			StartedAt: startedAt,
			Uptime:    90 * time.Second,
		}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"daemon", "status"})

	require.NoError(t, err)
	require.Equal(
		t,
		"state=running health=SERVING profile=work pid=1234 rpc=127.0.0.1:50051 uptime=1m30s started_at=2026-06-16T10:00:00Z\n",
		output.String(),
	)
}

func TestStatusCommandReturnsStatusErrorAfterPrintingStatus(t *testing.T) {
	originalGetDaemonStatus := getDaemonStatus
	originalOutput := daemonOutput
	t.Cleanup(func() {
		getDaemonStatus = originalGetDaemonStatus
		daemonOutput = originalOutput
	})

	expectedErr := errors.New("daemon is missing")
	var output bytes.Buffer
	daemonOutput = &output
	getDaemonStatus = func(context.Context) (handcli.DaemonStatus, error) {
		return handcli.DaemonStatus{State: "missing", Profile: "default"}, expectedErr
	}

	err := NewCommand().Run(context.Background(), []string{"daemon", "status"})

	require.ErrorIs(t, err, expectedErr)
	require.Equal(
		t,
		"state=missing health=- profile=default pid=0 rpc=-:0 uptime=0s started_at=-\n",
		output.String(),
	)
}

func TestStatusCommandReturnsWriteError(t *testing.T) {
	originalGetDaemonStatus := getDaemonStatus
	originalOutput := daemonOutput
	t.Cleanup(func() {
		getDaemonStatus = originalGetDaemonStatus
		daemonOutput = originalOutput
	})

	daemonOutput = errWriter{}
	getDaemonStatus = func(context.Context) (handcli.DaemonStatus, error) {
		return handcli.DaemonStatus{State: "running"}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"daemon", "status"})

	require.ErrorIs(t, err, errDaemonTestWrite)
}

func TestStartSubcommandIsNotAccepted(t *testing.T) {
	originalGetDaemonStatus := getDaemonStatus
	t.Cleanup(func() {
		getDaemonStatus = originalGetDaemonStatus
	})
	getDaemonStatus = func(context.Context) (handcli.DaemonStatus, error) {
		t.Fatal("status should not run for start")
		return handcli.DaemonStatus{}, nil
	}

	err := NewCommand().Run(context.Background(), []string{"daemon", "start"})

	require.Error(t, err)
}

func TestWriteDaemonStatusReturnsWriteError(t *testing.T) {
	err := writeDaemonStatus(errWriter{}, handcli.DaemonStatus{State: "running"})

	require.ErrorIs(t, err, errDaemonTestWrite)
}

func TestWriteDaemonStatusDefaultsEmptyState(t *testing.T) {
	var output bytes.Buffer

	err := writeDaemonStatus(&output, handcli.DaemonStatus{})

	require.NoError(t, err)
	require.Contains(t, output.String(), "state=unknown")
}

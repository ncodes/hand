package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatusModel_TextFallsBackToDefaultAndSubmitHint(t *testing.T) {
	status := newStatusModel()
	require.Equal(t, defaultStatus, status.Text())

	setStatusDefault(&status, "")
	require.Equal(t, statusReadySuffix, status.Text())
}

func TestStatusModel_SetTransientExpiresMatchingStatus(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	status := newStatusModel()
	cmd := setStatusTransient(&status, "saved")

	require.NotNil(t, cmd)
	require.True(t, statusHasTransient(status))
	require.Equal(t, "saved", status.Text())

	expireStatus(&status, statusExpiredMsg{startedAt: now.Add(time.Second)})
	require.Equal(t, "saved", status.Text())

	expireStatus(&status, statusExpiredMsg{startedAt: now})
	require.False(t, statusHasTransient(status))
	require.Equal(t, defaultStatus, status.Text())
}

func TestStatusModel_SetTransientBlankClearsStatus(t *testing.T) {
	status := newStatusModel()
	status.SetTransient("saved", time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	cmd := setStatusTransient(&status, " ")

	require.Nil(t, cmd)
	require.False(t, statusHasTransient(status))
	require.Equal(t, defaultStatus, status.Text())
}

func TestStatusModel_SetTransientUsesDefaultWindowWhenUnset(t *testing.T) {
	status := newStatusModel()
	status.SetHideAfter(0)

	cmd := setStatusTransient(&status, "saved")

	require.NotNil(t, cmd)
	require.True(t, statusHasTransient(status))
	require.Equal(t, "saved", status.Text())
}

func TestStatusModel_SetTransientExpirationCommandReturnsStartedAt(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() {
		currentTime = originalCurrentTime
	})
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return now
	}

	status := newStatusModel()
	status.SetHideAfter(time.Nanosecond)
	cmd := setStatusTransient(&status, "saved")

	require.NotNil(t, cmd)
	require.Equal(t, statusExpiredMsg{startedAt: now}, cmd())
}

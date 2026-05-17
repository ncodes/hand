package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatusModel_TextFallsBackToDefaultAndReady(t *testing.T) {
	status := newStatusModel()
	require.Equal(t, defaultStatus, status.Text())

	status.defaultText = ""
	require.Equal(t, "ready", status.Text())
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
	cmd := status.setTransient("saved")

	require.NotNil(t, cmd)
	require.True(t, status.hasTransient())
	require.Equal(t, "saved", status.Text())

	status.expire(statusExpiredMsg{startedAt: now.Add(time.Second)})
	require.Equal(t, "saved", status.Text())

	status.expire(statusExpiredMsg{startedAt: now})
	require.False(t, status.hasTransient())
	require.Equal(t, defaultStatus, status.Text())
}

func TestStatusModel_SetTransientBlankClearsStatus(t *testing.T) {
	status := newStatusModel()
	status.text = "saved"
	status.startedAt = time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	cmd := status.setTransient(" ")

	require.Nil(t, cmd)
	require.False(t, status.hasTransient())
	require.Equal(t, defaultStatus, status.Text())
}

func TestStatusModel_SetTransientUsesDefaultWindowWhenUnset(t *testing.T) {
	status := newStatusModel()
	status.hideAfter = 0

	cmd := status.setTransient("saved")

	require.NotNil(t, cmd)
	require.True(t, status.hasTransient())
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
	status.hideAfter = time.Nanosecond
	cmd := status.setTransient("saved")

	require.NotNil(t, cmd)
	require.Equal(t, statusExpiredMsg{startedAt: now}, cmd())
}

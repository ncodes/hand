package status

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModel_TextFallsBackToDefaultAndSubmitHint(t *testing.T) {
	status := New()
	require.Equal(t, DefaultText, status.Text())

	status.defaultText = ""
	require.Equal(t, ReadySuffix, status.Text())
}

func TestModel_SetTransientExpiresMatchingStatus(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	status := New()

	require.True(t, status.SetTransient("saved", now))
	require.True(t, status.HasTransient())
	require.Equal(t, "saved", status.Text())
	require.Equal(t, now, status.StartedAt())

	status.Expire(now.Add(time.Second))
	require.Equal(t, "saved", status.Text())

	status.Expire(now)
	require.False(t, status.HasTransient())
	require.Equal(t, DefaultText, status.Text())
}

func TestModel_SetTransientBlankClearsStatus(t *testing.T) {
	status := New()
	status.text = "saved"
	status.startedAt = time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	require.False(t, status.SetTransient(" ", time.Now()))
	require.False(t, status.HasTransient())
	require.Equal(t, DefaultText, status.Text())
}

func TestModel_HideAfterUsesDefaultWindowWhenUnset(t *testing.T) {
	status := New()
	status.hideAfter = 0

	require.Equal(t, AutoHideWindow, status.HideAfter())
}

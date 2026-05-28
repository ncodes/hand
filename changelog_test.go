package changelog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLatest_ReturnsEmbeddedLatestSection(t *testing.T) {
	latest := Latest()

	require.True(t, strings.HasPrefix(latest, "## Unreleased"))
	require.Contains(t, latest, "GitHub Copilot")
}

func TestLatestSection_ReturnsFirstSecondLevelSection(t *testing.T) {
	latest := latestSection(`# Changelog

## New

- one

## Old

- two
`)

	require.Equal(t, "## New\n\n- one", latest)
}

func TestLatestSection_FallsBackToTrimmedContent(t *testing.T) {
	require.Equal(t, "plain text", latestSection(" plain text \n"))
}

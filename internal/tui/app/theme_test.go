package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/tui/render"
	"github.com/wandxy/hand/pkg/termtheme"
)

func TestAdaptTUITheme_UsesDerivedDarkPaletteForNonBlackBackground(t *testing.T) {
	theme := adaptTUITheme(render.DefaultTheme, termtheme.Result{
		Theme:      "dark",
		Background: "#1e1e2e",
		Source:     "osc11",
	})

	require.Equal(t, "#2a2a39", theme.UserTranscriptBackground)
	require.Equal(t, "#5d5d68", theme.InputFrameBorder)
	require.Equal(t, "#3d3d4b", theme.MarkdownCodeBackground)
	require.Equal(t, "#61616c", theme.NoticeBorder)
	require.NotEqual(t, render.DefaultTheme.UserTranscriptBackground, theme.UserTranscriptBackground)
}

func TestAdaptTUITheme_KeepsDefaultPaletteForBlackBackground(t *testing.T) {
	theme := adaptTUITheme(render.DefaultTheme, termtheme.Result{
		Theme:      "dark",
		Background: "#000000",
		Source:     "osc11",
	})

	require.Equal(t, render.DefaultTheme, theme)
}

func TestAdaptTUITheme_KeepsDefaultPaletteForLightOrUnknownBackground(t *testing.T) {
	light := adaptTUITheme(render.DefaultTheme, termtheme.Result{
		Theme:      "light",
		Background: "#f5f5f5",
		Source:     "osc11",
	})
	unknown := adaptTUITheme(render.DefaultTheme, termtheme.Result{
		Theme:  "unknown",
		Source: "tty",
		Error:  "missing tty",
	})

	require.Equal(t, render.DefaultTheme, light)
	require.Equal(t, render.DefaultTheme, unknown)
}

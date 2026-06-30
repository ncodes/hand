package layout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPanelPadding_DisablesPaddingWhenNarrow(t *testing.T) {
	require.Equal(t, 0, PanelPadding(2))
	require.Equal(t, PanelHorizontalPadding, PanelPadding(3))
}

func TestPanelContentWidth_SubtractsHorizontalPadding(t *testing.T) {
	require.Equal(t, 1, PanelContentWidth(1))
	require.Equal(t, 98, PanelContentWidth(100))
}

func TestCompute_ReturnsStableRegions(t *testing.T) {
	regions := Compute(100, 30, 2, Metrics{
		MinInputHeight:              1,
		InputChromeHeight:           3,
		InputFrameChromeHeight:      2,
		TranscriptComposerGapHeight: 1,
		BottomStatusPanelHeight:     1,
	})

	require.Equal(t, Rect{X: 1, Y: 0, Width: 98, Height: 25}, regions.Transcript)
	require.Equal(t, Rect{X: 0, Y: 25, Width: 100, Height: 1}, regions.JumpToBottom)
	require.Equal(t, Rect{X: 0, Y: 26, Width: 100, Height: 4}, regions.Composer)
	require.Equal(t, Rect{X: 0, Y: 30, Width: 100, Height: 1}, regions.BottomStatusPanel)
	require.Equal(t, 98, regions.PanelContentWidth)
	require.Equal(t, 1, regions.PanelHorizontalPad)
}

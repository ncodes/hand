package tui

import tuilayout "github.com/wandxy/hand/internal/tui/layout"

const (
	rightSidebarWidth          = 28
	rightSidebarMinMainWidth   = 64
	rightSidebarMinViewportGap = 1
)

type tuiRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type tuiLayout struct {
	Transcript         tuiRect
	JumpToBottom       tuiRect
	Composer           tuiRect
	BottomStatusPanel  tuiRect
	PanelContentWidth  int
	PanelHorizontalPad int
}

func getTUILayout(width int, height int, inputHeight int) tuiLayout {
	return getTUILayoutForSidebar(width, height, inputHeight, true)
}

func getTUILayoutForSidebar(width int, height int, inputHeight int, sidebarVisible bool) tuiLayout {
	width = getMainPaneWidthForSidebar(width, sidebarVisible)

	return tuiLayoutFromRegions(tuilayout.Compute(width, height, inputHeight, tuilayout.Metrics{
		MinInputHeight:              minInputHeight,
		InputChromeHeight:           inputChromeHeight,
		InputFrameChromeHeight:      inputFrameChromeHeight,
		TranscriptComposerGapHeight: transcriptComposerGapHeight,
		BottomStatusPanelHeight:     bottomStatusPanelHeight,
	}))
}

func getMainPaneWidth(width int) int {
	return getMainPaneWidthForSidebar(width, true)
}

func getMainPaneWidthForSidebar(width int, sidebarVisible bool) int {
	width = max(width, 1)
	if !sidebarVisible {
		return width
	}

	sidebarWidth := getRightSidebarWidth(width)
	if sidebarWidth <= 0 {
		return width
	}

	return max(width-sidebarWidth, 1)
}

func getRightSidebarWidth(width int) int {
	width = max(width, 1)
	if width < rightSidebarMinMainWidth+rightSidebarWidth+rightSidebarMinViewportGap {
		return 0
	}

	return min(rightSidebarWidth, width-rightSidebarMinMainWidth)
}

func tuiLayoutFromRegions(regions tuilayout.Regions) tuiLayout {
	return tuiLayout{
		Transcript:         tuiRectFromLayoutRect(regions.Transcript),
		JumpToBottom:       tuiRectFromLayoutRect(regions.JumpToBottom),
		Composer:           tuiRectFromLayoutRect(regions.Composer),
		BottomStatusPanel:  tuiRectFromLayoutRect(regions.BottomStatusPanel),
		PanelContentWidth:  regions.PanelContentWidth,
		PanelHorizontalPad: regions.PanelHorizontalPad,
	}
}

func tuiRectFromLayoutRect(rect tuilayout.Rect) tuiRect {
	return tuiRect{
		X:      rect.X,
		Y:      rect.Y,
		Width:  rect.Width,
		Height: rect.Height,
	}
}

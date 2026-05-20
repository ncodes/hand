package tui

import tuilayout "github.com/wandxy/hand/internal/tuiapp/layout"

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
	return tuiLayoutFromRegions(tuilayout.Compute(width, height, inputHeight, tuilayout.Metrics{
		MinInputHeight:              minInputHeight,
		InputChromeHeight:           inputChromeHeight,
		InputFrameChromeHeight:      inputFrameChromeHeight,
		TranscriptComposerGapHeight: transcriptComposerGapHeight,
		BottomStatusPanelHeight:     bottomStatusPanelHeight,
	}))
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

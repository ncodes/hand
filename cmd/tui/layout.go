package tui

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
	width = max(width, 1)
	height = max(height, 1)
	inputHeight = max(inputHeight, minInputHeight)

	transcriptHeight := max(height-inputHeight-inputChromeHeight, 1)
	composerY := transcriptHeight + transcriptComposerGapHeight
	contentWidth := getPanelContentWidth(width)
	horizontalPadding := getPanelHorizontalPadding(width)

	return tuiLayout{
		Transcript: tuiRect{
			X:      horizontalPadding,
			Y:      0,
			Width:  contentWidth,
			Height: transcriptHeight,
		},
		JumpToBottom: tuiRect{
			X:      0,
			Y:      transcriptHeight,
			Width:  width,
			Height: transcriptComposerGapHeight,
		},
		Composer: tuiRect{
			X:      0,
			Y:      composerY,
			Width:  width,
			Height: inputHeight + inputFrameChromeHeight,
		},
		BottomStatusPanel: tuiRect{
			X:      0,
			Y:      composerY + inputHeight + inputFrameChromeHeight,
			Width:  width,
			Height: bottomStatusPanelHeight,
		},
		PanelContentWidth:  contentWidth,
		PanelHorizontalPad: horizontalPadding,
	}
}

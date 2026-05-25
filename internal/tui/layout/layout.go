package layout

// PanelHorizontalPadding is the package-level panel horizontal padding constant.
const PanelHorizontalPadding = 1

// Rect describes terminal layout geometry.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Regions describes terminal layout geometry.
type Regions struct {
	Transcript         Rect
	JumpToBottom       Rect
	Composer           Rect
	BottomStatusPanel  Rect
	PanelContentWidth  int
	PanelHorizontalPad int
}

// Metrics records metrics for display or diagnostics.
type Metrics struct {
	MinInputHeight              int
	InputChromeHeight           int
	InputFrameChromeHeight      int
	TranscriptComposerGapHeight int
	BottomStatusPanelHeight     int
}

// PanelPadding returns the padding used around a panel.
func PanelPadding(width int) int {
	if width <= PanelHorizontalPadding*2 {
		return 0
	}

	return PanelHorizontalPadding
}

// PanelContentWidth returns the content width remaining inside panel padding.
func PanelContentWidth(width int) int {
	padding := PanelPadding(width)

	return max(width-padding*2, 1)
}

// Compute calculates transcript, composer, and status bar regions for the terminal size.
func Compute(width int, height int, inputHeight int, metrics Metrics) Regions {
	width = max(width, 1)
	height = max(height, 1)
	inputHeight = max(inputHeight, metrics.MinInputHeight)

	transcriptHeight := max(height-inputHeight-metrics.InputChromeHeight, 1)
	composerY := transcriptHeight + metrics.TranscriptComposerGapHeight
	contentWidth := PanelContentWidth(width)
	horizontalPadding := PanelPadding(width)

	return Regions{
		Transcript: Rect{
			X:      horizontalPadding,
			Y:      0,
			Width:  contentWidth,
			Height: transcriptHeight,
		},
		JumpToBottom: Rect{
			X:      0,
			Y:      transcriptHeight,
			Width:  width,
			Height: metrics.TranscriptComposerGapHeight,
		},
		Composer: Rect{
			X:      0,
			Y:      composerY,
			Width:  width,
			Height: inputHeight + metrics.InputFrameChromeHeight,
		},
		BottomStatusPanel: Rect{
			X:      0,
			Y:      composerY + inputHeight + metrics.InputFrameChromeHeight,
			Width:  width,
			Height: metrics.BottomStatusPanelHeight,
		},
		PanelContentWidth:  contentWidth,
		PanelHorizontalPad: horizontalPadding,
	}
}

func max(left int, right int) int {
	if left > right {
		return left
	}

	return right
}

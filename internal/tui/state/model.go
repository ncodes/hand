package state

type Viewport struct {
	Width  int
	Height int
}

func NormalizeViewport(width int, height int) Viewport {
	return Viewport{
		Width:  max(width, 1),
		Height: max(height, 1),
	}
}

func max(left int, right int) int {
	if left > right {
		return left
	}

	return right
}

package state

type ViewportSizeAction struct {
	Width  int
	Height int
}

func (action ViewportSizeAction) Apply(viewport *Viewport) {
	if viewport == nil {
		return
	}

	*viewport = NormalizeViewport(action.Width, action.Height)
}

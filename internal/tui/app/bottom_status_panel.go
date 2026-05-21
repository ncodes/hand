package tui

// renderBottomStatusPanel renders the compact bottom status panel below the composer.
func (m model) renderBottomStatusPanel() string {
	availableWidth := getInputBoxWidth(m.width)
	return defaultBottomStatusPanelRenderer.Render(getBottomStatusPanel(availableWidth, m))
}

type bottomStatusPanel struct {
	Width             int
	HorizontalPadding int
	ContentWidth      int
	ModelName         string
	Status            string
	SessionTitle      string
	Context           string
	Thinking          bool
	ThinkingFrame     int
	ExitConfirmation  bool
}

func getBottomStatusPanel(width int, m model) bottomStatusPanel {
	return bottomStatusPanel{
		Width:             max(width, 1),
		HorizontalPadding: getPanelHorizontalPadding(width),
		ContentWidth:      getPanelContentWidth(width),
		ModelName:         m.modelName,
		Status:            m.bottomStatusText(),
		SessionTitle:      m.sessionTitle,
		Context:           m.context,
		Thinking:          m.isModelThinking(),
		ThinkingFrame:     m.thinkingComposerFrame,
		ExitConfirmation:  m.hasPendingExitConfirmation(),
	}
}

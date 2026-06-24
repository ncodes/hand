package tui

import tuilayout "github.com/wandxy/morph/internal/tui/layout"

const panelHorizontalPadding = tuilayout.PanelHorizontalPadding

func getPanelHorizontalPadding(width int) int {
	return tuilayout.PanelPadding(width)
}

func getPanelContentWidth(width int) int {
	return tuilayout.PanelContentWidth(width)
}

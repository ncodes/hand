package tui

const panelHorizontalPadding = 1

func getPanelHorizontalPadding(width int) int {
	if width <= panelHorizontalPadding*2 {
		return 0
	}

	return panelHorizontalPadding
}

func getPanelContentWidth(width int) int {
	padding := getPanelHorizontalPadding(width)

	return max(width-padding*2, 1)
}

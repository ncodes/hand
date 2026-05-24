package terminalmd

import "charm.land/lipgloss/v2"

func (r *Renderer) style(style Style) lipgloss.Style {
	result := lipgloss.NewStyle()
	if style.Foreground != "" {
		result = result.Foreground(lipgloss.Color(style.Foreground))
	}
	if style.Background != "" {
		result = result.Background(lipgloss.Color(style.Background))
	}
	if style.Bold {
		result = result.Bold(true)
	}
	if style.Italic {
		result = result.Italic(true)
	}
	if style.Underline {
		result = result.Underline(true)
	}
	if style.Strike {
		result = result.Strikethrough(true)
	}
	return result
}

// renderHyperlink wraps styled visible text in an OSC 8 hyperlink when enabled.
//
// OSC 8 is the terminal convention for clickable links:
// ESC ] 8 ;; URL BEL visible text ESC ] 8 ;; BEL
//
// The visible text is already styled by the caller. The URL is sanitized and
// restricted to link schemes terminals commonly open safely. The BEL terminator
// is used because it has the broadest support across terminal emulators,

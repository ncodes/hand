package tui

import tea "charm.land/bubbletea/v2"

type tuiEffect interface {
	tuiEffect()
}

type sendPromptEffect struct {
	Text string
}

type copyTranscriptEffect struct {
	Text string
}

type openLinkEffect struct {
	URL string
}

type loadSessionTimelineEffect struct{}

func (sendPromptEffect) tuiEffect()          {}
func (copyTranscriptEffect) tuiEffect()      {}
func (openLinkEffect) tuiEffect()            {}
func (loadSessionTimelineEffect) tuiEffect() {}

func (m *model) runEffect(effect tuiEffect) tea.Cmd {
	switch value := effect.(type) {
	case sendPromptEffect:
		return m.startResponse(value.Text)
	case copyTranscriptEffect:
		if err := writeClipboard(value.Text); err != nil {
			return m.setStatus("copy failed")
		}

		return m.setStatus("transcript copied")
	case openLinkEffect:
		if err := openExternalLink(value.URL); err != nil {
			return m.setStatus("open link failed")
		}

		return nil
	case loadSessionTimelineEffect:
		return loadSessionTimelineCmd(m.chatCtx, m.timeline)
	default:
		return nil
	}
}

package tui

type tuiTheme struct {
	InputFrameBackground     string
	InputFrameBorder         string
	UserTranscriptBackground string
	UserTranscriptPrompt     string
	UserTranscriptText       string
	MutedText                string
	JumpToBottomForeground   string
	JumpToBottomBackground   string
	ToolRunningDot           string
	ToolCompletedDot         string
	ToolTitle                string
	ToolBranch               string
	ToolDetail               string
	ToolAddition             string
	ToolDeletion             string
	NoticeBackground         string
	NoticeBorder             string
	NoticeMuted              string
	NoticeForeground         string
	MarkdownCodeBackground   string
	MarkdownCodeForeground   string
	MarkdownLinkForeground   string
}

var defaultTUITheme = tuiTheme{
	InputFrameBackground:     "#050505",
	InputFrameBorder:         "8",
	UserTranscriptBackground: "#151515",
	UserTranscriptPrompt:     "245",
	UserTranscriptText:       "252",
	MutedText:                "8",
	JumpToBottomForeground:   "252",
	JumpToBottomBackground:   "238",
	ToolRunningDot:           "244",
	ToolCompletedDot:         "83",
	ToolTitle:                "246",
	ToolBranch:               "244",
	ToolDetail:               "246",
	ToolAddition:             "83",
	ToolDeletion:             "203",
	NoticeBackground:         "#292929",
	NoticeBorder:             "#242424",
	NoticeMuted:              "#A0A0A0",
	NoticeForeground:         "#FFFFFF",
	MarkdownCodeBackground:   "#1A1A1A",
	MarkdownCodeForeground:   "250",
	MarkdownLinkForeground:   "39",
}

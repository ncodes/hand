package render

type Theme struct {
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

var DefaultTheme = Theme{
	InputFrameBackground:     "232",
	InputFrameBorder:         "8",
	UserTranscriptBackground: "235",
	UserTranscriptPrompt:     "245",
	UserTranscriptText:       "252",
	MutedText:                "8",
	JumpToBottomForeground:   "252",
	JumpToBottomBackground:   "234",
	ToolRunningDot:           "244",
	ToolCompletedDot:         "83",
	ToolTitle:                "246",
	ToolBranch:               "244",
	ToolDetail:               "246",
	ToolAddition:             "83",
	ToolDeletion:             "203",
	NoticeBackground:         "235",
	NoticeBorder:             "238",
	NoticeMuted:              "246",
	NoticeForeground:         "15",
	MarkdownCodeBackground:   "235",
	MarkdownCodeForeground:   "250",
	MarkdownLinkForeground:   "39",
}

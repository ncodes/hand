package tui

type toolTranscriptRenderer struct{}

var defaultToolTranscriptRenderer = toolTranscriptRenderer{}

func (toolTranscriptRenderer) RenderGroup(
	group toolTranscriptGroup,
	ctx transcriptRenderContext,
) string {
	return renderToolTranscriptGroupContent(group, ctx)
}

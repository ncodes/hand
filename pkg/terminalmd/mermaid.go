package terminalmd

import (
	"strings"
)

func isMermaidLanguage(language string) bool {
	return strings.EqualFold(strings.TrimSpace(language), "mermaid")
}

func (r *Renderer) renderMermaidDiagram(source string, indent string) string {
	source = strings.TrimRight(source, "\n")
	if source == "" {
		return ""
	}

	label := r.style(r.opts.Theme.Muted).Render("Mermaid diagram")
	body := r.style(r.opts.Theme.CodeBlock).Render(source)
	lines := strings.Split(body, "\n")
	rendered := make([]string, 0, len(lines)+1)
	rendered = append(rendered, indent+label)
	for _, line := range lines {
		rendered = append(rendered, indent+line)
	}

	return strings.Join(rendered, "\n")
}

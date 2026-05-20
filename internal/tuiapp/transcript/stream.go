package transcript

import "strings"

type MarkdownStreamCollector struct {
	stable []string
	tail   string
}

func (c *MarkdownStreamCollector) Add(delta string) []string {
	if delta == "" {
		return nil
	}

	c.tail += delta
	index := strings.LastIndex(c.tail, "\n")
	if index < 0 {
		return nil
	}

	chunk := c.tail[:index+1]
	c.tail = c.tail[index+1:]
	c.stable = append(c.stable, chunk)

	return []string{chunk}
}

func (c MarkdownStreamCollector) Render() string {
	return strings.Join(c.stable, "") + c.tail
}

func (c *MarkdownStreamCollector) Finalize() string {
	text := c.Render()
	c.Reset()

	return text
}

func (c *MarkdownStreamCollector) Reset() {
	c.stable = nil
	c.tail = ""
}

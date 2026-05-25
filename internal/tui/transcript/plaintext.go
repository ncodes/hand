package transcript

// PlainTextCell exposes terminal cell text without styling.
type PlainTextCell interface {
	PlainText() string
	IsEmpty() bool
}

// PlainTexts renders cells into their plain text strings.
func PlainTexts[T PlainTextCell](cells []T) []string {
	if len(cells) == 0 {
		return nil
	}

	values := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell.IsEmpty() {
			continue
		}
		if text := cell.PlainText(); text != "" {
			values = append(values, text)
		}
	}

	return values
}

// CloneCells clones clone cells.
func CloneCells[T any](cells []T) []T {
	if len(cells) == 0 {
		return nil
	}

	cloned := make([]T, len(cells))
	copy(cloned, cells)

	return cloned
}

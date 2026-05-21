package transcript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testCell struct {
	text  string
	empty bool
}

func (cell testCell) PlainText() string {
	return cell.text
}

func (cell testCell) IsEmpty() bool {
	return cell.empty
}

func TestPlainTexts_ReturnsVisiblePlainText(t *testing.T) {
	values := PlainTexts([]testCell{
		{text: "one"},
		{text: "hidden", empty: true},
		{text: ""},
		{text: "two"},
	})

	require.Equal(t, []string{"one", "two"}, values)
}

func TestCloneCells_CopiesSlice(t *testing.T) {
	original := []string{"one", "two"}
	cloned := CloneCells(original)
	cloned[0] = "changed"

	require.Equal(t, []string{"one", "two"}, original)
	require.Equal(t, []string{"changed", "two"}, cloned)
}

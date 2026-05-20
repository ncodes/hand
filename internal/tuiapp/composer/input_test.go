package composer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInput_ClassifiesInput(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Input
	}{
		{
			name: "empty",
			text: " \n\t ",
			want: Input{Kind: InputEmpty},
		},
		{
			name: "prompt",
			text: " hello ",
			want: Input{Kind: InputPrompt, Text: "hello"},
		},
		{
			name: "command",
			text: " /use project-a ",
			want: Input{
				Kind: InputCommand,
				Text: "/use project-a",
				Name: "use",
				Args: "project-a",
			},
		},
		{
			name: "local command",
			text: " !git status ",
			want: Input{
				Kind: InputLocalCommand,
				Text: "!git status",
				Args: "git status",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ParseInput(tt.text))
		})
	}
}

func TestNormalizePaste_TrimsTrailingLineBreaks(t *testing.T) {
	require.Equal(t, "first\n\nsecond", NormalizePaste("first\n\nsecond\n\r\n"))
	require.Equal(t, "first\n\nsecond", NormalizePaste("first\n\nsecond"))
}

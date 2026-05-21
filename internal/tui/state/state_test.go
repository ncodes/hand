package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViewportSizeAction_NormalizesBounds(t *testing.T) {
	viewport := Viewport{}

	ViewportSizeAction{Width: 0, Height: -10}.Apply(&viewport)

	require.Equal(t, Viewport{Width: 1, Height: 1}, viewport)
}

func TestEffect_DescribesPortableSideEffect(t *testing.T) {
	effect := Effect{Kind: EffectSendPrompt, Text: "hello"}

	require.Equal(t, EffectSendPrompt, effect.Kind)
	require.Equal(t, "hello", effect.Text)
}

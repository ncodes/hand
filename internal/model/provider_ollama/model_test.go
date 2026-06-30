package provider_ollama

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeModelID(t *testing.T) {
	require.Equal(t, "qwen3:8b", NormalizeModelID(" ollama/qwen3:8b "))
	require.Equal(t, "openai/qwen3:8b", NormalizeModelID(" openai/qwen3:8b "))
	require.Equal(t, "qwen3:8b", NormalizeModelID(" qwen3:8b "))
	require.Empty(t, NormalizeModelID(" "))
}

func TestNormalizeModelIDForComparison(t *testing.T) {
	require.Equal(t, "lfm2.5-thinking:latest", NormalizeModelIDForComparison("lfm2.5-thinking"))
	require.Equal(t, "lfm2.5-thinking:latest", NormalizeModelIDForComparison("ollama/lfm2.5-thinking"))
	require.Equal(t, "lfm2.5-thinking:latest", NormalizeModelIDForComparison("lfm2.5-thinking:latest"))
	require.Equal(t, "llama3.2:cloud", NormalizeModelIDForComparison("llama3.2:cloud"))
	require.Empty(t, NormalizeModelIDForComparison(" "))
}

func TestModelIDMatches(t *testing.T) {
	require.True(t, ModelIDMatches("lfm2.5-thinking:latest", "lfm2.5-thinking"))
	require.True(t, ModelIDMatches("ollama/lfm2.5-thinking", "lfm2.5-thinking:latest"))
	require.True(t, ModelIDMatches("QWEN3:8B", "qwen3:8b"))
	require.False(t, ModelIDMatches("qwen3:8b", "qwen3:14b"))
	require.False(t, ModelIDMatches("", "qwen3:8b"))
}

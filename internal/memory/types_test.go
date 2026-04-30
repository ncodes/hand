package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryItem_GuardrailSource(t *testing.T) {
	require.Equal(t, "memory:mem_123", MemoryItem{ID: " mem_123 "}.GuardrailSource())
	require.Equal(t, "memory", MemoryItem{}.GuardrailSource())
}

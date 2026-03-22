package identity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/context"
)

func TestGetBaseIdentity_ReturnsInstruction(t *testing.T) {
	instruction := GetBaseIdentity("Wandxie")

	require.IsType(t, context.Instruction{}, instruction)
	require.NotEmpty(t, instruction.Value)
}

func TestGetBaseIdentity_IncludesAgentNameAndCoreIdentity(t *testing.T) {
	instruction := GetBaseIdentity("Wandxie")

	require.True(t, strings.Contains(instruction.Value, "You are Wandxie,"))
	require.True(t, strings.Contains(instruction.Value, "developed by Wandxy"))
	require.True(t, strings.Contains(instruction.Value, "helpful, knowledgeable, and straightforward"))
}

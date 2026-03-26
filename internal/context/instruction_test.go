package context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInstructionsBuildsTrimmedList(t *testing.T) {
	instructions := NewInstructions(" first ", "   ", "second")
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, instructions)
}

func TestNewInstructionsReturnsEmptyWhenNoValuesProvided(t *testing.T) {
	require.Empty(t, NewInstructions())
}

func TestInstructions_StringJoinsValuesWithNewLines(t *testing.T) {
	instructions := Instructions{
		{Value: "first"},
		{Value: "second"},
		{Value: "third"},
	}

	require.Equal(t, "first\nsecond\nthird", instructions.String())
}

func TestInstructions_MarshalJSONEncodesJoinedString(t *testing.T) {
	instructions := Instructions{
		{Value: "first"},
		{Value: "second"},
	}

	data, err := instructions.MarshalJSON()

	require.NoError(t, err)
	require.JSONEq(t, `"first\nsecond"`, string(data))
}

func TestInstructions_UnmarshalJSONDecodesStringArray(t *testing.T) {
	var instructions Instructions

	err := instructions.UnmarshalJSON([]byte(`["first","second"]`))

	require.NoError(t, err)
	require.Equal(t, Instructions{
		{Value: "first"},
		{Value: "second"},
	}, instructions)
}

func TestInstructions_UnmarshalJSONRejectsInvalidShape(t *testing.T) {
	var instructions Instructions

	err := instructions.UnmarshalJSON([]byte(`"first"`))

	require.Error(t, err)
}

func TestInstructions_JSONRoundTripUsesMarshalAndUnmarshalImplementations(t *testing.T) {
	original := Instructions{
		{Value: "first"},
		{Value: "second"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Instructions
	err = json.Unmarshal([]byte(`["first","second"]`), &decoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
	require.JSONEq(t, `"first\nsecond"`, string(data))
}

func TestInstructions_ChainAppendsTrimmedInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	chained := original.Chain(Instruction{Value: " second "})

	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, chained)
}

func TestInstructions_ChainSkipsEmptyInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	chained := original.Chain(Instruction{Value: "   "})

	require.Equal(t, original, chained)
}

func TestInstructions_ChainValueAppendsInstruction(t *testing.T) {
	chained := Instructions{}.ChainValue(" first ")
	require.Equal(t, Instructions{{Value: "first"}}, chained)
}

func TestInstructions_ChainValueAppendsMultipleInstructions(t *testing.T) {
	chained := Instructions{}.ChainValue(" first ", "   ", "second")
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, chained)
}

func TestInstructions_ChainAppendsMultipleInstructions(t *testing.T) {
	original := Instructions{{Value: "first"}}
	chained := original.Chain(
		Instruction{Value: " second "},
		Instruction{Value: "   "},
		Instruction{Value: "third"},
	)

	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{
		{Value: "first"},
		{Value: "second"},
		{Value: "third"},
	}, chained)
}

func TestInstructions_FirstReturnsZeroValueWhenEmpty(t *testing.T) {
	require.Equal(t, Instruction{}, Instructions{}.First())
}

func TestInstructions_FirstReturnsFirstInstruction(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Value: "second"}}
	require.Equal(t, Instruction{Value: "first"}, instructions.First())
}

func TestInstructions_GetByNameReturnsNamedInstruction(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}

	instruction, ok := instructions.GetByName("request.instruct")

	require.True(t, ok)
	require.Equal(t, Instruction{Name: "request.instruct", Value: "be terse"}, instruction)
}

func TestInstructions_WithoutNameRemovesMatchingInstruction(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}

	filtered := instructions.WithoutName("request.instruct")

	require.Equal(t, Instructions{{Value: "first"}}, filtered)
}

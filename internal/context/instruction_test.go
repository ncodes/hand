package context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstructionsString_JoinsValuesWithNewLines(t *testing.T) {
	instructions := Instructions{
		{Value: "first"},
		{Value: "second"},
		{Value: "third"},
	}

	require.Equal(t, "first\nsecond\nthird", instructions.String())
}

func TestInstructionsMarshalJSON_EncodesJoinedString(t *testing.T) {
	instructions := Instructions{
		{Value: "first"},
		{Value: "second"},
	}

	data, err := instructions.MarshalJSON()

	require.NoError(t, err)
	require.JSONEq(t, `"first\nsecond"`, string(data))
}

func TestInstructionsUnmarshalJSON_DecodesStringArray(t *testing.T) {
	var instructions Instructions

	err := instructions.UnmarshalJSON([]byte(`["first","second"]`))

	require.NoError(t, err)
	require.Equal(t, Instructions{
		{Value: "first"},
		{Value: "second"},
	}, instructions)
}

func TestInstructionsUnmarshalJSON_RejectsInvalidShape(t *testing.T) {
	var instructions Instructions

	err := instructions.UnmarshalJSON([]byte(`"first"`))

	require.Error(t, err)
}

func TestInstructionsJSON_RoundTripUsesMarshalAndUnmarshalImplementations(t *testing.T) {
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

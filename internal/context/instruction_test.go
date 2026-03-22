package context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

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

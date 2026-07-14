package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefinitions_NameHelpers(t *testing.T) {
	definitions := Definitions{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: " "},
	}

	require.True(t, definitions.Has("alpha"))
	require.True(t, definitions.Has(" beta "))
	require.False(t, definitions.Has("missing"))
	require.Equal(t, []string{"alpha", "beta"}, definitions.Names())

	definition, ok := definitions.Get(" beta ")
	require.True(t, ok)
	require.Equal(t, "beta", definition.Name)

	_, ok = definitions.Get("")
	require.False(t, ok)
	require.Nil(t, Definitions(nil).Names())
	require.Nil(t, Definitions{{Name: ""}, {Name: " "}}.Names())
}

func TestError_StringReturnsJSON(t *testing.T) {
	require.JSONEq(t,
		`{"code":"invalid_input","message":"invalid input"}`,
		Error{Code: "invalid_input", Message: "invalid input"}.String(),
	)
	require.JSONEq(t,
		`{"code":"rate_limited","message":"retry \"later\"","retryable":true}`,
		Error{Code: "rate_limited", Message: `retry "later"`, Retryable: true}.String(),
	)
}

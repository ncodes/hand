package time

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/tools"
)

func TestDefinition_DeclaresObjectInputSchema(t *testing.T) {
	definition := Definition()

	require.Equal(t, "time", definition.Name)
	require.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
		"required":             []string{},
	}, definition.InputSchema)
}

func TestDefinition_HandlerReturnsRFC3339Time(t *testing.T) {
	originalNow := now
	t.Cleanup(func() {
		now = originalNow
	})
	now = func() time.Time {
		return time.Date(2026, time.March, 28, 1, 2, 3, 0, time.FixedZone("WAT", 3600))
	}

	definition := Definition()
	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Name: "time"})

	require.NoError(t, err)
	require.Equal(t, "2026-03-28T00:02:03Z", result.Output)
	require.Empty(t, result.Error)
	require.Equal(t, []string{"core"}, definition.Groups)
}

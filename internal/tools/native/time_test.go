package native

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/tools"
)

func TestTimeDefinition_DeclaresObjectInputSchema(t *testing.T) {
	definition := TimeDefinition()

	require.Equal(t, "time", definition.Name)
	require.Equal(t, map[string]any{
		"type": "object",
	}, definition.InputSchema)
}

func TestTimeDefinition_HandlerReturnsRFC3339Time(t *testing.T) {
	originalNow := now
	t.Cleanup(func() {
		now = originalNow
	})
	now = func() time.Time {
		return time.Date(2026, time.March, 28, 1, 2, 3, 0, time.FixedZone("WAT", 3600))
	}

	definition := TimeDefinition()
	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Name: "time"})

	require.NoError(t, err)
	require.Equal(t, "2026-03-28T00:02:03Z", result.Output)
	require.Empty(t, result.Error)
	require.Equal(t, []string{"core"}, definition.Groups)
}

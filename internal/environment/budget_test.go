package environment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIterationBudgetConsumesDownToZero(t *testing.T) {
	budget := NewIterationBudget(2)

	require.Equal(t, 2, budget.Remaining())
	require.True(t, budget.Consume())
	require.Equal(t, 1, budget.Remaining())
	require.True(t, budget.Consume())
	require.Equal(t, 0, budget.Remaining())
	require.False(t, budget.Consume())
}

func TestIterationBudgetConsumeRejectsNilReceiver(t *testing.T) {
	var budget *IterationBudget
	require.False(t, budget.Consume())
}

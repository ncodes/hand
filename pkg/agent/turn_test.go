package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunLoop_ReturnsReplyWhenStepCompletes(t *testing.T) {
	remaining := 2

	reply, err := RunLoop(context.Background(), LoopOptions{
		Consume: func() bool {
			remaining--
			return remaining >= 0
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{Done: true, Reply: "done"}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "done", reply)
}

func TestRunLoop_CallsFallbackWhenBudgetExhausts(t *testing.T) {
	reply, err := RunLoop(context.Background(), LoopOptions{
		Consume: func() bool { return false },
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, nil
		},
		OnExhausted: func(context.Context) (string, error) {
			return "fallback", nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "fallback", reply)
}

func TestRunLoop_PropagatesStepErrors(t *testing.T) {
	expected := errors.New("step failed")

	reply, err := RunLoop(context.Background(), LoopOptions{
		Consume: func() bool { return true },
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, expected
		},
	})

	require.Empty(t, reply)
	require.ErrorIs(t, err, expected)
}

func TestRunLoop_RequiresCallbacks(t *testing.T) {
	_, err := RunLoop(context.Background(), LoopOptions{})
	require.EqualError(t, err, "loop budget consumer is required")

	_, err = RunLoop(context.Background(), LoopOptions{
		Consume: func() bool { return true },
	})
	require.EqualError(t, err, "loop step is required")

	_, err = RunLoop(context.Background(), LoopOptions{
		Consume: func() bool { return false },
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, nil
		},
	})
	require.EqualError(t, err, "loop exhausted")
}

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunTurnLifecycle_RunsHooksInTurnOrder(t *testing.T) {
	var calls []string
	remaining := 2

	reply, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{Instruct: " be kind "}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			calls = append(calls, "load")
			return nil
		},
		SetRequestInstruction: func(value string) {
			calls = append(calls, "instruction:"+value)
		},
		Prepare: func(context.Context) error {
			calls = append(calls, "prepare")
			return nil
		},
		CheckInput: func(context.Context, string) (InputCheck, error) {
			calls = append(calls, "check")
			return InputCheck{}, nil
		},
		AcceptUserMessage: func(context.Context, string) error {
			calls = append(calls, "accept")
			return nil
		},
		LoadMemory: func(context.Context, string) error {
			calls = append(calls, "memory")
			return nil
		},
		ConsumeIteration: func() bool {
			if remaining <= 0 {
				return false
			}
			remaining--
			return true
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			calls = append(calls, "step")
			if remaining == 0 {
				return LoopDecision{Done: true, Reply: "done"}, nil
			}
			return LoopDecision{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "done", reply)
	require.Equal(t, []string{
		"load",
		"instruction:be kind",
		"prepare",
		"check",
		"accept",
		"memory",
		"step",
		"step",
	}, calls)
}

func TestRunTurnLifecycle_ReturnsBlockedInputReply(t *testing.T) {
	reply, err := RunTurnLifecycle(context.Background(), "blocked", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
		CheckInput: func(context.Context, string) (InputCheck, error) {
			return InputCheck{Blocked: true, Reply: "nope"}, nil
		},
		ConsumeIteration: func() bool {
			return true
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			t.Fatal("step should not run for blocked input")
			return LoopDecision{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "nope", reply)
}

func TestRunTurnLifecycle_UsesExhaustedFallback(t *testing.T) {
	reply, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
		ConsumeIteration: func() bool {
			return false
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			t.Fatal("step should not run when budget is exhausted")
			return LoopDecision{}, nil
		},
		OnExhausted: func(context.Context) (string, error) {
			return "fallback", nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "fallback", reply)
}

func TestRunTurnLifecycle_RequiresLoaderAndStep(t *testing.T) {
	_, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{})
	require.EqualError(t, err, "turn loader is required")

	_, err = RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
	})
	require.EqualError(t, err, "turn step is required")
}

func TestRunTurnLifecycle_ReturnsHookErrors(t *testing.T) {
	expected := errors.New("prepare failed")

	_, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
		Prepare: func(context.Context) error {
			return expected
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, nil
		},
	})

	require.ErrorIs(t, err, expected)
}

func TestRunTurnLifecycle_ReturnsLoadError(t *testing.T) {
	expected := errors.New("load failed")

	_, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return expected
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, nil
		},
	})

	require.ErrorIs(t, err, expected)
}

func TestRunTurnLifecycle_ReturnsOpenError(t *testing.T) {
	expected := errors.New("open failed")

	_, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
		Open: func(context.Context, RespondOptions) (TurnCloser, error) {
			return nil, expected
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{}, nil
		},
	})

	require.ErrorIs(t, err, expected)
}

func TestRunTurnLifecycle_ClosesOpenedTurn(t *testing.T) {
	closer := &testTurnCloser{}

	reply, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, TurnLifecycle{
		Load: func(context.Context, RespondOptions) error {
			return nil
		},
		Open: func(context.Context, RespondOptions) (TurnCloser, error) {
			return closer, nil
		},
		ConsumeIteration: func() bool {
			return true
		},
		RunStep: func(context.Context) (LoopDecision, error) {
			return LoopDecision{Done: true, Reply: "done"}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "done", reply)
	require.True(t, closer.closed)
}

func TestRunTurnLifecycle_ReturnsOptionalHookErrors(t *testing.T) {
	expected := errors.New("hook failed")

	tests := []struct {
		name      string
		lifecycle TurnLifecycle
	}{
		{
			name: "check input",
			lifecycle: TurnLifecycle{
				CheckInput: func(context.Context, string) (InputCheck, error) {
					return InputCheck{}, expected
				},
			},
		},
		{
			name: "accept user message",
			lifecycle: TurnLifecycle{
				AcceptUserMessage: func(context.Context, string) error {
					return expected
				},
			},
		},
		{
			name: "load memory",
			lifecycle: TurnLifecycle{
				LoadMemory: func(context.Context, string) error {
					return expected
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.lifecycle.Load = func(context.Context, RespondOptions) error {
				return nil
			}
			test.lifecycle.RunStep = func(context.Context) (LoopDecision, error) {
				return LoopDecision{}, nil
			}

			_, err := RunTurnLifecycle(context.Background(), "hello", RespondOptions{}, test.lifecycle)
			require.ErrorIs(t, err, expected)
		})
	}
}

type testTurnCloser struct {
	closed bool
}

func (c *testTurnCloser) Close() {
	c.closed = true
}

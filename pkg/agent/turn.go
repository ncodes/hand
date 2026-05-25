package agent

import (
	"context"
	"errors"
)

type LoopDecision struct {
	Reply string
	Done  bool
}

type LoopOptions struct {
	Consume     func() bool
	RunStep     func(context.Context) (LoopDecision, error)
	OnExhausted func(context.Context) (string, error)
}

func RunLoop(ctx context.Context, opts LoopOptions) (string, error) {
	if opts.Consume == nil {
		return "", errors.New("loop budget consumer is required")
	}
	if opts.RunStep == nil {
		return "", errors.New("loop step is required")
	}

	for opts.Consume() {
		decision, err := opts.RunStep(ctx)
		if err != nil {
			return "", err
		}
		if decision.Done {
			return decision.Reply, nil
		}
	}

	if opts.OnExhausted == nil {
		return "", errors.New("loop exhausted")
	}

	return opts.OnExhausted(ctx)
}

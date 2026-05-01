package memory

import "context"

type Guardrails interface {
	ValidateSearch(context.Context, SearchQuery) error
	ValidateWrite(context.Context, MemoryItem) error
	ValidateDelete(context.Context, DeleteRequest) error
	SafetyScan(context.Context, MemoryItem) error
	Redact(context.Context, MemoryItem) (MemoryItem, error)
}

func validateSearch(ctx context.Context, guardrails Guardrails, query SearchQuery) error {
	if guardrails == nil {
		return nil
	}
	return guardrails.ValidateSearch(ctx, query)
}

func validateWrite(ctx context.Context, guardrails Guardrails, item MemoryItem) error {
	if guardrails == nil {
		return nil
	}
	if err := guardrails.ValidateWrite(ctx, item); err != nil {
		return err
	}
	return guardrails.SafetyScan(ctx, item)
}

func validateDelete(ctx context.Context, guardrails Guardrails, req DeleteRequest) error {
	if guardrails == nil {
		return nil
	}
	return guardrails.ValidateDelete(ctx, req)
}

func redactItem(ctx context.Context, guardrails Guardrails, item MemoryItem) (MemoryItem, error) {
	if guardrails == nil {
		return item, nil
	}
	return guardrails.Redact(ctx, item)
}

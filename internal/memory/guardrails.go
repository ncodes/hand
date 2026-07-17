package memory

import "context"

// Guardrails is the safety boundary around every provider operation.
//
// Validation methods reject malformed requests before storage work starts.
// SafetyScan protects writes before they become durable. Redact protects reads
// before memory content is injected into prompts or returned to callers.
type Guardrails interface {
	ValidateSearch(context.Context, SearchQuery) error
	ValidateWrite(context.Context, MemoryItem) error
	ValidateDelete(context.Context, DeleteRequest) error
	SafetyScan(context.Context, MemoryItem) error
	Redact(context.Context, MemoryItem) (MemoryItem, error)
}

// validateSearch is intentionally a no-op when guardrails are not configured so
// tests and local stores can use the provider without installing a full safety
// stack.
func validateSearch(ctx context.Context, guardrails Guardrails, query SearchQuery) error {
	if guardrails == nil {
		return nil
	}

	return guardrails.ValidateSearch(ctx, query)
}

// validateWrite runs structural validation and content safety scanning together
// because both are required before a memory can become durable.
func validateWrite(ctx context.Context, guardrails Guardrails, item MemoryItem) error {
	if guardrails == nil {
		return nil
	}
	if err := guardrails.ValidateWrite(ctx, item); err != nil {
		return err
	}

	return guardrails.SafetyScan(ctx, item)
}

// validateDelete keeps delete validation symmetrical with search/write, even
// though deletes usually only need ID-level checks.
func validateDelete(ctx context.Context, guardrails Guardrails, req DeleteRequest) error {
	if guardrails == nil {
		return nil
	}

	return guardrails.ValidateDelete(ctx, req)
}

// redactItem is applied on reads. Storage should keep the canonical memory; the
// provider redacts only the copy crossing back toward prompts and callers.
func redactItem(ctx context.Context, guardrails Guardrails, item MemoryItem) (MemoryItem, error) {
	if guardrails == nil {
		return item, nil
	}

	return guardrails.Redact(ctx, item)
}

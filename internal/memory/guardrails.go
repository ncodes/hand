package memory

import "context"

type Guardrails interface {
	ValidateSearch(context.Context, SearchQuery) error
	ValidateWrite(context.Context, MemoryItem) error
	ValidateDelete(context.Context, DeleteRequest) error
	SafetyScan(context.Context, MemoryItem) error
	Redact(context.Context, MemoryItem) (MemoryItem, error)
}

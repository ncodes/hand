package core

// Store defines the aggregate durable state store.
type Store interface {
	Session() SessionStore
	Memory() (MemoryStore, bool)
	Trace() (TraceStore, bool)
	SupportsVectorSearch() bool
}

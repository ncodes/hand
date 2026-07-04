package core

// Store defines the aggregate durable state store.
type Store interface {
	Session() SessionStore
	Automation() (AutomationStore, bool)
	Memory() (MemoryStore, bool)
	Trace() (TraceStore, bool)
	SupportsVectorSearch() bool
}

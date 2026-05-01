package memory

import (
	"context"

	statecore "github.com/wandxy/hand/internal/state/core"
)

type Kind = statecore.MemoryKind

const (
	KindPinned     = statecore.MemoryKindPinned
	KindSemantic   = statecore.MemoryKindSemantic
	KindEpisodic   = statecore.MemoryKindEpisodic
	KindProcedural = statecore.MemoryKindProcedural
)

type Status = statecore.MemoryStatus

const (
	StatusCandidate  = statecore.MemoryStatusCandidate
	StatusActive     = statecore.MemoryStatusActive
	StatusSuperseded = statecore.MemoryStatusSuperseded
	StatusDeleted    = statecore.MemoryStatusDeleted
)

type SourceLink = statecore.MemorySourceLink
type MemoryItem = statecore.MemoryItem
type SearchQuery = statecore.MemorySearchQuery
type SearchHit = statecore.MemorySearchHit
type SearchResult = statecore.MemorySearchResult
type DeleteRequest = statecore.MemoryDeleteRequest

type Capabilities struct {
	SupportsPinned           bool
	SupportsSearch           bool
	SupportsWrite            bool
	SupportsDelete           bool
	SupportsEpisodeRecording bool
	SupportsReflection       bool
	SupportsBM25             bool
	SupportsVectors          bool
	SupportsReranking        bool
	SupportsAudit            bool
	SupportsObservability    bool
}

type EpisodeRecord struct {
	Item MemoryItem
}

type ReflectionRequest struct {
	Limit int
}

type ReflectionResult struct {
	Items []MemoryItem
}

type Provider interface {
	Name() string
	Capabilities(context.Context) (Capabilities, error)
	ConfigureObservability(Observability) error
	Close() error
}

type PinnedProvider interface {
	LoadPinned(context.Context, SearchQuery) ([]MemoryItem, error)
}

type SearchProvider interface {
	Search(context.Context, SearchQuery) (SearchResult, error)
}

type WriteProvider interface {
	Upsert(context.Context, MemoryItem) (MemoryItem, error)
	Delete(context.Context, DeleteRequest) error
}

type EpisodeProvider interface {
	RecordEpisode(context.Context, EpisodeRecord) (MemoryItem, error)
}

type ReflectionProvider interface {
	Reflect(context.Context, ReflectionRequest) (ReflectionResult, error)
}

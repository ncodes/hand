package memory

import (
	"context"
	"time"
)

type Kind string

const (
	KindPinned     Kind = "pinned"
	KindSemantic   Kind = "semantic"
	KindEpisodic   Kind = "episodic"
	KindProcedural Kind = "procedural"
)

type Status string

const (
	StatusCandidate  Status = "candidate"
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusDeleted    Status = "deleted"
)

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

type SourceLink struct {
	SessionID     string
	MessageIDs    []uint
	Offsets       []int
	SummaryID     string
	CreatedBy     string
	CreatedReason string
}

type MemoryItem struct {
	ID          string
	Kind        Kind
	Status      Status
	Title       string
	Text        string
	Tags        []string
	Metadata    map[string]string
	SourceLinks []SourceLink
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SearchQuery struct {
	Text     string
	Kinds    []Kind
	Statuses []Status
	Tags     []string
	Limit    int
	MaxChars int
}

type SearchHit struct {
	Item  MemoryItem
	Score float64
}

type SearchResult struct {
	Hits []SearchHit
}

type DeleteRequest struct {
	ID     string
	Reason string
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

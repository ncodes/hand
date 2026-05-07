package memory

import (
	"context"
	"time"

	"github.com/wandxy/hand/internal/memory/episodic"
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
type MemoryPatch = statecore.MemoryPatch
type SearchQuery = statecore.MemorySearchQuery
type SearchHit = statecore.MemorySearchHit
type SearchResult = statecore.MemorySearchResult
type DeleteRequest = statecore.MemoryDeleteRequest

type Capabilities struct {
	SupportsPinned                      bool
	SupportsSearch                      bool
	SupportsWrite                       bool
	SupportsDelete                      bool
	SupportsEpisodeRecording            bool
	SupportsSemanticProceduralRecording bool
	SupportsReflection                  bool
	SupportsBM25                        bool
	SupportsVectors                     bool
	SupportsReranking                   bool
	SupportsAudit                       bool
	SupportsObservability               bool
}

type EpisodeRecord = episodic.EpisodeRecord
type ExtractionRequest = episodic.Request
type ExtractionResult = episodic.Result
type ExtractionWindowResult = episodic.WindowResult
type EpisodicBackgroundOptions = episodic.BackgroundOptions
type TraceRecorder = episodic.TraceRecorder

type SemanticRecord struct {
	Item MemoryItem
}

type ProceduralRecord struct {
	Item MemoryItem
}

type ReflectionRequest struct {
	SessionID    string
	Limit        int
	RelatedLimit int
}

type ReflectionBackgroundOptions struct {
	Enabled      bool
	Interval     time.Duration
	Limit        int
	RelatedLimit int
}

type ReflectionResult struct {
	SessionID    string
	SourceCount  int
	RelatedCount int
	WriteCount   int
	Items        []MemoryItem
}

type ReflectionGenerationRequest struct {
	SessionID string
	Sources   []MemoryItem
	Related   []MemoryItem
	Limit     int
}

type ReflectionGenerationResult struct {
	Items []MemoryItem
}

type ReflectionGenerator interface {
	GenerateReflectionCandidates(context.Context, ReflectionGenerationRequest) (ReflectionGenerationResult, error)
}

type PromotionRequest struct {
	ID     string
	Reason string
	Strict bool
}

type PromotionBackgroundOptions struct {
	Enabled  bool
	Interval time.Duration
	Limit    int
	Reason   string
}

type LifecycleResult struct {
	Item     MemoryItem
	Related  []MemoryItem
	Decision PromotionDecision
}

type PromotionPolicyRequest struct {
	Candidate          MemoryItem
	Related            []MemoryItem
	Reason             string
	Strict             bool
	AdmissionResult    string
	GuardrailResult    string
	ReflectionEvidence bool
	ConflictState      string
}

type PromotionDecision struct {
	Approved      bool
	Policy        string
	Reason        string
	Confidence    float64
	ConflictState string
}

type PromotionPolicy interface {
	EvaluatePromotion(context.Context, PromotionPolicyRequest) (PromotionDecision, error)
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

type SemanticProceduralProvider interface {
	RecordSemanticMemory(context.Context, SemanticRecord) (MemoryItem, error)
	RecordProceduralMemory(context.Context, ProceduralRecord) (MemoryItem, error)
}

type ExtractionProvider interface {
	ExtractEpisodes(context.Context, ExtractionRequest) (ExtractionResult, error)
}

type BackgroundProvider interface {
	StartBackground(context.Context) error
}

type ReflectionProvider interface {
	Reflect(context.Context, ReflectionRequest) (ReflectionResult, error)
}

type LifecycleProvider interface {
	PromoteCandidate(context.Context, PromotionRequest) (LifecycleResult, error)
}

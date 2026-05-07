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

// Capabilities describes the behavioral surface a memory provider exposes.
// Callers use it as feature negotiation instead of assuming every backend can
// search, reflect, promote, rerank, and emit traces.
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

// ReflectionRequest consolidates unreflected episodic memories for one session
// into higher-order candidates. Empty SessionID means "use the current session".
type ReflectionRequest struct {
	SessionID    string
	Limit        int
	RelatedLimit int
}

// ReflectionBackgroundOptions configures the periodic reflection loop. The loop
// calls the same Reflect method as manual invocations, so validation, dedupe,
// provenance, and writes stay centralized.
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

// ReflectionGenerationRequest is the LLM-facing prompt payload. The provider
// gives the generator source memories and nearby related memories, but keeps
// responsibility for candidate validation and storage.
type ReflectionGenerationRequest struct {
	SessionID string
	Sources   []MemoryItem
	Related   []MemoryItem
	Limit     int
}

type ReflectionGenerationResult struct {
	Items []MemoryItem
}

// ReflectionGenerator proposes candidates only. It should not perform storage
// writes or lifecycle decisions; the provider owns those responsibilities.
type ReflectionGenerator interface {
	GenerateReflectionCandidates(context.Context, ReflectionGenerationRequest) (ReflectionGenerationResult, error)
}

// PromotionRequest evaluates one candidate for activation. Strict is reserved
// for flows that intentionally allow sensitive transitions, such as promoting a
// pinned candidate that background promotion must reject.
type PromotionRequest struct {
	ID     string
	Reason string
	Strict bool
}

// PromotionBackgroundOptions controls autonomous promotion of unevaluated
// candidates. A candidate with PromotionEvaluatedAt set is intentionally skipped
// so rejected candidates do not churn forever.
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

// PromotionPolicyRequest gives a policy all available evidence without giving
// it direct access to storage. Provider-owned hard gates are passed in as
// strings so a custom policy can explain them, but cannot bypass them.
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

// PromotionDecision is persisted as audit metadata whenever a candidate is
// evaluated. The simple shape keeps CLI output, trace inspection, and database
// debugging readable.
type PromotionDecision struct {
	Approved      bool
	Policy        string
	Reason        string
	Confidence    float64
	ConflictState string
}

// PromotionPolicy makes the final promote/reject recommendation. Policies
// should be pure decision functions; writes and metadata stay inside provider
// lifecycle code.
type PromotionPolicy interface {
	EvaluatePromotion(context.Context, PromotionPolicyRequest) (PromotionDecision, error)
}

// Provider is the root interface common to all memory implementations. Feature
// interfaces below are split so callers can depend on only the behavior they
// need.
type Provider interface {
	Name() string
	Capabilities(context.Context) (Capabilities, error)
	ConfigureObservability(Observability) error
	Close() error
}

// PinnedProvider loads memory intended for direct prompt-context injection.
type PinnedProvider interface {
	LoadPinned(context.Context, SearchQuery) ([]MemoryItem, error)
}

// SearchProvider retrieves existing memories through the provider guardrail and
// redaction boundary.
type SearchProvider interface {
	Search(context.Context, SearchQuery) (SearchResult, error)
}

// WriteProvider records or deletes memory through provider validation.
type WriteProvider interface {
	Upsert(context.Context, MemoryItem) (MemoryItem, error)
	Delete(context.Context, DeleteRequest) error
}

// EpisodeProvider records one already-curated episodic candidate.
type EpisodeProvider interface {
	RecordEpisode(context.Context, EpisodeRecord) (MemoryItem, error)
}

// SemanticProceduralProvider records explicit non-episodic candidates.
type SemanticProceduralProvider interface {
	RecordSemanticMemory(context.Context, SemanticRecord) (MemoryItem, error)
	RecordProceduralMemory(context.Context, ProceduralRecord) (MemoryItem, error)
}

// ExtractionProvider runs model-backed episodic extraction over message windows.
type ExtractionProvider interface {
	ExtractEpisodes(context.Context, ExtractionRequest) (ExtractionResult, error)
}

// BackgroundProvider starts provider-owned background loops.
type BackgroundProvider interface {
	StartBackground(context.Context) error
}

// ReflectionProvider consolidates episodic evidence into derived candidates.
type ReflectionProvider interface {
	Reflect(context.Context, ReflectionRequest) (ReflectionResult, error)
}

// LifecycleProvider governs candidate activation.
type LifecycleProvider interface {
	PromoteCandidate(context.Context, PromotionRequest) (LifecycleResult, error)
}

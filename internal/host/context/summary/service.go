package summary

import (
	"context"
	"errors"
	"time"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/host/context/compaction"
	storage "github.com/wandxy/hand/internal/state/core"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	models "github.com/wandxy/hand/pkg/agent/model"
)

// SummaryStore describes the persisted session operations needed by summary
// loading, refresh, and compaction paths.
type SummaryStore interface {
	Get(context.Context, string) (storage.Session, bool, error)
	Save(context.Context, storage.Session) error
	GetSummary(context.Context, string) (storage.SessionSummary, bool, error)
	SaveSummary(context.Context, storage.SessionSummary) error
	GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	CountMessages(context.Context, string, storage.MessageQueryOptions) (int, error)
}

// Service owns summary retrieval and refresh decisions for a turn.
type Service struct {
	modelClient     models.Client
	summaryClient   models.Client
	store           SummaryStore
	evaluator       *compaction.Evaluator
	compactionOn    bool
	model           string
	provider        string
	summaryModel    string
	summaryProvider string
	apiMode         string
	debugRequests   bool
	recentTail      int
	now             func() time.Time
}

// RefreshInput contains the current model request and token state used to
// decide whether summary compaction should run.
type RefreshInput struct {
	LastPromptTokens int
	Request          models.Request
	SessionID        string
	TraceSession     traceRecorder
}

// traceRecorder is the small trace surface summary refresh needs.
type traceRecorder interface {
	Record(string, any)
}

// NewService builds the summary service used for summary loading, automatic
// compaction, persisted compaction, and recall summarization.
func NewService(cfg *config.Config, modelClient, summaryClient models.Client, summaryStore SummaryStore) *Service {
	if summaryClient == nil {
		summaryClient = modelClient
	}

	service := &Service{
		modelClient:   modelClient,
		summaryClient: summaryClient,
		store:         summaryStore,
		evaluator:     getSummaryCompactionEvaluator(cfg),
		compactionOn:  isSummaryCompactionEnabled(cfg),
		recentTail:    getSummaryRecentSessionTail(cfg),
		now:           func() time.Time { return time.Now().UTC() },
	}

	if cfg != nil {
		service.model = cfg.Models.Main.Name
		service.provider = cfg.Models.Main.Provider
		service.summaryModel = cfg.SummaryModelEffective()
		service.summaryProvider = cfg.SummaryProviderEffective()
		service.apiMode = cfg.SummaryModelAPIModeEffective()
		service.debugRequests = cfg.Debug.Requests
	}

	logEvent := summaryLog.Debug().
		Str("plan", "load_existing_summary_refresh_when_context_budget_requires_compaction").
		Str("model", service.model).
		Str("summary_model", service.summaryModel).
		Bool("compaction_enabled", service.compactionOn)

	if cfg != nil && cfg.SummaryProviderEffective() != cfg.Models.Main.Provider {
		logEvent = logEvent.Str("summary_provider", cfg.SummaryProviderEffective())
	}

	if cfg != nil && cfg.SummaryModelAPIModeEffective() != cfg.Models.Main.APIMode {
		logEvent = logEvent.Str("summary_api_mode", cfg.SummaryModelAPIModeEffective())
	}

	logEvent.Msg("summary service initialized")

	return service
}

// Load returns the current summary view for a session from persisted summary
// state without reading the live session transcript.
func (s *Service) Load(ctx context.Context, sessionID string) (*State, error) {
	if s == nil {
		return nil, errors.New("summary service is required")
	}

	if s.store == nil {
		return nil, errors.New("summary store is required")
	}

	summary, _, err := s.store.GetSummary(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	state := &State{Current: SummaryFromStorage(summary)}

	if state.Current != nil {
		summaryLog.Debug().
			Str("session_id", sessionID).
			Int("source_end_offset", state.Current.SourceEndOffset).
			Msg("summary loaded with existing summary")
	} else {
		summaryLog.Debug().
			Str("session_id", sessionID).
			Msg("summary loaded")
	}

	return state, nil
}

package summary

import (
	"context"
	"errors"
	"time"

	"github.com/wandxy/hand/internal/agent/context/compaction"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/storage/session"
)

type SummaryStore interface {
	Get(context.Context, string) (storage.Session, bool, error)
	Save(context.Context, storage.Session) error
	GetSummary(context.Context, string) (storage.SessionSummary, bool, error)
	SaveSummary(context.Context, storage.SessionSummary) error
	GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	CountMessages(context.Context, string, storage.MessageQueryOptions) (int, error)
}

type Service struct {
	modelClient   models.Client
	summaryClient models.Client
	store         SummaryStore
	evaluator     *compaction.Evaluator
	compactionOn  bool
	model         string
	summaryModel  string
	apiMode       string
	debugRequests bool
	now           func() time.Time
}

type RefreshInput struct {
	LastPromptTokens int
	Request          models.Request
	SessionID        string
	TraceSession     traceRecorder
}

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
		evaluator:     summaryCompactionEvaluator(cfg),
		compactionOn:  summaryCompactionEnabled(cfg),
		now:           func() time.Time { return time.Now().UTC() },
	}

	if cfg != nil {
		service.model = cfg.Model
		service.summaryModel = cfg.SummaryModelEffective()
		service.apiMode = cfg.SummaryModelAPIModeEffective()
		service.debugRequests = cfg.DebugRequests
	}

	logEvent := summaryLog.Debug().
		Str("model", service.model).
		Str("summary_model", service.summaryModel).
		Bool("compaction_enabled", service.compactionOn)

	if cfg != nil && cfg.SummaryProviderEffective() != cfg.ModelProvider {
		logEvent = logEvent.Str("summary_provider", cfg.SummaryProviderEffective())
	}

	if cfg != nil && cfg.SummaryModelAPIModeEffective() != cfg.ModelAPIMode {
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
		summaryLog.Debug().Str("session_id", sessionID).
			Int("source_end_offset", state.Current.SourceEndOffset).
			Msg("summary loaded with existing summary")
	} else {
		summaryLog.Debug().Str("session_id", sessionID).Msg("summary loaded")
	}

	return state, nil
}

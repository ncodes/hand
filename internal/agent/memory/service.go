package memory

import (
	"context"
	"errors"
	"time"

	"github.com/wandxy/hand/internal/agent/compaction"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/storage"
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

func NewService(cfg *config.Config, modelClient models.Client, summaryStore SummaryStore) *Service {
	service := &Service{
		modelClient:  modelClient,
		store:        summaryStore,
		evaluator:    summaryCompactionEvaluator(cfg),
		compactionOn: summaryCompactionEnabled(cfg),
		now:          func() time.Time { return time.Now().UTC() },
	}

	if cfg != nil {
		service.model = cfg.Model
		service.summaryModel = cfg.SummaryModelEffective()
		service.apiMode = cfg.ModelAPIMode
		service.debugRequests = cfg.DebugRequests
	}

	memLog.Debug().
		Str("model", service.model).
		Str("summary_model", service.summaryModel).
		Bool("compaction_enabled", service.compactionOn).
		Msg("memory service initialized")

	return service
}

func (s *Service) Load(ctx context.Context, sessionID string) (*Memory, error) {
	if s == nil {
		return nil, errors.New("memory service is required")
	}

	if s.store == nil {
		return nil, errors.New("summary store is required")
	}

	summary, _, err := s.store.GetSummary(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	mem := &Memory{Summary: SummaryFromStorage(summary)}

	if mem.Summary != nil {
		memLog.Debug().Str("session_id", sessionID).
			Int("source_end_offset", mem.Summary.SourceEndOffset).
			Msg("memory loaded with existing summary")
	} else {
		memLog.Debug().Str("session_id", sessionID).Msg("memory loaded")
	}

	return mem, nil
}

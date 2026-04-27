package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage/retrieval"
	base "github.com/wandxy/hand/internal/storage/session"
	common "github.com/wandxy/hand/internal/storage/session/common"
)

const currentSessionStateKey = "current_session"
const sessionMessageSearchTable = "session_message_search"
const defaultVectorStoreRebuildBatchSize = 100
const defaultHybridCandidateLimit = 100
const defaultRerankCandidateLimit = 100
const maxHybridCandidateLimit = 1000
const reciprocalRankFusionConstant = 60

type Session = base.Session
type ArchivedSession = base.ArchivedSession
type MessageQueryOptions = base.MessageQueryOptions
type SearchMessageOptions = base.SearchMessageOptions
type SearchMessageResult = base.SearchMessageResult
type SessionSummary = base.SessionSummary
type SessionCompaction = base.SessionCompaction
type SessionCompactionStatus = base.SessionCompactionStatus
type MessageRecord = base.MessageRecord

// sessionModel stores durable session-level state and compaction progress.
type sessionModel struct {
	ID                           string `gorm:"primaryKey"`
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	LastPromptTokens             int
	CompactionStatus             string
	CompactionRequestedAt        time.Time
	CompactionStartedAt          time.Time
	CompactionCompletedAt        time.Time
	CompactionFailedAt           time.Time
	CompactionLastError          string
	CompactionTargetMessageCount int
	CompactionTargetOffset       int
}

// TableName returns the SQLite table used for active sessions.
func (sessionModel) TableName() string {
	return "sessions"
}

// archiveModel stores metadata for archived session message sets.
type archiveModel struct {
	ID              string    `gorm:"primaryKey"`
	SourceSessionID string    `gorm:"index;not null"`
	ArchivedAt      time.Time `gorm:"index"`
	ExpiresAt       time.Time `gorm:"index"`
	CreatedAt       time.Time
}

// TableName returns the SQLite table used for session archives.
func (archiveModel) TableName() string {
	return "session_archives"
}

// stateModel stores small named session state values such as the current session.
type stateModel struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
	CreatedAt time.Time
}

// TableName returns the SQLite table used for session state entries.
func (stateModel) TableName() string {
	return "session_state"
}

// summaryModel stores the latest generated summary for a session.
type summaryModel struct {
	SessionID          string `gorm:"primaryKey"`
	SourceEndOffset    int
	SourceMessageCount int
	UpdatedAt          time.Time
	SessionSummary     string `gorm:"type:text"`
	CurrentTask        string `gorm:"type:text"`
	Discoveries        string `gorm:"type:text"`
	OpenQuestions      string `gorm:"type:text"`
	NextActions        string `gorm:"type:text"`
}

// TableName returns the SQLite table used for session summaries.
func (summaryModel) TableName() string {
	return "session_summaries"
}

// messageModel stores active session messages in append order.
type messageModel struct {
	ID         uint `gorm:"primaryKey"`
	SessionID  string
	Sequence   int `gorm:"index;not null"`
	Role       string
	Name       string
	Content    string
	ToolCalls  string `gorm:"type:text"`
	ToolCallID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// messageModels is a typed slice for active message conversion helpers.
type messageModels []messageModel

// TableName returns the SQLite table used for active session messages.
func (messageModel) TableName() string {
	return "session_messages"
}

// archivedMessageModel stores messages moved out of an active session archive.
type archivedMessageModel struct {
	ID         uint `gorm:"primaryKey"`
	ArchiveID  string
	Sequence   int `gorm:"index;not null"`
	Role       string
	Name       string
	Content    string
	ToolCalls  string `gorm:"type:text"`
	ToolCallID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// archivedMessageModels is a typed slice for archived message conversion helpers.
type archivedMessageModels []archivedMessageModel

// TableName returns the SQLite table used for archived session messages.
func (archivedMessageModel) TableName() string {
	return "archived_session_messages"
}

// searchSessionResultRow is the intermediate shape used to map ranked SQL rows to public search results.
type searchSessionResultRow struct {
	ID              uint
	SessionID       string
	Sequence        int
	Role            string
	Name            string
	Content         string
	ToolCalls       string
	ToolCallID      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	MatchedText     string
	MatchedToolName string
	Score           float64
	BestScore       float64
	MatchCount      int
	LastMatchedAt   string
}

// searchCandidate is a merged lexical/vector candidate before final grouping.
type searchCandidate struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ID              uint
	SessionID       string
	Role            string
	Name            string
	Content         string
	ToolCalls       string
	ToolCallID      string
	MatchedText     string
	MatchedToolName string
	LexicalScore    float64
	RerankScore     float64
	VectorScore     float64
	FusedScore      float64
	LexicalRank     int
	VectorRank      int
	HasLexical      bool
	HasRerank       bool
	HasVector       bool
}

// searchCandidateSet collects merged search candidates keyed by message row ID.
type searchCandidateSet map[uint]*searchCandidate

// SessionStore persists sessions, messages, search rows, summaries, and optional vector indexing in SQLite.
type SessionStore struct {
	vectors *vectorConfig
	db      *gorm.DB
}

// VectorStoreOptions configures optional vector indexing and hybrid reranking for session search.
type VectorStoreOptions struct {
	Embedder            retrieval.Embedder
	Reranker            retrieval.Reranker
	VectorStore         retrieval.VectorStore
	EnableRerank        *bool
	EmbeddingModel      string
	RebuildBatchSize    int
	RerankMaxCandidates int
	Diagnostics         bool
	Required            bool
}

// vectorConfig holds normalized vector dependencies and operational limits.
type vectorConfig struct {
	provider    retrieval.Embedder
	reranker    retrieval.Reranker
	store       retrieval.VectorStore
	model       string
	batchSize   int
	rerankMax   int
	diagnostics bool
	rerank      bool
	required    bool
}

// vectorInput is the searchable text unit sent to the embedding provider.
type vectorInput struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string
	SourceID  string
	SessionID string
	Role      string
	ToolName  string
	Text      string
}

func NewSessionStore(path string) (*SessionStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("session sqlite path is required")
	}

	backend, err := gormOpenSQLite(path)
	if err != nil {
		return nil, err
	}

	return backend, nil
}

func NewSessionStoreFromDB(db *gorm.DB) (*SessionStore, error) {
	if db == nil {
		return nil, errors.New("session db is required")
	}

	if err := db.AutoMigrate(
		&sessionModel{},
		&archiveModel{},
		&stateModel{},
		&summaryModel{},
		&messageModel{},
		&archivedMessageModel{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate session db: %w", err)
	}

	if err := ensureSearchIndex(db); err != nil {
		return nil, err
	}

	return &SessionStore{db: db}, nil
}

// ConfigureVectorStore enables or disables vector indexing and hybrid search for this store.
func (s *SessionStore) ConfigureVectorStore(opts VectorStoreOptions) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	model := strings.TrimSpace(opts.EmbeddingModel)
	if opts.Embedder == nil && opts.VectorStore == nil && model == "" {
		s.vectors = nil
		return nil
	}
	if opts.Embedder == nil {
		return errors.New("vector store embedding provider is required")
	}
	if opts.VectorStore == nil {
		return errors.New("vector store is required")
	}
	if model == "" {
		return errors.New("vector store embedding model is required")
	}
	batchSize := opts.RebuildBatchSize
	if batchSize < 0 {
		return errors.New("vector store rebuild batch size must be greater than or equal to zero")
	}
	if batchSize == 0 {
		batchSize = defaultVectorStoreRebuildBatchSize
	}
	rerankMax := opts.RerankMaxCandidates
	if rerankMax < 0 {
		return errors.New("vector store rerank max candidates must be greater than or equal to zero")
	}
	if rerankMax == 0 {
		rerankMax = defaultRerankCandidateLimit
	}
	rerankEnabled := true
	if opts.EnableRerank != nil {
		rerankEnabled = *opts.EnableRerank
	}

	s.vectors = &vectorConfig{
		provider:    opts.Embedder,
		reranker:    opts.Reranker,
		store:       opts.VectorStore,
		model:       model,
		batchSize:   batchSize,
		rerankMax:   rerankMax,
		diagnostics: opts.Diagnostics,
		rerank:      rerankEnabled,
		required:    opts.Required,
	}

	return nil
}

// RebuildVectorStore refreshes all vector rows for one active session in batches.
func (s *SessionStore) RebuildVectorStore(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	var session sessionModel
	if err := s.db.WithContext(ctx).First(&session, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("session not found")
		}
		return err
	}
	if s.vectors == nil {
		return nil
	}

	lastSequence := -1
	for {
		var records []messageModel
		if err := s.db.WithContext(ctx).
			Where("session_id = ? AND sequence > ?", id, lastSequence).
			Order("sequence asc").
			Limit(s.vectors.batchSize).
			Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}

		if err := s.deleteVectorRows(ctx, messageModels(records).sourceIDs()); err != nil {
			if s.vectors.required {
				return err
			}
		}
		if err := s.indexVectors(ctx, records); err != nil {
			return s.handleVectorStoreError(err)
		}

		lastSequence = records[len(records)-1].Sequence
	}
}

func gormOpenSQLite(path string) (*SessionStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("session sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session db directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open session db: %w", err)
	}

	return NewSessionStoreFromDB(db)
}

// Save upserts session metadata without modifying stored messages.
func (s *SessionStore) Save(ctx context.Context, session Session) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	session.ID = strings.TrimSpace(session.ID)
	if err := common.ValidateSessionID(session.ID); err != nil {
		return err
	}

	var existing sessionModel

	if err := s.db.WithContext(ctx).First(&existing, "id = ?", session.ID).Error; err == nil {
		session.CreatedAt = existing.CreatedAt

		if session.Compaction == (base.SessionCompaction{}) {
			session.Compaction = base.SessionCompaction{
				CompletedAt:        existing.CompactionCompletedAt,
				FailedAt:           existing.CompactionFailedAt,
				LastError:          existing.CompactionLastError,
				RequestedAt:        existing.CompactionRequestedAt,
				StartedAt:          existing.CompactionStartedAt,
				Status:             base.SessionCompactionStatus(existing.CompactionStatus),
				TargetMessageCount: existing.CompactionTargetMessageCount,
				TargetOffset:       existing.CompactionTargetOffset,
			}
		}

		session.UpdatedAt = time.Now().UTC()
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	} else {
		session.CreatedAt = session.CreatedAt.UTC()
	}

	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	} else {
		session.UpdatedAt = session.UpdatedAt.UTC()
	}

	record := sessionModel{
		CreatedAt:                    session.CreatedAt,
		CompactionCompletedAt:        session.Compaction.CompletedAt,
		CompactionFailedAt:           session.Compaction.FailedAt,
		CompactionLastError:          session.Compaction.LastError,
		CompactionRequestedAt:        session.Compaction.RequestedAt,
		CompactionStartedAt:          session.Compaction.StartedAt,
		CompactionStatus:             string(session.Compaction.Status),
		CompactionTargetMessageCount: session.Compaction.TargetMessageCount,
		CompactionTargetOffset:       session.Compaction.TargetOffset,
		ID:                           session.ID,
		LastPromptTokens:             session.LastPromptTokens,
		UpdatedAt:                    session.UpdatedAt,
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Save(&record).Error
	})
}

// Get loads one active session by ID.
func (s *SessionStore) Get(ctx context.Context, id string) (Session, bool, error) {
	if s == nil || s.db == nil {
		return Session{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, false, nil
	}

	if err := common.ValidateSessionID(id); err != nil {
		return Session{}, false, err
	}

	var record sessionModel
	if err := s.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}

	session, err := sessionModelToSession(record)
	if err != nil {
		return Session{}, false, err
	}

	return session, true, nil
}

// List returns active sessions ordered by most recently updated.
func (s *SessionStore) List(ctx context.Context) ([]Session, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	var records []sessionModel
	if err := s.db.WithContext(ctx).Order("updated_at desc").Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(records))
	for _, record := range records {
		session, err := sessionModelToSession(record)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Delete removes an active session and its messages, summaries, search rows, and vector rows.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	if id == base.DefaultSessionID {
		return errors.New("default session cannot be deleted")
	}

	var sourceIDs []string
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sessionModel

		if err := tx.First(&session, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		var messageIDs []uint
		if err := tx.Model(&messageModel{}).
			Where("session_id = ?", id).
			Pluck("id", &messageIDs).Error; err != nil {
			return err
		}
		sourceIDs = sourceIDsFromMessageIDs(id, messageIDs)

		if err := tx.Where("session_id = ?", id).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSearchRows(tx, id); err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", id).Delete(&summaryModel{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&session).Error; err != nil {
			return err
		}

		if err := tx.Where("key = ? AND value = ?", currentSessionStateKey, id).
			Delete(&stateModel{}).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	if err := s.deleteVectorRows(ctx, sourceIDs); err != nil {
		return s.handleVectorStoreError(err)
	}

	return nil
}

// AppendMessages appends messages to a session and updates lexical/vector search indexes.
func (s *SessionStore) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	var records []messageModel
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record sessionModel

		if err := tx.First(&record, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		var nextSequence int64
		if err := tx.Model(&messageModel{}).Where("session_id = ?", id).
			Count(&nextSequence).Error; err != nil {
			return err
		}

		records = messagesToMessageModelsWithOffset(id, messages, int(nextSequence))
		if len(records) > 0 {
			if err := tx.Create(&records).Error; err != nil {
				return err
			}
			if err := messageModels(records).searchRows().insert(tx); err != nil {
				return err
			}
		}

		record.UpdatedAt = time.Now().UTC()

		return tx.Save(&record).Error
	}); err != nil {
		return err
	}

	if err := s.indexVectors(ctx, records); err != nil {
		return s.handleVectorStoreError(err)
	}

	return nil
}

// GetMessages loads active or archived messages with role, name, order, offset, and limit filters.
func (s *SessionStore) GetMessages(
	ctx context.Context,
	id string,
	opts MessageQueryOptions,
) ([]handmsg.Message, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return nil, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}

	if !opts.Archived {
		if err := common.ValidateSessionID(id); err != nil {
			return nil, err
		}
	} else if err := common.ValidateArchiveID(id); err != nil {
		return nil, err
	}

	offset := max(opts.Offset, 0)

	if opts.Archived {
		var records []archivedMessageModel
		query := applyArchivedMessageFilters(s.db.WithContext(ctx), id, opts).
			Order("sequence " + messageQueryOrder(opts)).
			Offset(offset)
		if opts.Limit > 0 {
			query = query.Limit(opts.Limit)
		}
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}

		return archivedMessageModels(records).messages(), nil
	}

	var records []messageModel
	query := applySessionMessageFilters(s.db.WithContext(ctx), id, opts).
		Order("sequence " + messageQueryOrder(opts)).
		Offset(offset)
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	return messageModels(records).messages(), nil
}

// GetMessagesByIDs loads active session messages by database row IDs.
func (s *SessionStore) GetMessagesByIDs(
	ctx context.Context,
	id string,
	messageIDs []uint,
) ([]base.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || len(messageIDs) == 0 {
		return nil, nil
	}
	if err := common.ValidateSessionID(id); err != nil {
		return nil, err
	}

	var records []messageModel
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id IN ?", id, messageIDs).
		Order("sequence asc").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return messageModels(records).records(), nil
}

// GetMessageWindow returns messages surrounding an anchor message in sequence order.
func (s *SessionStore) GetMessageWindow(
	ctx context.Context,
	id string,
	anchorMessageID uint,
	before int,
	after int,
) ([]base.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || anchorMessageID == 0 {
		return nil, nil
	}
	if err := common.ValidateSessionID(id); err != nil {
		return nil, err
	}
	if before < 0 || after < 0 {
		return nil, errors.New("before and after must be greater than or equal to zero")
	}

	var anchor messageModel
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id = ?", id, anchorMessageID).
		First(&anchor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	start := max(anchor.Sequence-before, 0)
	end := anchor.Sequence + after + 1

	var records []messageModel
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND sequence >= ? AND sequence < ?", id, start, end).
		Order("sequence asc").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return messageModels(records).records(), nil
}

// CountMessages counts active or archived messages matching the supplied filters.
func (s *SessionStore) CountMessages(
	ctx context.Context,
	id string,
	opts MessageQueryOptions,
) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("session store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return 0, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil
	}

	if !opts.Archived {
		if err := common.ValidateSessionID(id); err != nil {
			return 0, err
		}
	} else if err := common.ValidateArchiveID(id); err != nil {
		return 0, err
	}

	var count int64
	if opts.Archived {
		if err := applyArchivedMessageFilters(s.db.WithContext(ctx), id, opts).
			Count(&count).Error; err != nil {
			return 0, err
		}
		return int(count), nil
	}

	if err := applySessionMessageFilters(s.db.WithContext(ctx), id, opts).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return int(count), nil
}

// SearchMessages runs BM25 search or hybrid BM25/vector search depending on configuration.
func (s *SessionStore) SearchMessages(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
) ([]base.SearchMessageResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id != "" {
		if err := common.ValidateSessionID(id); err != nil {
			return nil, err
		}
	} else if opts.IgnoreSessionID = strings.TrimSpace(opts.IgnoreSessionID); opts.IgnoreSessionID != "" {
		if err := common.ValidateSessionID(opts.IgnoreSessionID); err != nil {
			return nil, err
		}
	}

	queryText := buildSearchQuery(opts.Query)
	if queryText == "" {
		return nil, nil
	}

	if s.vectors != nil {
		return s.searchMessagesHybrid(ctx, id, opts, queryText)
	}

	records, err := s.searchMessagesLexical(ctx, id, opts, queryText, 0, true)
	if err != nil {
		return nil, err
	}

	return searchMessageResultRowsToResults(records), nil
}

// searchMessagesLexical returns BM25-ranked message rows with optional early limiting.
func (s *SessionStore) searchMessagesLexical(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	queryText string,
	rowLimit int,
	applyLimits bool,
) ([]searchSessionResultRow, error) {

	args := []any{queryText}
	var sql strings.Builder
	sql.WriteString(`
-- Read matching FTS rows and compute BM25; lower scores are more relevant.
WITH raw_hits AS (
	SELECT
		rowid AS search_rowid,
		CAST(message_id AS INTEGER) AS message_id,
		session_id,
		body AS matched_text,
		tool_name AS matched_tool_name,
		bm25(`)
	sql.WriteString(sessionMessageSearchTable)
	sql.WriteString(`) AS score
	FROM `)
	sql.WriteString(sessionMessageSearchTable)
	sql.WriteString(`
	WHERE body MATCH ?`)
	if id != "" {
		sql.WriteString(` AND session_id = ?`)
		args = append(args, id)
	} else if opts.IgnoreSessionID != "" {
		sql.WriteString(` AND session_id <> ?`)
		args = append(args, opts.IgnoreSessionID)
	}
	if role := strings.TrimSpace(string(opts.Role)); role != "" {
		sql.WriteString(` AND role = ?`)
		args = append(args, role)
	}
	if toolName := normalizeSearchValue(opts.ToolName); toolName != "" {
		sql.WriteString(` AND tool_name = ?`)
		args = append(args, toolName)
	}
	sql.WriteString(`
),
-- Collapse duplicate indexed rows for the same message to the best hit.
ranked_hits AS (
	SELECT
		message_id,
		session_id,
		matched_text,
		matched_tool_name,
		score,
		ROW_NUMBER() OVER (
			PARTITION BY message_id
			ORDER BY score ASC, CASE WHEN matched_tool_name <> '' THEN 0 ELSE 1 END, search_rowid ASC
		) AS hit_rank
	FROM raw_hits
),

-- Join the selected FTS hits back to durable message rows.
message_hits AS (
	SELECT
		m.id,
		m.session_id,
		m.sequence,
		m.role,
		m.name,
		m.content,
		m.tool_calls,
		m.tool_call_id,
		m.created_at,
		m.updated_at,
		hits.matched_text,
		hits.matched_tool_name,
		hits.score
	FROM session_messages AS m
	JOIN ranked_hits AS hits ON hits.message_id = m.id AND hits.hit_rank = 1
),

-- Aggregate per-session metadata used for grouping and session ranking.
session_groups AS (
	SELECT
		session_id,
		COUNT(*) AS match_count,
		MIN(score) AS best_score,
		strftime('%Y-%m-%dT%H:%M:%fZ', MAX(created_at)) AS last_matched_at
	FROM message_hits
	GROUP BY session_id
),

-- Rank sessions by best message relevance, then latest matching message.
ranked_sessions AS (
	SELECT
		session_id,
		match_count,
		best_score,
		last_matched_at,
		ROW_NUMBER() OVER (
			ORDER BY best_score ASC, last_matched_at DESC, session_id ASC
		) AS session_rank
	FROM session_groups
),

-- Rank messages inside each selected session by relevance, then recency.
ranked_session_hits AS (
	SELECT
		mh.id,
		mh.session_id,
		mh.sequence,
		mh.role,
		mh.name,
		mh.content,
		mh.tool_calls,
		mh.tool_call_id,
		mh.created_at,
		mh.updated_at,
		mh.matched_text,
		mh.matched_tool_name,
		mh.score,
		rs.match_count,
		rs.best_score,
		rs.last_matched_at,
		ROW_NUMBER() OVER (
			PARTITION BY mh.session_id
			ORDER BY mh.score ASC, mh.created_at DESC, mh.id DESC
	) AS message_rank
	FROM message_hits AS mh
	JOIN ranked_sessions AS rs ON rs.session_id = mh.session_id`)
	if applyLimits && opts.MaxSessions > 0 {
		sql.WriteString(`
	WHERE rs.session_rank <= ?`)
		args = append(args, opts.MaxSessions)
	}
	sql.WriteString(`
)

-- Emit ranked rows for the Go result mapper.
SELECT
	id,
	session_id,
	sequence,
	role,
	name,
	content,
	tool_calls,
	tool_call_id,
	created_at,
	updated_at,
	matched_text,
	matched_tool_name,
	score,
	match_count,
	best_score,
	last_matched_at
FROM ranked_session_hits`)
	if applyLimits && opts.MaxMessagesPerSession > 0 {
		sql.WriteString(`
WHERE message_rank <= ?`)
		args = append(args, opts.MaxMessagesPerSession)
	}
	sql.WriteString(`
ORDER BY best_score ASC, last_matched_at DESC, session_id ASC, score ASC, created_at DESC, id DESC`)
	if rowLimit > 0 {
		sql.WriteString(`
LIMIT ?`)
		args = append(args, rowLimit)
	}

	var records []searchSessionResultRow
	if err := s.db.WithContext(ctx).Raw(sql.String(), args...).Scan(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

func (s *SessionStore) rerankEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.rerank
}

func (s *SessionStore) diagnosticsEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.diagnostics
}

// searchMessagesHybrid merges lexical and vector candidates, reranks them, and maps them to public results.
func (s *SessionStore) searchMessagesHybrid(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	queryText string,
) ([]base.SearchMessageResult, error) {

	candidateLimit := hybridCandidateLimit(opts)
	lexicalRows, err := s.searchMessagesLexical(ctx, id, opts, queryText, candidateLimit, false)
	if err != nil {
		return nil, err
	}
	candidates := searchCandidatesFromLexicalRows(lexicalRows)

	s.logSearchEvent("lexical candidates gathered", id, opts).
		Int("candidate_count", len(candidates)).
		Int("row_count", len(lexicalRows)).
		Msg("session search lexical candidates gathered")

	vectorRows, err := s.searchMessagesVector(ctx, id, opts, candidateLimit)
	if err != nil {
		s.logSearchEvent("vector search failed", id, opts).Err(err).Msg("session search vector search failed")
		return nil, err
	}

	s.logSearchEvent("vector candidates gathered", id, opts).
		Int("candidate_count", len(vectorRows)).
		Msg("session search vector candidates gathered")

	beforeMerge := len(candidates)
	candidates.merge(vectorRows)
	if len(candidates) == 0 {
		s.logSearchEvent("no candidates", id, opts).Msg("session search returned no hybrid candidates")
		return nil, nil
	}

	s.logSearchEvent("hybrid candidates merged", id, opts).
		Int("lexical_candidate_count", beforeMerge).
		Int("vector_candidate_count", len(vectorRows)).
		Int("merged_candidate_count", len(candidates)).
		Msg("session search hybrid candidates merged")

	s.logCandidateDiagnostics("candidate merged", candidates.sorted())

	matchCounts, lastMatchedAt := candidates.sessionStats()
	reranked := candidates.sorted()
	if s.rerankEnabled() {
		reranked = s.rerankSearchCandidates(ctx, opts, candidates)
		s.logCandidateDiagnostics("candidate reranked", reranked)
	} else {
		s.logSearchEvent("rerank skipped", id, opts).Msg("session search rerank skipped")
	}
	rows := rankedSearchRowsFromCandidateSlice(reranked, opts, matchCounts, lastMatchedAt)
	results := searchMessageResultRowsToResults(rows)

	s.logSearchEvent("results ranked", id, opts).
		Int("session_count", len(results)).
		Int("message_count", len(rows)).
		Msg("session search hybrid results ranked")

	return results, nil
}

func searchCandidatesFromLexicalRows(rows []searchSessionResultRow) searchCandidateSet {
	candidates := make(searchCandidateSet, len(rows))
	for idx, row := range rows {
		candidates[row.ID] = &searchCandidate{
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
			ID:              row.ID,
			SessionID:       row.SessionID,
			Role:            row.Role,
			Name:            row.Name,
			Content:         row.Content,
			ToolCalls:       row.ToolCalls,
			ToolCallID:      row.ToolCallID,
			MatchedText:     row.MatchedText,
			MatchedToolName: row.MatchedToolName,
			LexicalScore:    row.Score,
			LexicalRank:     idx + 1,
			HasLexical:      true,
		}
	}

	return candidates
}

// merge combines vector evidence into existing lexical candidates by message ID.
func (candidates searchCandidateSet) merge(vectorCandidates []*searchCandidate) {
	for _, vectorCandidate := range vectorCandidates {
		if vectorCandidate == nil {
			continue
		}
		candidate, ok := candidates[vectorCandidate.ID]
		if !ok {
			candidates[vectorCandidate.ID] = vectorCandidate
			continue
		}

		candidate.VectorScore = vectorCandidate.VectorScore
		candidate.VectorRank = vectorCandidate.VectorRank
		candidate.HasVector = true
		if strings.TrimSpace(candidate.MatchedText) == "" {
			candidate.MatchedText = vectorCandidate.MatchedText
			candidate.MatchedToolName = vectorCandidate.MatchedToolName
		}
	}
}

func rankedSearchRowsFromCandidates(
	candidates searchCandidateSet,
	opts base.SearchMessageOptions,
) []searchSessionResultRow {
	return rankedSearchRowsFromCandidateSlice(candidates.sorted(), opts, nil, nil)
}

func rankedSearchRowsFromCandidateSlice(
	candidates []*searchCandidate,
	opts base.SearchMessageOptions,
	matchCounts map[string]int,
	lastMatchedAt map[string]time.Time,
) []searchSessionResultRow {
	groups := make(map[string][]*searchCandidate)
	for _, candidate := range candidates {
		groups[candidate.SessionID] = append(groups[candidate.SessionID], candidate)
	}

	sessions := make([]string, 0, len(groups))
	bestScoreBySession := make(map[string]float64, len(groups))
	lastMatchedBySession := make(map[string]time.Time, len(groups))
	for sessionID, sessionCandidates := range groups {
		slices.SortStableFunc(sessionCandidates, compareCandidatesWithinSession)
		bestScoreBySession[sessionID] = searchCandidateRankingScore(sessionCandidates[0])
		for _, candidate := range sessionCandidates {
			if candidate.CreatedAt.After(lastMatchedBySession[sessionID]) {
				lastMatchedBySession[sessionID] = candidate.CreatedAt
			}
		}
		if lastMatchedAt[sessionID].After(lastMatchedBySession[sessionID]) {
			lastMatchedBySession[sessionID] = lastMatchedAt[sessionID]
		}
		sessions = append(sessions, sessionID)
	}

	slices.SortStableFunc(sessions, func(left string, right string) int {
		return compareRankedSessions(left, right, bestScoreBySession, lastMatchedBySession)
	})
	if opts.MaxSessions > 0 && len(sessions) > opts.MaxSessions {
		sessions = sessions[:opts.MaxSessions]
	}

	rows := make([]searchSessionResultRow, 0, len(candidates))
	for _, sessionID := range sessions {
		sessionCandidates := groups[sessionID]
		if opts.MaxMessagesPerSession > 0 && len(sessionCandidates) > opts.MaxMessagesPerSession {
			sessionCandidates = sessionCandidates[:opts.MaxMessagesPerSession]
		}
		lastMatchedAt := ""
		if !lastMatchedBySession[sessionID].IsZero() {
			lastMatchedAt = lastMatchedBySession[sessionID].UTC().Format(time.RFC3339Nano)
		}
		for _, candidate := range sessionCandidates {
			rows = append(rows, searchSessionResultRow{
				CreatedAt:       candidate.CreatedAt,
				UpdatedAt:       candidate.UpdatedAt,
				ID:              candidate.ID,
				SessionID:       candidate.SessionID,
				Sequence:        0,
				Role:            candidate.Role,
				Name:            candidate.Name,
				Content:         candidate.Content,
				ToolCalls:       candidate.ToolCalls,
				ToolCallID:      candidate.ToolCallID,
				MatchedText:     candidate.MatchedText,
				MatchedToolName: candidate.MatchedToolName,
				Score:           searchCandidateRankingScore(candidate),
				BestScore:       bestScoreBySession[sessionID],
				MatchCount:      searchCandidateMatchCount(sessionID, groups, matchCounts),
				LastMatchedAt:   lastMatchedAt,
			})
		}
	}

	return rows
}

func compareCandidatesWithinSession(left *searchCandidate, right *searchCandidate) int {
	leftScore := searchCandidateRankingScore(left)
	rightScore := searchCandidateRankingScore(right)
	if leftScore != rightScore {
		if leftScore > rightScore {
			return -1
		}
		return 1
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if left.CreatedAt.After(right.CreatedAt) {
			return -1
		}
		return 1
	}
	if left.ID > right.ID {
		return -1
	}
	if left.ID < right.ID {
		return 1
	}
	return 0
}

func compareRankedSessions(
	left string,
	right string,
	bestScoreBySession map[string]float64,
	lastMatchedBySession map[string]time.Time,
) int {
	if bestScoreBySession[left] != bestScoreBySession[right] {
		if bestScoreBySession[left] > bestScoreBySession[right] {
			return -1
		}
		return 1
	}
	if !lastMatchedBySession[left].Equal(lastMatchedBySession[right]) {
		if lastMatchedBySession[left].After(lastMatchedBySession[right]) {
			return -1
		}
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// sessionStats returns per-session candidate counts and latest candidate timestamps.
func (candidates searchCandidateSet) sessionStats() (map[string]int, map[string]time.Time) {
	matchCounts := make(map[string]int)
	lastMatchedAt := make(map[string]time.Time)
	for _, candidate := range candidates {
		matchCounts[candidate.SessionID]++
		if candidate.CreatedAt.After(lastMatchedAt[candidate.SessionID]) {
			lastMatchedAt[candidate.SessionID] = candidate.CreatedAt
		}
	}

	return matchCounts, lastMatchedAt
}

func searchCandidateMatchCount(
	sessionID string,
	groups map[string][]*searchCandidate,
	matchCounts map[string]int,
) int {
	if matchCounts[sessionID] > 0 {
		return matchCounts[sessionID]
	}

	return len(groups[sessionID])
}

// sorted returns candidates ordered by current ranking score and deterministic tie-breaks.
func (candidates searchCandidateSet) sorted() []*searchCandidate {
	items := make([]*searchCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.FusedScore = fusedSearchCandidateScore(candidate)
		items = append(items, candidate)
	}
	slices.SortStableFunc(items, compareSearchCandidates)

	return items
}

func compareSearchCandidates(left *searchCandidate, right *searchCandidate) int {
	leftScore := searchCandidateRankingScore(left)
	rightScore := searchCandidateRankingScore(right)
	if leftScore != rightScore {
		if leftScore > rightScore {
			return -1
		}
		return 1
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if left.CreatedAt.After(right.CreatedAt) {
			return -1
		}
		return 1
	}
	if left.SessionID != right.SessionID {
		return strings.Compare(left.SessionID, right.SessionID)
	}
	if left.ID > right.ID {
		return -1
	}
	if left.ID < right.ID {
		return 1
	}
	return 0
}

func searchCandidateRankingScore(candidate *searchCandidate) float64 {
	if candidate.HasRerank {
		return candidate.RerankScore
	}

	return candidate.FusedScore
}

// fusedSearchCandidateScore computes the fused score for a search candidate using
// the reciprocal rank fusion constant.
func fusedSearchCandidateScore(candidate *searchCandidate) float64 {
	var score float64
	if candidate.HasLexical && candidate.LexicalRank > 0 {
		score += 1 / float64(reciprocalRankFusionConstant+candidate.LexicalRank)
	}
	if candidate.HasVector && candidate.VectorRank > 0 {
		score += 1 / float64(reciprocalRankFusionConstant+candidate.VectorRank)
	}

	return score
}

func hybridCandidateLimit(opts base.SearchMessageOptions) int {
	limit := defaultHybridCandidateLimit
	if opts.MaxSessions > 0 && opts.MaxMessagesPerSession > 0 {
		limit = max(limit, opts.MaxSessions*opts.MaxMessagesPerSession)
	}
	if limit > maxHybridCandidateLimit {
		limit = maxHybridCandidateLimit
	}

	return limit
}

// rerankSearchCandidates converts merged search candidates to the shared retrieval reranker contract.
func (s *SessionStore) rerankSearchCandidates(
	ctx context.Context,
	opts base.SearchMessageOptions,
	candidates searchCandidateSet,
) []*searchCandidate {
	items := candidates.sorted()
	if len(items) == 0 {
		return nil
	}

	maxCandidates := defaultRerankCandidateLimit
	reranker := retrieval.Reranker(retrieval.DeterministicReranker{})
	if s != nil && s.vectors != nil {
		maxCandidates = s.vectors.rerankMax
		if s.vectors.reranker != nil {
			reranker = s.vectors.reranker
		}
	}
	if maxCandidates > 0 && len(items) > maxCandidates {
		items = items[:maxCandidates]
	}
	s.logSearchEvent("rerank started", "", opts).
		Int("candidate_count", len(items)).
		Int("max_candidates", maxCandidates).
		Msg("session search rerank started")

	retrievalCandidates := make([]retrieval.Candidate, 0, len(items))
	searchCandidateByID := make(map[string]*searchCandidate, len(items))
	for _, candidate := range items {
		retrievalCandidate := retrievalCandidateFromSearchCandidate(candidate)
		retrievalCandidates = append(retrievalCandidates, retrievalCandidate)
		searchCandidateByID[retrievalCandidate.ID] = candidate
	}

	result, err := retrieval.RerankWithFallback(ctx, reranker, retrieval.DeterministicReranker{}, retrieval.RerankRequest{
		Query:      strings.TrimSpace(opts.Query),
		Caller:     "session_search",
		SourceKind: retrieval.SourceKindSessionMessage,
		Candidates: retrievalCandidates,
		Options: retrieval.RerankOptions{
			LexicalDirection: retrieval.ScoreLowerIsBetter,
			VectorDirection:  retrieval.ScoreHigherIsBetter,
			FusedDirection:   retrieval.ScoreHigherIsBetter,
		},
	})
	if err != nil {
		s.logSearchEvent("rerank fallback failed", "", opts).
			Err(err).
			Int("candidate_count", len(items)).
			Msg("session search rerank fallback failed")
		return items
	}

	reranked := make([]*searchCandidate, 0, len(result.Items))
	for _, item := range result.Items {
		candidate := searchCandidateByID[item.CandidateID]
		candidate.RerankScore = item.Score
		candidate.HasRerank = true
		reranked = append(reranked, candidate)
	}

	s.logSearchEvent("rerank completed", "", opts).
		Int("candidate_count", len(items)).
		Int("result_count", len(reranked)).
		Msg("session search rerank completed")

	return reranked
}

func retrievalCandidateFromSearchCandidate(candidate *searchCandidate) retrieval.Candidate {
	text := strings.TrimSpace(candidate.MatchedText)
	if text == "" {
		text = strings.TrimSpace(candidate.Content)
	}

	return retrieval.Candidate{
		CreatedAt:    candidate.CreatedAt,
		UpdatedAt:    candidate.UpdatedAt,
		ID:           sourceIDForMessage(candidate.SessionID, candidate.ID),
		SourceKind:   retrieval.SourceKindSessionMessage,
		SessionID:    candidate.SessionID,
		Text:         text,
		LexicalScore: candidate.LexicalScore,
		VectorScore:  candidate.VectorScore,
		FusedScore:   candidate.FusedScore,
		MessageID:    candidate.ID,
	}
}

// searchMessagesVector embeds the query and asks the vector store for session-message candidates.
func (s *SessionStore) searchMessagesVector(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	candidateLimit int,
) ([]*searchCandidate, error) {
	embeddingReq := retrieval.EmbeddingRequest{
		Model: strings.TrimSpace(s.vectors.model),
		Inputs: []retrieval.EmbeddingInput{{
			ID:         "query",
			Text:       strings.TrimSpace(opts.Query),
			SourceKind: retrieval.SourceKindSessionMessage,
		}},
	}

	s.logSearchEvent("query embedding started", id, opts).
		Str("embedding_model", embeddingReq.Model).
		Msg("session search query embedding started")

	embedding, err := s.vectors.provider.Embed(ctx, embeddingReq)
	if err != nil {
		s.logSearchEvent("query embedding failed", id, opts).Err(err).Msg("session search query embedding failed")
		return nil, err
	}
	if err := retrieval.ValidateEmbeddingResult(embeddingReq, embedding); err != nil {
		s.logSearchEvent("query embedding validation failed", id, opts).Err(err).Msg("session search query embedding validation failed")
		return nil, err
	}

	s.logSearchEvent("query embedding completed", id, opts).
		Int("dimensions", embedding.Dimensions).
		Str("embedding_model", strings.TrimSpace(embedding.Model)).
		Msg("session search query embedding completed")

	result, err := s.vectors.store.Search(ctx, retrieval.VectorSearchRequest{
		EmbeddingModel: strings.TrimSpace(s.vectors.model),
		Dimensions:     embedding.Dimensions,
		QueryVector:    embedding.Items[0].Vector,
		Limit:          candidateLimit,
		Filter: retrieval.VectorFilter{
			SourceKind:      retrieval.SourceKindSessionMessage,
			SessionID:       id,
			IgnoreSessionID: opts.IgnoreSessionID,
			Role:            strings.TrimSpace(string(opts.Role)),
			ToolName:        normalizeSearchValue(opts.ToolName),
		},
	})
	if err != nil {
		s.logSearchEvent("vector search failed", id, opts).Err(err).Msg("session search vector retrieval failed")
		return nil, err
	}

	s.logSearchEvent("vector search completed", id, opts).
		Int("match_count", len(result.Matches)).
		Int("limit", candidateLimit).
		Int("dimensions", embedding.Dimensions).
		Msg("session search vector retrieval completed")

	return s.vectorMatchesToCandidates(ctx, id, opts, result.Matches)
}

// vectorMatchesToCandidates resolves vector hits back to durable messages and searchable rows.
func (s *SessionStore) vectorMatchesToCandidates(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	matches []retrieval.VectorSearchMatch,
) ([]*searchCandidate, error) {
	if len(matches) == 0 {
		return nil, nil
	}

	refs := make(messageRefs, 0, len(matches))
	for _, match := range matches {
		ref, ok := messageRefFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil, nil
	}

	records, err := s.messagesByRef(ctx, refs)
	if err != nil {
		return nil, err
	}

	candidates := make([]*searchCandidate, 0, len(matches))
	seen := map[uint]struct{}{}
	for idx, match := range matches {
		ref, ok := messageRefFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		record, ok := records.get(ref)
		if !ok || !vectorRecordMatchesOptions(record, id, opts) {
			continue
		}
		if _, ok := seen[record.ID]; ok {
			continue
		}
		row, ok := searchRowForVectorRecord(record, match.Record.ID)
		if !ok || !searchRowMatchesOptions(row, opts) {
			continue
		}

		candidates = append(candidates, &searchCandidate{
			CreatedAt:       record.CreatedAt,
			UpdatedAt:       record.UpdatedAt,
			ID:              record.ID,
			SessionID:       record.SessionID,
			Role:            record.Role,
			Name:            record.Name,
			Content:         record.Content,
			ToolCalls:       record.ToolCalls,
			ToolCallID:      record.ToolCallID,
			MatchedText:     row.Body,
			MatchedToolName: row.ToolName,
			VectorScore:     match.Score,
			VectorRank:      idx + 1,
			HasVector:       true,
		})
		seen[record.ID] = struct{}{}
	}

	return candidates, nil
}

// messagesByRef loads active messages for unique session/message references.
func (s *SessionStore) messagesByRef(ctx context.Context, refs messageRefs) (messageLookup, error) {
	refs = refs.unique()
	records := make(messageLookup, len(refs))
	if len(refs) == 0 {
		return records, nil
	}

	where, args := refs.tupleCondition()
	var rows []messageModel
	if err := s.db.WithContext(ctx).Where("(session_id, id) IN ("+where+")", args...).Find(&rows).Error; err != nil {
		return nil, err
	}

	records = make(messageLookup, len(rows))
	for _, row := range rows {
		records.set(messageRef{SessionID: row.SessionID, MessageID: row.ID}, row)
	}
	return records, nil
}

// messageRef identifies a message row within a session.
type messageRef struct {
	SessionID string
	MessageID uint
}

// messageRefs is a typed slice for building tuple queries by message reference.
type messageRefs []messageRef

// messageLookup stores messages keyed by session/message reference.
type messageLookup map[string]messageModel

// unique removes duplicate message references while preserving first occurrence order.
func (refs messageRefs) unique() messageRefs {
	seen := make(map[string]struct{}, len(refs))
	unique := make(messageRefs, 0, len(refs))
	for _, ref := range refs {
		key := ref.key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, ref)
	}

	return unique
}

// tupleCondition builds a SQLite row-value placeholder list for session/message references.
func (refs messageRefs) tupleCondition() (string, []any) {
	var where strings.Builder
	args := make([]any, 0, len(refs)*2)
	for _, ref := range refs {
		if where.Len() > 0 {
			where.WriteString(", ")
		}
		where.WriteString("(?, ?)")
		args = append(args, ref.SessionID, ref.MessageID)
	}

	return where.String(), args
}

// key returns a stable map key for this message reference.
func (r messageRef) key() string {
	return fmt.Sprintf("%s:%d", r.SessionID, r.MessageID)
}

// get returns the message for a reference.
func (lookup messageLookup) get(ref messageRef) (messageModel, bool) {
	record, ok := lookup[ref.key()]
	return record, ok
}

// set stores a message for a reference.
func (lookup messageLookup) set(ref messageRef, record messageModel) {
	lookup[ref.key()] = record
}

func messageRefFromSourceID(sourceID string) (messageRef, bool) {
	value, ok := strings.CutPrefix(sourceID, string(retrieval.SourceKindSessionMessage)+":")
	if !ok {
		return messageRef{}, false
	}
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return messageRef{}, false
	}
	messageID, err := strconv.ParseUint(value[idx+1:], 10, 64)
	if err != nil || messageID == 0 {
		return messageRef{}, false
	}

	return messageRef{SessionID: value[:idx], MessageID: uint(messageID)}, true
}

func vectorRecordMatchesOptions(record messageModel, id string, opts base.SearchMessageOptions) bool {
	if id != "" && record.SessionID != id {
		return false
	}
	if opts.IgnoreSessionID != "" && record.SessionID == opts.IgnoreSessionID {
		return false
	}
	if role := strings.TrimSpace(string(opts.Role)); role != "" && record.Role != role {
		return false
	}

	return true
}

func searchRowForVectorRecord(record messageModel, vectorID string) (searchRow, bool) {
	rows := searchRowsFromMessageModel(record)
	if len(rows) == 0 {
		return searchRow{}, false
	}

	idx := strings.LastIndex(vectorID, ":row:")
	if idx < 0 {
		return searchRow{}, false
	}
	rowNumber, err := strconv.Atoi(vectorID[idx+5:])
	if err != nil || rowNumber <= 0 || rowNumber > len(rows) {
		return searchRow{}, false
	}

	return rows[rowNumber-1], true
}

func searchRowMatchesOptions(row searchRow, opts base.SearchMessageOptions) bool {
	if toolName := normalizeSearchValue(opts.ToolName); toolName != "" && row.ToolName != toolName {
		return false
	}

	return true
}

func applySessionMessageFilters(query *gorm.DB, id string, opts MessageQueryOptions) *gorm.DB {
	query = query.Model(&messageModel{}).Where("session_id = ?", id)
	if role := strings.TrimSpace(string(opts.Role)); role != "" {
		query = query.Where("role = ?", role)
	}
	if name := strings.TrimSpace(opts.Name); name != "" {
		query = query.Where("name = ?", name)
	}
	return query
}

func applyArchivedMessageFilters(query *gorm.DB, id string, opts MessageQueryOptions) *gorm.DB {
	query = query.Model(&archivedMessageModel{}).Where("archive_id = ?", id)
	if role := strings.TrimSpace(string(opts.Role)); role != "" {
		query = query.Where("role = ?", role)
	}
	if name := strings.TrimSpace(opts.Name); name != "" {
		query = query.Where("name = ?", name)
	}
	return query
}

func messageQueryOrder(opts MessageQueryOptions) string {
	order, err := base.NormalizeMessageQueryOrder(opts.Order)
	if err != nil {
		return base.MessageOrderAsc
	}

	return order
}

// GetMessage loads one active or archived message by its sequence index.
func (s *SessionStore) GetMessage(
	ctx context.Context,
	id string,
	index int,
	opts MessageQueryOptions,
) (handmsg.Message, bool, error) {
	if s == nil || s.db == nil {
		return handmsg.Message{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || index < 0 {
		return handmsg.Message{}, false, nil
	}

	if !opts.Archived {
		if err := common.ValidateSessionID(id); err != nil {
			return handmsg.Message{}, false, err
		}
	} else if err := common.ValidateArchiveID(id); err != nil {
		return handmsg.Message{}, false, err
	}

	if opts.Archived {
		var record archivedMessageModel
		if err := s.db.WithContext(ctx).Where("archive_id = ? AND sequence = ?", id, index).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return handmsg.Message{}, false, nil
			}
			return handmsg.Message{}, false, err
		}

		return archivedMessageModels([]archivedMessageModel{record}).messages()[0], true, nil
	}

	var record messageModel
	if err := s.db.WithContext(ctx).Where("session_id = ? AND sequence = ?", id, index).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return handmsg.Message{}, false, nil
		}
		return handmsg.Message{}, false, err
	}

	return messageModels([]messageModel{record}).messages()[0], true, nil
}

// SaveSummary stores or replaces a session summary.
func (s *SessionStore) SaveSummary(ctx context.Context, summary SessionSummary) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	normalized, err := common.NormalizeSessionSummary(summary)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sessionModel

		if err := tx.First(&session, "id = ?", normalized.SessionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		record := summaryModel{
			SessionID:          normalized.SessionID,
			SourceEndOffset:    normalized.SourceEndOffset,
			SourceMessageCount: normalized.SourceMessageCount,
			UpdatedAt:          normalized.UpdatedAt,
			SessionSummary:     normalized.SessionSummary,
			CurrentTask:        normalized.CurrentTask,
			Discoveries:        stringsToJSON(normalized.Discoveries),
			OpenQuestions:      stringsToJSON(normalized.OpenQuestions),
			NextActions:        stringsToJSON(normalized.NextActions),
		}

		return tx.Save(&record).Error
	})
}

// GetSummary loads the current summary for a session.
func (s *SessionStore) GetSummary(ctx context.Context, sessionID string) (SessionSummary, bool, error) {
	if s == nil || s.db == nil {
		return SessionSummary{}, false, errors.New("session store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionSummary{}, false, nil
	}

	if err := common.ValidateSessionID(sessionID); err != nil {
		return SessionSummary{}, false, err
	}

	var record summaryModel
	if err := s.db.WithContext(ctx).First(&record, "session_id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SessionSummary{}, false, nil
		}
		return SessionSummary{}, false, err
	}

	summary, err := summaryModelToSessionSummary(record)
	if err != nil {
		return SessionSummary{}, false, err
	}

	return summary, true, nil
}

// DeleteSummary removes a session summary if it exists.
func (s *SessionStore) DeleteSummary(ctx context.Context, sessionID string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if err := common.ValidateSessionID(sessionID); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&summaryModel{}).Error
}

// CreateArchive moves active session messages into an archive and clears related indexes.
func (s *SessionStore) CreateArchive(ctx context.Context, archive ArchivedSession) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	archive, err := common.NormalizeCreateArchive(archive)
	if err != nil {
		return err
	}

	var sourceIDs []string
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source []messageModel
		if err := tx.Where("session_id = ?", archive.SourceSessionID).Order("sequence asc").
			Find(&source).Error; err != nil {
			return err
		}

		if len(source) == 0 {
			return errors.New("source session has no messages")
		}
		sourceIDs = messageModels(source).sourceIDs()

		record := archiveModel{
			ID:              archive.ID,
			SourceSessionID: archive.SourceSessionID,
			ArchivedAt:      archive.ArchivedAt,
			ExpiresAt:       archive.ExpiresAt,
		}

		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		if err := tx.Where("archive_id = ?", archive.ID).Delete(&archivedMessageModel{}).Error; err != nil {
			return err
		}

		records := messagesToArchivedMessageModels(archive.ID, messageModels(source).messages())
		if err := tx.Create(&records).Error; err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSearchRows(tx, archive.SourceSessionID); err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&summaryModel{}).Error; err != nil {
			return err
		}

		if archive.SourceSessionID == base.DefaultSessionID {
			return tx.Model(&sessionModel{}).
				Where("id = ?", archive.SourceSessionID).
				Updates(map[string]any{
					"compaction_completed_at":         time.Time{},
					"compaction_failed_at":            time.Time{},
					"compaction_last_error":           "",
					"compaction_requested_at":         time.Time{},
					"compaction_started_at":           time.Time{},
					"compaction_status":               "",
					"compaction_target_message_count": 0,
					"compaction_target_offset":        0,
				}).Error
		}

		if err := tx.Where("id = ?", archive.SourceSessionID).Delete(&sessionModel{}).Error; err != nil {
			return err
		}

		return tx.Where("key = ? AND value = ?", currentSessionStateKey, archive.SourceSessionID).
			Delete(&stateModel{}).Error
	}); err != nil {
		return err
	}

	if err := s.deleteVectorRows(ctx, sourceIDs); err != nil {
		return s.handleVectorStoreError(err)
	}

	return nil
}

// GetArchive loads archive metadata by archive ID.
func (s *SessionStore) GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error) {
	if s == nil || s.db == nil {
		return ArchivedSession{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ArchivedSession{}, false, nil
	}

	if err := common.ValidateArchiveID(id); err != nil {
		return ArchivedSession{}, false, err
	}

	var record archiveModel
	if err := s.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ArchivedSession{}, false, nil
		}
		return ArchivedSession{}, false, err
	}

	archive, err := archiveModelToArchivedSession(record)
	if err != nil {
		return ArchivedSession{}, false, err
	}

	return archive, true, nil
}

// ClearMessages removes all messages for an active session or archive.
func (s *SessionStore) ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if !opts.Archived {
		if err := common.ValidateSessionID(id); err != nil {
			return err
		}
	} else if err := common.ValidateArchiveID(id); err != nil {
		return err
	}

	var sourceIDs []string
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if opts.Archived {
			var archive archiveModel
			if err := tx.First(&archive, "id = ?", id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("archive not found")
				}
				return err
			}

			return tx.Where("archive_id = ?", id).Delete(&archivedMessageModel{}).Error
		}

		var session sessionModel
		if err := tx.First(&session, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		var messageIDs []uint
		if err := tx.Model(&messageModel{}).Where("session_id = ?", id).Pluck("id", &messageIDs).Error; err != nil {
			return err
		}
		sourceIDs = sourceIDsFromMessageIDs(id, messageIDs)

		if err := tx.Where("session_id = ?", id).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSearchRows(tx, id); err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", id).Delete(&summaryModel{}).Error; err != nil {
			return err
		}

		session.CompactionCompletedAt = time.Time{}
		session.CompactionFailedAt = time.Time{}
		session.CompactionLastError = ""
		session.CompactionRequestedAt = time.Time{}
		session.CompactionStartedAt = time.Time{}
		session.CompactionStatus = ""
		session.CompactionTargetMessageCount = 0
		session.CompactionTargetOffset = 0
		session.UpdatedAt = time.Now().UTC()

		return tx.Save(&session).Error
	}); err != nil {
		return err
	}

	if err := s.deleteVectorRows(ctx, sourceIDs); err != nil {
		return s.handleVectorStoreError(err)
	}

	return nil
}

// ListArchives returns archives, optionally scoped to one source session.
func (s *SessionStore) ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	query := s.db.WithContext(ctx).Order("archived_at desc").Order("id asc")
	sourceSessionID = strings.TrimSpace(sourceSessionID)

	if sourceSessionID != "" {
		query = query.Where("source_session_id = ?", sourceSessionID)
	}

	var records []archiveModel
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	archives := make([]ArchivedSession, 0, len(records))
	for _, record := range records {
		archive, err := archiveModelToArchivedSession(record)
		if err != nil {
			return nil, err
		}
		archives = append(archives, archive)
	}

	return archives, nil
}

// DeleteArchive removes one archive and its archived messages.
func (s *SessionStore) DeleteArchive(ctx context.Context, archiveID string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	archiveID = strings.TrimSpace(archiveID)
	if err := common.ValidateArchiveID(archiveID); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var archive archiveModel
		if err := tx.First(&archive, "id = ?", archiveID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("archive not found")
			}
			return err
		}

		if err := tx.Where("archive_id = ?", archiveID).Delete(&archivedMessageModel{}).Error; err != nil {
			return err
		}

		return tx.Delete(&archive).Error
	})
}

// DeleteExpiredArchives removes archives whose expiration time is at or before now.
func (s *SessionStore) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(&archiveModel{}).Where("expires_at <= ?", now.UTC()).
			Pluck("id", &ids).Error; err != nil {
			return err
		}

		if len(ids) == 0 {
			return nil
		}

		if err := tx.Where("archive_id IN ?", ids).Delete(&archivedMessageModel{}).Error; err != nil {
			return err
		}

		return tx.Where("id IN ?", ids).Delete(&archiveModel{}).Error
	})
}

// SetCurrent marks an existing session as the current session.
func (s *SessionStore) SetCurrent(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	_, ok, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("session not found")
	}

	record := stateModel{
		Key:       currentSessionStateKey,
		Value:     id,
		UpdatedAt: time.Now().UTC(),
	}

	return s.db.WithContext(ctx).Save(&record).Error
}

// Current returns the current session ID if one is set.
func (s *SessionStore) Current(ctx context.Context) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("session store is required")
	}

	var record stateModel
	if err := s.db.WithContext(ctx).First(&record, "key = ?", currentSessionStateKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}

	value := strings.TrimSpace(record.Value)
	if value == "" {
		return "", false, nil
	}

	return value, true, nil
}

func summaryModelToSessionSummary(record summaryModel) (SessionSummary, error) {
	return common.NormalizeSessionSummary(SessionSummary{
		SessionID:          record.SessionID,
		SourceEndOffset:    record.SourceEndOffset,
		SourceMessageCount: record.SourceMessageCount,
		UpdatedAt:          record.UpdatedAt,
		SessionSummary:     record.SessionSummary,
		CurrentTask:        record.CurrentTask,
		Discoveries:        jsonToStrings(record.Discoveries),
		OpenQuestions:      jsonToStrings(record.OpenQuestions),
		NextActions:        jsonToStrings(record.NextActions),
	})
}

func archiveModelToArchivedSession(record archiveModel) (ArchivedSession, error) {
	return common.NormalizeCreateArchive(ArchivedSession{
		ID:              record.ID,
		SourceSessionID: record.SourceSessionID,
		ArchivedAt:      record.ArchivedAt,
		ExpiresAt:       record.ExpiresAt,
	})
}

func sessionModelToSession(record sessionModel) (Session, error) {
	id := strings.TrimSpace(record.ID)
	if id == "" {
		return Session{}, errors.New("session id is required")
	}

	session := Session{
		CreatedAt: record.CreatedAt,
		Compaction: base.SessionCompaction{
			CompletedAt:        record.CompactionCompletedAt,
			FailedAt:           record.CompactionFailedAt,
			LastError:          record.CompactionLastError,
			RequestedAt:        record.CompactionRequestedAt,
			StartedAt:          record.CompactionStartedAt,
			Status:             base.SessionCompactionStatus(record.CompactionStatus),
			TargetMessageCount: record.CompactionTargetMessageCount,
			TargetOffset:       record.CompactionTargetOffset,
		},
		ID:               id,
		LastPromptTokens: record.LastPromptTokens,
	}

	if !record.CreatedAt.IsZero() {
		session.CreatedAt = record.CreatedAt.UTC()
	}

	if !record.UpdatedAt.IsZero() {
		session.UpdatedAt = record.UpdatedAt.UTC()
	}

	return session, nil
}

func messagesToMessageModels(sessionID string, messages []handmsg.Message) []messageModel {
	return messagesToMessageModelsWithOffset(sessionID, messages, 0)
}

func messagesToMessageModelsWithOffset(sessionID string, messages []handmsg.Message, offset int) []messageModel {
	if len(messages) == 0 {
		return nil
	}

	records := make([]messageModel, 0, len(messages))
	for i, message := range messages {
		records = append(records, messageModel{
			ID:         message.ID,
			SessionID:  sessionID,
			Sequence:   offset + i,
			Role:       string(message.Role),
			Name:       message.Name,
			Content:    message.Content,
			ToolCalls:  toolCallsToJSON(message.ToolCalls),
			ToolCallID: message.ToolCallID,
			CreatedAt:  message.CreatedAt,
		})
	}

	return records
}

func messagesToArchivedMessageModels(archiveID string, messages []handmsg.Message) []archivedMessageModel {
	if len(messages) == 0 {
		return nil
	}

	records := make([]archivedMessageModel, 0, len(messages))
	for i, message := range messages {
		records = append(records, archivedMessageModel{
			ID:         message.ID,
			ArchiveID:  archiveID,
			Sequence:   i,
			Role:       string(message.Role),
			Name:       message.Name,
			Content:    message.Content,
			ToolCalls:  toolCallsToJSON(message.ToolCalls),
			ToolCallID: message.ToolCallID,
			CreatedAt:  message.CreatedAt,
		})
	}

	return records
}

// messages converts active message models to domain messages.
func (records messageModels) messages() []handmsg.Message {
	if len(records) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, handmsg.Message{
			ID:         record.ID,
			Role:       handmsg.Role(record.Role),
			Name:       record.Name,
			Content:    record.Content,
			ToolCalls:  jsonToToolCalls(record.ToolCalls),
			ToolCallID: record.ToolCallID,
			CreatedAt:  record.CreatedAt,
		})
	}

	return messages
}

// records converts active message models to message records with sequence offsets.
func (records messageModels) records() []base.MessageRecord {
	if len(records) == 0 {
		return nil
	}

	messages := records.messages()
	messageRecords := make([]base.MessageRecord, 0, len(records))
	for idx, record := range records {
		messageRecords = append(messageRecords, base.MessageRecord{
			Offset:  record.Sequence,
			Message: messages[idx],
		})
	}

	return messageRecords
}

// messages converts archived message models to domain messages.
func (records archivedMessageModels) messages() []handmsg.Message {
	if len(records) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, handmsg.Message{
			ID:         record.ID,
			Role:       handmsg.Role(record.Role),
			Name:       record.Name,
			Content:    record.Content,
			ToolCalls:  jsonToToolCalls(record.ToolCalls),
			ToolCallID: record.ToolCallID,
			CreatedAt:  record.CreatedAt,
		})
	}

	return messages
}

func searchMessageResultRowsToResults(records []searchSessionResultRow) []base.SearchMessageResult {
	if len(records) == 0 {
		return nil
	}

	results := make([]base.SearchMessageResult, 0)
	indexBySessionID := make(map[string]int, len(records))
	for _, record := range records {
		index, ok := indexBySessionID[record.SessionID]
		if !ok {
			results = append(results, base.SearchMessageResult{
				SessionID:     record.SessionID,
				LastMatchedAt: searchSessionResultTime(record.LastMatchedAt),
				MatchCount:    record.MatchCount,
				Messages:      make([]base.SearchMessageHit, 0),
			})
			index = len(results) - 1
			indexBySessionID[record.SessionID] = index
		}

		results[index].Messages = append(results[index].Messages, base.SearchMessageHit{
			SessionID: record.SessionID,
			Message: handmsg.Message{
				ID:         record.ID,
				Role:       handmsg.Role(record.Role),
				Name:       record.Name,
				Content:    record.Content,
				ToolCalls:  jsonToToolCalls(record.ToolCalls),
				ToolCallID: record.ToolCallID,
				CreatedAt:  record.CreatedAt,
			},
			MatchedText:     strings.TrimSpace(record.MatchedText),
			MatchedToolName: strings.TrimSpace(record.MatchedToolName),
		})
	}

	return results
}

func searchSessionResultTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}

	return parsed.UTC()
}

func toolCallsToJSON(toolCalls []handmsg.ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	raw, _ := json.Marshal(toolCalls)

	return string(raw)
}

func stringsToJSON(values []string) string {
	if len(values) == 0 {
		return ""
	}

	raw, _ := json.Marshal(values)

	return string(raw)
}

func jsonToStrings(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil
	}

	return values
}

func jsonToToolCalls(value string) []handmsg.ToolCall {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var toolCalls []handmsg.ToolCall
	if err := json.Unmarshal([]byte(value), &toolCalls); err != nil {
		return nil
	}

	return toolCalls
}

// searchRow is one FTS row and the matching unit used for vector indexing.
type searchRow struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	MessageID uint
	SessionID string
	Role      string
	ToolName  string
	Body      string
}

// searchRows is a typed slice for FTS/vector searchable message rows.
type searchRows []searchRow

func ensureSearchIndex(db *gorm.DB) error {
	if db == nil {
		return errors.New("session db is required")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS ` + sessionMessageSearchTable + ` USING fts5(
	message_id UNINDEXED,
	session_id UNINDEXED,
	role UNINDEXED,
	tool_name UNINDEXED,
	body,
	tokenize='unicode61'
)`).Error; err != nil {
			return fmt.Errorf("failed to create session message search index: %w", err)
		}

		return nil
	})
}

// insert writes search rows to the FTS table.
func (rows searchRows) insert(tx *gorm.DB) error {
	if tx == nil || len(rows) == 0 {
		return nil
	}

	for _, row := range rows {
		if err := tx.Exec(
			`INSERT INTO `+sessionMessageSearchTable+` (message_id, session_id, role, tool_name, body) VALUES (?, ?, ?, ?, ?)`,
			row.MessageID,
			row.SessionID,
			row.Role,
			row.ToolName,
			row.Body,
		).Error; err != nil {
			return fmt.Errorf("failed to insert session message search row: %w", err)
		}
	}

	return nil
}

func deleteSearchRows(tx *gorm.DB, sessionID string) error {
	if tx == nil {
		return nil
	}

	if err := tx.Exec(`DELETE FROM `+sessionMessageSearchTable+` WHERE session_id = ?`, sessionID).Error; err != nil {
		return fmt.Errorf("failed to delete session message search rows: %w", err)
	}

	return nil
}

// searchRows derives FTS/vector searchable rows from active message models.
func (records messageModels) searchRows() searchRows {
	if len(records) == 0 {
		return nil
	}

	rows := make(searchRows, 0, len(records))
	for _, record := range records {
		rows = append(rows, searchRowsFromMessageModel(record)...)
	}

	return rows
}

func searchRowsFromMessageModel(record messageModel) searchRows {
	baseRow := searchRow{
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
		MessageID: record.ID,
		SessionID: strings.TrimSpace(record.SessionID),
		Role:      normalizeSearchValue(record.Role),
	}

	body := strings.TrimSpace(record.Content)
	role := handmsg.Role(strings.TrimSpace(record.Role))

	switch role {
	case handmsg.RoleAssistant:
		toolCalls := jsonToToolCalls(record.ToolCalls)
		if len(toolCalls) == 0 {
			if body == "" {
				return nil
			}

			row := baseRow
			row.Body = body
			return searchRows{row}
		}

		rows := make(searchRows, 0, len(toolCalls)+1)
		if body != "" {
			row := baseRow
			row.Body = body
			rows = append(rows, row)
		}

		for _, toolCall := range toolCalls {
			toolBody := strings.TrimSpace(handmsg.ToolCallSearchText(toolCall))
			if toolBody == "" {
				continue
			}

			row := baseRow
			row.ToolName = normalizeSearchValue(toolCall.Name)
			row.Body = toolBody
			rows = append(rows, row)
		}

		return rows
	case handmsg.RoleTool:
		if body == "" {
			return nil
		}

		row := baseRow
		row.ToolName = normalizeSearchValue(record.Name)
		row.Body = body
		return searchRows{row}
	default:
		if body == "" {
			return nil
		}

		row := baseRow
		row.Body = body
		return searchRows{row}
	}
}

// indexVectors embeds searchable message rows and upserts the resulting vector records.
func (s *SessionStore) indexVectors(ctx context.Context, records []messageModel) error {
	if s == nil || s.vectors == nil || len(records) == 0 {
		return nil
	}

	inputs := messageModels(records).searchRows().vectorInputs()
	if len(inputs) == 0 {
		return nil
	}

	embeddingInputs := make([]retrieval.EmbeddingInput, 0, len(inputs))
	for _, input := range inputs {
		embeddingInputs = append(embeddingInputs, retrieval.EmbeddingInput{
			ID:         input.ID,
			Text:       input.Text,
			SourceKind: retrieval.SourceKindSessionMessage,
		})
	}

	req := retrieval.EmbeddingRequest{
		Model:  s.vectors.model,
		Inputs: embeddingInputs,
	}
	s.logVectorEvent("embedding started").
		Int("input_count", len(req.Inputs)).
		Str("embedding_model", strings.TrimSpace(req.Model)).
		Msg("session vector embedding started")
	result, err := s.vectors.provider.Embed(ctx, req)
	if err != nil {
		s.logVectorEvent("embedding failed").Err(err).Msg("session vector embedding failed")
		return err
	}
	if err := retrieval.ValidateEmbeddingResult(req, result); err != nil {
		s.logVectorEvent("embedding validation failed").Err(err).Msg("session vector embedding validation failed")
		return err
	}
	s.logVectorEvent("embedding completed").
		Int("input_count", len(req.Inputs)).
		Int("dimensions", result.Dimensions).
		Str("embedding_model", strings.TrimSpace(result.Model)).
		Msg("session vector embedding completed")

	inputByID := make(map[string]vectorInput, len(inputs))
	for _, input := range inputs {
		inputByID[input.ID] = input
	}

	recordsToUpsert := make([]retrieval.VectorRecord, 0, len(result.Items))
	for _, item := range result.Items {
		input := inputByID[item.ID]
		recordsToUpsert = append(recordsToUpsert, retrieval.VectorRecord{
			CreatedAt:      input.CreatedAt,
			UpdatedAt:      input.UpdatedAt,
			ID:             item.ID,
			SourceKind:     retrieval.SourceKindSessionMessage,
			SourceID:       input.SourceID,
			SessionID:      input.SessionID,
			Role:           input.Role,
			ToolName:       input.ToolName,
			EmbeddingModel: result.Model,
			ContentHash:    item.ContentHash,
			Vector:         item.Vector,
			Dimensions:     result.Dimensions,
		})
	}

	if err := s.vectors.store.Upsert(ctx, recordsToUpsert); err != nil {
		s.logVectorEvent("upsert failed").Err(err).Int("record_count", len(recordsToUpsert)).Msg("session vector upsert failed")
		return err
	}
	s.logVectorEvent("upsert completed").Int("record_count", len(recordsToUpsert)).Msg("session vector upsert completed")

	return nil
}

// deleteVectorRows removes vector records for one or more session-message source IDs.
func (s *SessionStore) deleteVectorRows(ctx context.Context, sourceIDs []string) error {
	if s == nil || s.vectors == nil || len(sourceIDs) == 0 {
		return nil
	}

	sourceIDs = uniqueStrings(sourceIDs)
	if len(sourceIDs) == 0 {
		return nil
	}
	if err := s.vectors.store.Delete(ctx, retrieval.VectorDeleteRequest{
		SourceKind: retrieval.SourceKindSessionMessage,
		SourceIDs:  sourceIDs,
	}); err != nil {
		return err
	}

	return nil
}

// handleVectorStoreError applies best-effort versus required vector indexing semantics.
func (s *SessionStore) handleVectorStoreError(err error) error {
	if err == nil || s == nil || s.vectors == nil || !s.vectors.required {
		return nil
	}

	return err
}

// vectorInputs maps search rows to stable embedding inputs.
func (rows searchRows) vectorInputs() []vectorInput {
	if len(rows) == 0 {
		return nil
	}

	countsByMessageID := make(map[uint]int, len(rows))
	inputs := make([]vectorInput, 0, len(rows))
	for _, row := range rows {
		sourceID := sourceIDForMessage(row.SessionID, row.MessageID)
		countsByMessageID[row.MessageID]++
		inputs = append(inputs, vectorInput{
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			ID:        fmt.Sprintf("%s:row:%d", sourceID, countsByMessageID[row.MessageID]),
			SourceID:  sourceID,
			SessionID: row.SessionID,
			Role:      row.Role,
			ToolName:  row.ToolName,
			Text:      row.Body,
		})
	}

	return inputs
}

// sourceIDs returns stable vector source IDs for active message models.
func (records messageModels) sourceIDs() []string {
	if len(records) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(records))
	for _, record := range records {
		sourceIDs = append(sourceIDs, sourceIDForMessage(record.SessionID, record.ID))
	}

	return sourceIDs
}

func sourceIDsFromMessageIDs(sessionID string, messageIDs []uint) []string {
	if len(messageIDs) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		sourceIDs = append(sourceIDs, sourceIDForMessage(sessionID, messageID))
	}

	return sourceIDs
}

func sourceIDForMessage(sessionID string, messageID uint) string {
	return retrieval.StableSessionMessageID(strings.TrimSpace(sessionID), messageID)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	return unique
}

func normalizeSearchValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func buildSearchQuery(query string) string {
	tokens := searchTokens(query)
	if len(tokens) == 0 {
		return ""
	}

	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted = append(quoted, `"`+token+`"`)
	}

	return strings.Join(quoted, " AND ")
}

func searchTokens(query string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(fields) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		tokens = append(tokens, normalizeSearchValue(field))
	}

	return tokens
}

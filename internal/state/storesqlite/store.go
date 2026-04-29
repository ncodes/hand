package storesqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	handmsg "github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/indexing"
)

const currentSessionStateKey = "current_session"
const sessionMessageSearchTable = "session_message_search"

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

// Store persists sessions, messages, search rows, summaries, and optional vector indexing in SQLite.
type Store struct {
	vectors *vectorConfig
	db      *gorm.DB
}

func NewStore(path string) (*Store, error) {
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

func NewStoreFromDB(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("session db is required")
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})

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

	return &Store{db: db}, nil
}

func gormOpenSQLite(path string) (*Store, error) {
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

	return NewStoreFromDB(db)
}

// Save upserts session metadata without modifying stored messages.
func (s *Store) Save(ctx context.Context, session Session) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	session.ID = strings.TrimSpace(session.ID)
	if err := base.ValidateSessionID(session.ID); err != nil {
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
func (s *Store) Get(ctx context.Context, id string) (Session, bool, error) {
	if s == nil || s.db == nil {
		return Session{}, false, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, false, nil
	}

	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) List(ctx context.Context) ([]Session, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
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
func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) GetMessages(
	ctx context.Context,
	id string,
	opts MessageQueryOptions,
) ([]handmsg.Message, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return nil, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}

	if !opts.Archived {
		if err := base.ValidateSessionID(id); err != nil {
			return nil, err
		}
	} else if err := base.ValidateArchiveID(id); err != nil {
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
func (s *Store) GetMessagesByIDs(
	ctx context.Context,
	id string,
	messageIDs []uint,
) ([]base.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || len(messageIDs) == 0 {
		return nil, nil
	}
	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) GetMessageWindow(
	ctx context.Context,
	id string,
	anchorMessageID uint,
	before int,
	after int,
) ([]base.MessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || anchorMessageID == 0 {
		return nil, nil
	}
	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) CountMessages(
	ctx context.Context,
	id string,
	opts MessageQueryOptions,
) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return 0, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil
	}

	if !opts.Archived {
		if err := base.ValidateSessionID(id); err != nil {
			return 0, err
		}
	} else if err := base.ValidateArchiveID(id); err != nil {
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
func (s *Store) SearchMessages(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
) ([]base.SearchMessageResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id != "" {
		if err := base.ValidateSessionID(id); err != nil {
			return nil, err
		}
	} else if opts.IgnoreSessionID = strings.TrimSpace(opts.IgnoreSessionID); opts.IgnoreSessionID != "" {
		if err := base.ValidateSessionID(opts.IgnoreSessionID); err != nil {
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

	s.logSearchEvent("lexical search started", id, opts).Msg("session search lexical search started")
	records, err := s.searchMessagesLexical(ctx, id, opts, queryText, 0, true)
	if err != nil {
		logSafeError(s.logSearchEvent("lexical search failed", id, opts), err).
			Msg("session search lexical search failed")
		return nil, err
	}

	results := searchMessageResultRowsToResults(records)
	s.logSearchEvent("lexical search completed", id, opts).
		Int("session_count", len(results)).
		Int("message_count", len(records)).
		Msg("session search lexical search completed")

	return results, nil
}

// searchMessagesLexical returns BM25-ranked message rows with optional early limiting.
func (s *Store) searchMessagesLexical(
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
func (s *Store) GetMessage(
	ctx context.Context,
	id string,
	index int,
	opts MessageQueryOptions,
) (handmsg.Message, bool, error) {
	if s == nil || s.db == nil {
		return handmsg.Message{}, false, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || index < 0 {
		return handmsg.Message{}, false, nil
	}

	if !opts.Archived {
		if err := base.ValidateSessionID(id); err != nil {
			return handmsg.Message{}, false, err
		}
	} else if err := base.ValidateArchiveID(id); err != nil {
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
func (s *Store) SaveSummary(ctx context.Context, summary SessionSummary) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	normalized, err := base.NormalizeSessionSummary(summary)
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
func (s *Store) GetSummary(ctx context.Context, sessionID string) (SessionSummary, bool, error) {
	if s == nil || s.db == nil {
		return SessionSummary{}, false, errors.New("store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionSummary{}, false, nil
	}

	if err := base.ValidateSessionID(sessionID); err != nil {
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
func (s *Store) DeleteSummary(ctx context.Context, sessionID string) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if err := base.ValidateSessionID(sessionID); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&summaryModel{}).Error
}

// CreateArchive moves active session messages into an archive and clears related indexes.
func (s *Store) CreateArchive(ctx context.Context, archive ArchivedSession) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	archive, err := base.NormalizeCreateArchive(archive)
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
func (s *Store) GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error) {
	if s == nil || s.db == nil {
		return ArchivedSession{}, false, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ArchivedSession{}, false, nil
	}

	if err := base.ValidateArchiveID(id); err != nil {
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
func (s *Store) ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if !opts.Archived {
		if err := base.ValidateSessionID(id); err != nil {
			return err
		}
	} else if err := base.ValidateArchiveID(id); err != nil {
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
func (s *Store) ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is required")
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
func (s *Store) DeleteArchive(ctx context.Context, archiveID string) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	archiveID = strings.TrimSpace(archiveID)
	if err := base.ValidateArchiveID(archiveID); err != nil {
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
func (s *Store) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
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
func (s *Store) SetCurrent(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
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
func (s *Store) Current(ctx context.Context) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("store is required")
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
	return base.NormalizeSessionSummary(SessionSummary{
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
	return base.NormalizeCreateArchive(ArchivedSession{
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
type searchRow = indexing.MessageIndexRow

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
	rows := indexing.MessageIndexRowsFromMessage(record.SessionID, handmsg.Message{
		ID:         record.ID,
		Role:       handmsg.Role(strings.TrimSpace(record.Role)),
		Content:    record.Content,
		Name:       record.Name,
		ToolCallID: record.ToolCallID,
		ToolCalls:  jsonToToolCalls(record.ToolCalls),
		CreatedAt:  record.CreatedAt,
	})
	for idx := range rows {
		rows[idx].UpdatedAt = record.UpdatedAt
	}

	return searchRows(rows)
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

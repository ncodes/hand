package sqlite

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
	base "github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
)

const currentSessionStateKey = "current_session"
const sessionMessageSearchTable = "session_message_search"

type Session = base.Session
type ArchivedSession = base.ArchivedSession
type MessageQueryOptions = base.MessageQueryOptions
type SessionSummary = base.SessionSummary
type SearchMessageOptions = base.SearchMessageOptions
type SessionCompaction = base.SessionCompaction
type SessionCompactionStatus = base.SessionCompactionStatus

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

func (sessionModel) TableName() string {
	return "sessions"
}

type archiveModel struct {
	ID              string    `gorm:"primaryKey"`
	SourceSessionID string    `gorm:"index;not null"`
	ArchivedAt      time.Time `gorm:"index"`
	ExpiresAt       time.Time `gorm:"index"`
	CreatedAt       time.Time
}

func (archiveModel) TableName() string {
	return "session_archives"
}

type stateModel struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
	CreatedAt time.Time
}

func (stateModel) TableName() string {
	return "session_state"
}

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

func (summaryModel) TableName() string {
	return "session_summaries"
}

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

func (messageModel) TableName() string {
	return "session_messages"
}

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

func (archivedMessageModel) TableName() string {
	return "archived_session_messages"
}

type searchMessageHitModel struct {
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
}

type SessionStore struct {
	db *gorm.DB
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

	if err := ensureSessionMessageSearchIndex(db); err != nil {
		return nil, err
	}

	return &SessionStore{db: db}, nil
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

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sessionModel

		if err := tx.First(&session, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		if err := tx.Where("session_id = ?", id).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSessionMessageSearchRows(tx, id); err != nil {
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
	})
}

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

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		records := messagesToMessageModelsWithOffset(id, messages, int(nextSequence))
		if len(records) > 0 {
			if err := tx.Create(&records).Error; err != nil {
				return err
			}
			if err := insertToSearchRows(tx, messageModelsToSearchRows(records)); err != nil {
				return err
			}
		}

		record.UpdatedAt = time.Now().UTC()

		return tx.Save(&record).Error
	})
}

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

		return archivedMessageModelsToMessages(records), nil
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

	return messageModelsToMessages(records), nil
}

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

func (s *SessionStore) SearchMessages(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
) ([]base.SearchMessageHit, error) {
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

	queryText := buildSessionMessageSearchQuery(opts.Query)
	if queryText == "" {
		return nil, nil
	}

	args := []any{queryText}
	var sql strings.Builder
	sql.WriteString(`WITH ranked_hits AS (
	SELECT
		CAST(message_id AS INTEGER) AS message_id,
		body AS matched_text,
		tool_name AS matched_tool_name,
		ROW_NUMBER() OVER (
			PARTITION BY CAST(message_id AS INTEGER)
			ORDER BY CASE WHEN tool_name <> '' THEN 0 ELSE 1 END, rowid ASC
		) AS hit_rank
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
	if toolName := normalizeSessionMessageSearchValue(opts.ToolName); toolName != "" {
		sql.WriteString(` AND tool_name = ?`)
		args = append(args, toolName)
	}
	sql.WriteString(`
)
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
	hits.matched_tool_name
FROM session_messages AS m
JOIN ranked_hits AS hits ON hits.message_id = m.id AND hits.hit_rank = 1`)
	if id != "" {
		sql.WriteString(` ORDER BY m.sequence DESC`)
	} else {
		sql.WriteString(` ORDER BY m.created_at DESC, m.id DESC`)
	}

	offset := max(opts.Offset, 0)
	if opts.Limit > 0 {
		sql.WriteString(` LIMIT ? OFFSET ?`)
		args = append(args, opts.Limit, offset)
	} else if offset > 0 {
		sql.WriteString(` LIMIT -1 OFFSET ?`)
		args = append(args, offset)
	}

	var records []searchMessageHitModel
	if err := s.db.WithContext(ctx).Raw(sql.String(), args...).Scan(&records).Error; err != nil {
		return nil, err
	}

	return searchMessageHitModelsToHits(records), nil
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

		return archivedMessageModelsToMessages([]archivedMessageModel{record})[0], true, nil
	}

	var record messageModel
	if err := s.db.WithContext(ctx).Where("session_id = ? AND sequence = ?", id, index).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return handmsg.Message{}, false, nil
		}
		return handmsg.Message{}, false, err
	}

	return messageModelsToMessages([]messageModel{record})[0], true, nil
}

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

func (s *SessionStore) CreateArchive(ctx context.Context, archive ArchivedSession) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	archive, err := common.NormalizeCreateArchive(archive)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source []messageModel
		if err := tx.Where("session_id = ?", archive.SourceSessionID).Order("sequence asc").
			Find(&source).Error; err != nil {
			return err
		}

		if len(source) == 0 {
			return errors.New("source session has no messages")
		}

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

		records := messagesToArchivedMessageModels(archive.ID, messageModelsToMessages(source))
		if err := tx.Create(&records).Error; err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSessionMessageSearchRows(tx, archive.SourceSessionID); err != nil {
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
	})
}

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

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		if err := tx.Where("session_id = ?", id).Delete(&messageModel{}).Error; err != nil {
			return err
		}
		if err := deleteSessionMessageSearchRows(tx, id); err != nil {
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
	})
}

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

func messageModelsToMessages(records []messageModel) []handmsg.Message {
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

func archivedMessageModelsToMessages(records []archivedMessageModel) []handmsg.Message {
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

func searchMessageHitModelsToHits(records []searchMessageHitModel) []base.SearchMessageHit {
	if len(records) == 0 {
		return nil
	}

	hits := make([]base.SearchMessageHit, 0, len(records))
	for _, record := range records {
		hits = append(hits, base.SearchMessageHit{
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

	return hits
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

type sessionMessageSearchRow struct {
	MessageID uint
	SessionID string
	Role      string
	ToolName  string
	Body      string
}

func ensureSessionMessageSearchIndex(db *gorm.DB) error {
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

func insertToSearchRows(tx *gorm.DB, rows []sessionMessageSearchRow) error {
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

func deleteSessionMessageSearchRows(tx *gorm.DB, sessionID string) error {
	if tx == nil {
		return nil
	}

	if err := tx.Exec(`DELETE FROM `+sessionMessageSearchTable+` WHERE session_id = ?`, sessionID).Error; err != nil {
		return fmt.Errorf("failed to delete session message search rows: %w", err)
	}

	return nil
}

func messageModelsToSearchRows(records []messageModel) []sessionMessageSearchRow {
	if len(records) == 0 {
		return nil
	}

	rows := make([]sessionMessageSearchRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, messageModelToSearchRows(record)...)
	}

	return rows
}

func messageModelToSearchRows(record messageModel) []sessionMessageSearchRow {
	baseRow := sessionMessageSearchRow{
		MessageID: record.ID,
		SessionID: strings.TrimSpace(record.SessionID),
		Role:      normalizeSessionMessageSearchValue(record.Role),
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
			return []sessionMessageSearchRow{row}
		}

		rows := make([]sessionMessageSearchRow, 0, len(toolCalls)+1)
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
			row.ToolName = normalizeSessionMessageSearchValue(toolCall.Name)
			row.Body = toolBody
			rows = append(rows, row)
		}

		return rows
	case handmsg.RoleTool:
		if body == "" {
			return nil
		}

		row := baseRow
		row.ToolName = normalizeSessionMessageSearchValue(record.Name)
		row.Body = body
		return []sessionMessageSearchRow{row}
	default:
		if body == "" {
			return nil
		}

		row := baseRow
		row.Body = body
		return []sessionMessageSearchRow{row}
	}
}

func normalizeSessionMessageSearchValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func buildSessionMessageSearchQuery(query string) string {
	tokens := sessionMessageSearchTokens(query)
	if len(tokens) == 0 {
		return ""
	}

	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted = append(quoted, `"`+token+`"`)
	}

	return strings.Join(quoted, " AND ")
}

func sessionMessageSearchTokens(query string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(fields) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		tokens = append(tokens, normalizeSessionMessageSearchValue(field))
	}

	return tokens
}

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

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	handmsg "github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
)

const currentSessionStateKey = "current_session"

type Session = base.Session
type ArchivedSession = base.ArchivedSession
type MessageQueryOptions = base.MessageQueryOptions
type SessionSummary = base.SessionSummary

type sessionModel struct {
	CreatedAt        time.Time
	ID               string `gorm:"primaryKey"`
	LastPromptTokens int
	UpdatedAt        time.Time
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
		CreatedAt:        session.CreatedAt,
		ID:               session.ID,
		LastPromptTokens: session.LastPromptTokens,
		UpdatedAt:        session.UpdatedAt,
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		return nil
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

	session := Session{
		CreatedAt:        record.CreatedAt,
		ID:               record.ID,
		LastPromptTokens: record.LastPromptTokens,
	}
	if !record.CreatedAt.IsZero() {
		session.CreatedAt = record.CreatedAt.UTC()
	}
	if !record.UpdatedAt.IsZero() {
		session.UpdatedAt = record.UpdatedAt.UTC()
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
		session, err := decodeSessionRecord(record)
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

		records := encodeSessionMessagesWithOffset(id, messages, int(nextSequence))
		if len(records) > 0 {
			if err := tx.Create(&records).Error; err != nil {
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

	if opts.Archived {
		var records []archivedMessageModel
		if err := s.db.WithContext(ctx).Where("archive_id = ?", id).Order("sequence asc").
			Find(&records).Error; err != nil {
			return nil, err
		}

		return decodeArchivedMessages(records), nil
	}

	var records []messageModel
	if err := s.db.WithContext(ctx).Where("session_id = ?", id).Order("sequence asc").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return decodeSessionMessages(records), nil
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

		return decodeArchivedMessages([]archivedMessageModel{record})[0], true, nil
	}

	var record messageModel
	if err := s.db.WithContext(ctx).Where("session_id = ? AND sequence = ?", id, index).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return handmsg.Message{}, false, nil
		}

		return handmsg.Message{}, false, err
	}

	return decodeSessionMessages([]messageModel{record})[0], true, nil
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
			Discoveries:        encodeStrings(normalized.Discoveries),
			OpenQuestions:      encodeStrings(normalized.OpenQuestions),
			NextActions:        encodeStrings(normalized.NextActions),
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

	summary, err := decodeSummaryRecord(record)
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

		records := encodeArchivedMessages(archive.ID, decodeSessionMessages(source))
		if err := tx.Create(&records).Error; err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&messageModel{}).Error; err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&summaryModel{}).Error; err != nil {
			return err
		}

		if archive.SourceSessionID == base.DefaultSessionID {
			return nil
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

	archive, err := decodeArchiveRecord(record)
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

		if err := tx.Where("session_id = ?", id).Delete(&summaryModel{}).Error; err != nil {
			return err
		}

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
		archive, err := decodeArchiveRecord(record)
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

func decodeSessionRecord(record sessionModel) (Session, error) {
	return sessionFromRecord(record.CreatedAt, record.ID, record.LastPromptTokens, record.UpdatedAt)
}

func decodeSummaryRecord(record summaryModel) (SessionSummary, error) {
	return common.NormalizeSessionSummary(SessionSummary{
		SessionID:          record.SessionID,
		SourceEndOffset:    record.SourceEndOffset,
		SourceMessageCount: record.SourceMessageCount,
		UpdatedAt:          record.UpdatedAt,
		SessionSummary:     record.SessionSummary,
		CurrentTask:        record.CurrentTask,
		Discoveries:        decodeStrings(record.Discoveries),
		OpenQuestions:      decodeStrings(record.OpenQuestions),
		NextActions:        decodeStrings(record.NextActions),
	})
}

func decodeArchiveRecord(record archiveModel) (ArchivedSession, error) {
	return common.NormalizeCreateArchive(ArchivedSession{
		ID:              record.ID,
		SourceSessionID: record.SourceSessionID,
		ArchivedAt:      record.ArchivedAt,
		ExpiresAt:       record.ExpiresAt,
	})
}

func sessionFromRecord(createdAt time.Time, id string, lastPromptTokens int, updatedAt time.Time) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, errors.New("session id is required")
	}

	session := Session{
		CreatedAt:        createdAt,
		ID:               id,
		LastPromptTokens: lastPromptTokens,
	}

	if !createdAt.IsZero() {
		session.CreatedAt = createdAt.UTC()
	}

	if !updatedAt.IsZero() {
		session.UpdatedAt = updatedAt.UTC()
	}

	return session, nil
}

func encodeSessionMessages(sessionID string, messages []handmsg.Message) []messageModel {
	return encodeSessionMessagesWithOffset(sessionID, messages, 0)
}

func encodeSessionMessagesWithOffset(sessionID string, messages []handmsg.Message, offset int) []messageModel {
	if len(messages) == 0 {
		return nil
	}

	records := make([]messageModel, 0, len(messages))
	for i, message := range messages {
		records = append(records, messageModel{
			SessionID:  sessionID,
			Sequence:   offset + i,
			Role:       string(message.Role),
			Name:       message.Name,
			Content:    message.Content,
			ToolCalls:  encodeToolCalls(message.ToolCalls),
			ToolCallID: message.ToolCallID,
			CreatedAt:  message.CreatedAt,
		})
	}

	return records
}

func encodeArchivedMessages(archiveID string, messages []handmsg.Message) []archivedMessageModel {
	if len(messages) == 0 {
		return nil
	}

	records := make([]archivedMessageModel, 0, len(messages))
	for i, message := range messages {
		records = append(records, archivedMessageModel{
			ArchiveID:  archiveID,
			Sequence:   i,
			Role:       string(message.Role),
			Name:       message.Name,
			Content:    message.Content,
			ToolCalls:  encodeToolCalls(message.ToolCalls),
			ToolCallID: message.ToolCallID,
			CreatedAt:  message.CreatedAt,
		})
	}

	return records
}

func decodeSessionMessages(records []messageModel) []handmsg.Message {
	if len(records) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, handmsg.Message{
			Role:       handmsg.Role(record.Role),
			Name:       record.Name,
			Content:    record.Content,
			ToolCalls:  decodeToolCalls(record.ToolCalls),
			ToolCallID: record.ToolCallID,
			CreatedAt:  record.CreatedAt,
		})
	}

	return messages
}

func decodeArchivedMessages(records []archivedMessageModel) []handmsg.Message {
	if len(records) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, handmsg.Message{
			Role:       handmsg.Role(record.Role),
			Name:       record.Name,
			Content:    record.Content,
			ToolCalls:  decodeToolCalls(record.ToolCalls),
			ToolCallID: record.ToolCallID,
			CreatedAt:  record.CreatedAt,
		})
	}

	return messages
}

func encodeToolCalls(toolCalls []handmsg.ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	raw, _ := json.Marshal(toolCalls)

	return string(raw)
}

func encodeStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}

	raw, _ := json.Marshal(values)

	return string(raw)
}

func decodeStrings(value string) []string {
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

func decodeToolCalls(value string) []handmsg.ToolCall {
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

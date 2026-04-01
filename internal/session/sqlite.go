package session

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
)

const currentSessionStateKey = "current_session"

type sqliteRecord struct {
	CreatedAt time.Time
	ID        string `gorm:"primaryKey"`
	UpdatedAt time.Time
}

func (sqliteRecord) TableName() string {
	return "sessions"
}

type sqliteArchiveRecord struct {
	ID              string    `gorm:"primaryKey"`
	SourceSessionID string    `gorm:"index;not null"`
	ArchivedAt      time.Time `gorm:"index"`
	ExpiresAt       time.Time `gorm:"index"`
	CreatedAt       time.Time
}

func (sqliteArchiveRecord) TableName() string {
	return "session_archives"
}

type sqliteStateRecord struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
	CreatedAt time.Time
}

func (sqliteStateRecord) TableName() string {
	return "session_state"
}

type sqliteMessageRecord struct {
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

func (sqliteMessageRecord) TableName() string {
	return "session_messages"
}

type sqliteArchivedMessageRecord struct {
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

func (sqliteArchivedMessageRecord) TableName() string {
	return "archived_session_messages"
}

type SQLiteStore struct {
	db *gorm.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
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

	if err := db.AutoMigrate(
		&sqliteRecord{},
		&sqliteArchiveRecord{},
		&sqliteStateRecord{},
		&sqliteMessageRecord{},
		&sqliteArchivedMessageRecord{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate session db: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Save(ctx context.Context, session Session) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}
	session.ID = strings.TrimSpace(session.ID)
	if err := validateSessionID(session.ID); err != nil {
		return err
	}

	if session.CreatedAt.IsZero() {
		var existing sqliteRecord
		if err := s.db.WithContext(ctx).First(&existing, "id = ?", session.ID).Error; err == nil {
			session.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	session, err := normalizeSession(session)
	if err != nil {
		return err
	}

	record := sqliteRecord{
		CreatedAt: session.CreatedAt,
		ID:        session.ID,
		UpdatedAt: session.UpdatedAt,
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (Session, bool, error) {
	if s == nil || s.db == nil {
		return Session{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, false, nil
	}
	if err := validateSessionID(id); err != nil {
		return Session{}, false, err
	}

	var record sqliteRecord
	if err := s.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}

	session, err := sessionFromRecord(record.CreatedAt, record.ID, record.UpdatedAt)
	if err != nil {
		return Session{}, false, err
	}

	return session, true, nil
}

func (s *SQLiteStore) List(ctx context.Context) ([]Session, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	var records []sqliteRecord
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

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := validateSessionID(id); err != nil {
		return err
	}
	if id == DefaultSessionID {
		return errors.New("default session cannot be deleted")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sqliteRecord
		if err := tx.First(&session, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		if err := tx.Where("session_id = ?", id).Delete(&sqliteMessageRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&session).Error; err != nil {
			return err
		}

		if err := tx.Where("key = ? AND value = ?", currentSessionStateKey, id).
			Delete(&sqliteStateRecord{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (s *SQLiteStore) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := validateSessionID(id); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record sqliteRecord
		if err := tx.First(&record, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		var nextSequence int64
		if err := tx.Model(&sqliteMessageRecord{}).Where("session_id = ?", id).
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

func (s *SQLiteStore) GetMessages(
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
		if err := validateSessionID(id); err != nil {
			return nil, err
		}
	}

	if opts.Archived {
		var records []sqliteArchivedMessageRecord
		if err := s.db.WithContext(ctx).Where("archive_id = ?", id).Order("sequence asc").
			Find(&records).Error; err != nil {
			return nil, err
		}
		return decodeArchivedMessages(records), nil
	}

	var records []sqliteMessageRecord
	if err := s.db.WithContext(ctx).Where("session_id = ?", id).Order("sequence asc").
		Find(&records).Error; err != nil {
		return nil, err
	}

	return decodeSessionMessages(records), nil
}

func (s *SQLiteStore) GetMessage(
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
		if err := validateSessionID(id); err != nil {
			return handmsg.Message{}, false, err
		}
	}

	if opts.Archived {
		var record sqliteArchivedMessageRecord
		if err := s.db.WithContext(ctx).Where("archive_id = ? AND sequence = ?", id, index).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return handmsg.Message{}, false, nil
			}
			return handmsg.Message{}, false, err
		}
		return decodeArchivedMessages([]sqliteArchivedMessageRecord{record})[0], true, nil
	}

	var record sqliteMessageRecord
	if err := s.db.WithContext(ctx).Where("session_id = ? AND sequence = ?", id, index).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return handmsg.Message{}, false, nil
		}
		return handmsg.Message{}, false, err
	}

	return decodeSessionMessages([]sqliteMessageRecord{record})[0], true, nil
}

func (s *SQLiteStore) CreateArchive(ctx context.Context, archive ArchivedSession) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	archive, err := normalizeCreateArchive(archive)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source []sqliteMessageRecord
		if err := tx.Where("session_id = ?", archive.SourceSessionID).Order("sequence asc").
			Find(&source).Error; err != nil {
			return err
		}
		if len(source) == 0 {
			return errors.New("source session has no messages")
		}

		record := sqliteArchiveRecord{
			ID:              archive.ID,
			SourceSessionID: archive.SourceSessionID,
			ArchivedAt:      archive.ArchivedAt,
			ExpiresAt:       archive.ExpiresAt,
		}
		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		if err := tx.Where("archive_id = ?", archive.ID).Delete(&sqliteArchivedMessageRecord{}).Error; err != nil {
			return err
		}

		records := encodeArchivedMessages(archive.ID, decodeSessionMessages(source))
		if err := tx.Create(&records).Error; err != nil {
			return err
		}

		if err := tx.Where("session_id = ?", archive.SourceSessionID).Delete(&sqliteMessageRecord{}).Error; err != nil {
			return err
		}

		if archive.SourceSessionID == DefaultSessionID {
			return nil
		}

		if err := tx.Where("id = ?", archive.SourceSessionID).Delete(&sqliteRecord{}).Error; err != nil {
			return err
		}

		return tx.Where("key = ? AND value = ?", currentSessionStateKey, archive.SourceSessionID).
			Delete(&sqliteStateRecord{}).Error
	})
}

func (s *SQLiteStore) GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error) {
	if s == nil || s.db == nil {
		return ArchivedSession{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ArchivedSession{}, false, nil
	}

	var record sqliteArchiveRecord
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

func (s *SQLiteStore) ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if !opts.Archived {
		if err := validateSessionID(id); err != nil {
			return err
		}
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if opts.Archived {
			var archive sqliteArchiveRecord
			if err := tx.First(&archive, "id = ?", id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("archive not found")
				}
				return err
			}
			return tx.Where("archive_id = ?", id).Delete(&sqliteArchivedMessageRecord{}).Error
		}

		var session sqliteRecord
		if err := tx.First(&session, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("session not found")
			}
			return err
		}

		if err := tx.Where("session_id = ?", id).Delete(&sqliteMessageRecord{}).Error; err != nil {
			return err
		}

		session.UpdatedAt = time.Now().UTC()
		return tx.Save(&session).Error
	})
}

func (s *SQLiteStore) ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session store is required")
	}

	query := s.db.WithContext(ctx).Order("archived_at desc").Order("id asc")
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	if sourceSessionID != "" {
		query = query.Where("source_session_id = ?", sourceSessionID)
	}

	var records []sqliteArchiveRecord
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

func (s *SQLiteStore) DeleteArchives(ctx context.Context, archiveID string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	archiveID = strings.TrimSpace(archiveID)
	if archiveID == "" {
		return errors.New("archive id is required")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var archive sqliteArchiveRecord
		if err := tx.First(&archive, "id = ?", archiveID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("archive not found")
			}
			return err
		}

		if err := tx.Where("archive_id = ?", archiveID).Delete(&sqliteArchivedMessageRecord{}).Error; err != nil {
			return err
		}

		return tx.Delete(&archive).Error
	})
}

func (s *SQLiteStore) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(&sqliteArchiveRecord{}).Where("expires_at <= ?", now.UTC()).
			Pluck("id", &ids).Error; err != nil {
			return err
		}

		if len(ids) == 0 {
			return nil
		}

		if err := tx.Where("archive_id IN ?", ids).Delete(&sqliteArchivedMessageRecord{}).Error; err != nil {
			return err
		}

		return tx.Where("id IN ?", ids).Delete(&sqliteArchiveRecord{}).Error
	})
}

func (s *SQLiteStore) SetCurrent(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := validateSessionID(id); err != nil {
		return err
	}

	_, ok, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("session not found")
	}

	record := sqliteStateRecord{
		Key:       currentSessionStateKey,
		Value:     id,
		UpdatedAt: time.Now().UTC(),
	}

	return s.db.WithContext(ctx).Save(&record).Error
}

func (s *SQLiteStore) Current(ctx context.Context) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("session store is required")
	}

	var record sqliteStateRecord
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

func decodeSessionRecord(record sqliteRecord) (Session, error) {
	return sessionFromRecord(record.CreatedAt, record.ID, record.UpdatedAt)
}

func decodeArchiveRecord(record sqliteArchiveRecord) (ArchivedSession, error) {
	return normalizeCreateArchive(ArchivedSession{
		ID:              record.ID,
		SourceSessionID: record.SourceSessionID,
		ArchivedAt:      record.ArchivedAt,
		ExpiresAt:       record.ExpiresAt,
	})
}

func sessionFromRecord(createdAt time.Time, id string, updatedAt time.Time) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, errors.New("session id is required")
	}

	session := Session{
		CreatedAt: createdAt,
		ID:        id,
	}

	if !createdAt.IsZero() {
		session.CreatedAt = createdAt.UTC()
	}

	if !updatedAt.IsZero() {
		session.UpdatedAt = updatedAt.UTC()
	}

	return session, nil
}

func encodeSessionMessages(sessionID string, messages []handmsg.Message) []sqliteMessageRecord {
	return encodeSessionMessagesWithOffset(sessionID, messages, 0)
}

func encodeSessionMessagesWithOffset(sessionID string, messages []handmsg.Message, offset int) []sqliteMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	records := make([]sqliteMessageRecord, 0, len(messages))
	for i, message := range messages {
		records = append(records, sqliteMessageRecord{
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

func encodeArchivedMessages(archiveID string, messages []handmsg.Message) []sqliteArchivedMessageRecord {
	if len(messages) == 0 {
		return nil
	}

	records := make([]sqliteArchivedMessageRecord, 0, len(messages))
	for i, message := range messages {
		records = append(records, sqliteArchivedMessageRecord{
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

func decodeSessionMessages(records []sqliteMessageRecord) []handmsg.Message {
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

func decodeArchivedMessages(records []sqliteArchivedMessageRecord) []handmsg.Message {
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

	raw, err := json.Marshal(toolCalls)
	if err != nil {
		return ""
	}

	return string(raw)
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

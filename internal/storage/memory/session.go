package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
)

type Session = base.Session
type ArchivedSession = base.ArchivedSession
type MessageQueryOptions = base.MessageQueryOptions
type SessionSummary = base.SessionSummary

type SessionStore struct {
	mu              sync.RWMutex
	sessions        map[string]Session
	messages        map[string][]handmsg.Message
	summaries       map[string]SessionSummary
	archives        map[string]ArchivedSession
	archiveMessages map[string][]handmsg.Message
	currentSession  string
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions:        make(map[string]Session),
		messages:        make(map[string][]handmsg.Message),
		summaries:       make(map[string]SessionSummary),
		archives:        make(map[string]ArchivedSession),
		archiveMessages: make(map[string][]handmsg.Message),
	}
}

func (s *SessionStore) Save(_ context.Context, session Session) error {
	if s == nil {
		return errors.New("session store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session.ID = strings.TrimSpace(session.ID)
	if err := common.ValidateSessionID(session.ID); err != nil {
		return err
	}

	if existing, ok := s.sessions[session.ID]; ok {
		session.CreatedAt = existing.CreatedAt
		if session.Compaction == (base.SessionCompaction{}) {
			session.Compaction = existing.Compaction
		}
		session.UpdatedAt = time.Now().UTC()
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

	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStore) Get(_ context.Context, id string) (Session, bool, error) {
	if s == nil {
		return Session{}, false, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[strings.TrimSpace(id)]
	return session, ok, nil
}

func (s *SessionStore) List(context.Context) ([]Session, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
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

func (s *SessionStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	if id == base.DefaultSessionID {
		return errors.New("default session cannot be deleted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return errors.New("session not found")
	}

	delete(s.sessions, id)
	delete(s.messages, id)
	delete(s.summaries, id)
	if s.currentSession == id {
		s.currentSession = ""
	}

	return nil
}

func (s *SessionStore) AppendMessages(_ context.Context, id string, messages []handmsg.Message) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}

	copied := cloneMessages(messages)
	s.messages[id] = append(s.messages[id], copied...)
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session

	return nil
}

func (s *SessionStore) GetMessages(
	_ context.Context,
	id string,
	opts MessageQueryOptions,
) ([]handmsg.Message, error) {
	if s == nil {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	if opts.Archived {
		return queryMessages(s.archiveMessages[id], opts), nil
	}

	return queryMessages(s.messages[id], opts), nil
}

func (s *SessionStore) CountMessages(_ context.Context, id string, opts MessageQueryOptions) (int, error) {
	if s == nil {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	if opts.Archived {
		return len(filterMessages(s.archiveMessages[id], opts)), nil
	}

	return len(filterMessages(s.messages[id], opts)), nil
}

func (s *SessionStore) GetMessage(_ context.Context, id string, index int, opts MessageQueryOptions) (handmsg.Message, bool, error) {
	if s == nil {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	var messages []handmsg.Message
	if opts.Archived {
		messages = s.archiveMessages[id]
	} else {
		messages = s.messages[id]
	}

	if index >= len(messages) {
		return handmsg.Message{}, false, nil
	}

	return cloneMessages(messages[index : index+1])[0], true, nil
}

func (s *SessionStore) CreateArchive(_ context.Context, archive ArchivedSession) error {
	if s == nil {
		return errors.New("session store is required")
	}

	normalized, err := common.NormalizeCreateArchive(archive)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceMessages := s.messages[normalized.SourceSessionID]
	if len(sourceMessages) == 0 {
		return errors.New("source session has no messages")
	}

	s.archiveMessages[normalized.ID] = cloneMessages(sourceMessages)
	s.archives[normalized.ID] = normalized

	delete(s.messages, normalized.SourceSessionID)
	delete(s.summaries, normalized.SourceSessionID)
	if normalized.SourceSessionID != base.DefaultSessionID {
		delete(s.sessions, normalized.SourceSessionID)
		if s.currentSession == normalized.SourceSessionID {
			s.currentSession = ""
		}
	} else if session, ok := s.sessions[normalized.SourceSessionID]; ok {
		session.Compaction = base.SessionCompaction{}
		s.sessions[normalized.SourceSessionID] = session
	}

	return nil
}

func (s *SessionStore) GetArchive(_ context.Context, id string) (ArchivedSession, bool, error) {
	if s == nil {
		return ArchivedSession{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ArchivedSession{}, false, nil
	}

	if err := common.ValidateArchiveID(id); err != nil {
		return ArchivedSession{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	archive, ok := s.archives[id]
	return archive, ok, nil
}

func (s *SessionStore) ListArchives(_ context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	sourceSessionID = strings.TrimSpace(sourceSessionID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	archives := make([]ArchivedSession, 0, len(s.archives))
	for _, archive := range s.archives {
		if sourceSessionID != "" && archive.SourceSessionID != sourceSessionID {
			continue
		}
		archives = append(archives, archive)
	}

	sort.Slice(archives, func(i, j int) bool {
		if archives[i].ArchivedAt.Equal(archives[j].ArchivedAt) {
			return archives[i].ID < archives[j].ID
		}

		return archives[i].ArchivedAt.After(archives[j].ArchivedAt)
	})

	return archives, nil
}

func (s *SessionStore) DeleteArchive(_ context.Context, archiveID string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	archiveID = strings.TrimSpace(archiveID)
	if err := common.ValidateArchiveID(archiveID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.archives[archiveID]; !ok {
		return errors.New("archive not found")
	}

	delete(s.archives, archiveID)
	delete(s.archiveMessages, archiveID)
	return nil
}

func (s *SessionStore) DeleteExpiredArchives(_ context.Context, now time.Time) error {
	if s == nil {
		return errors.New("session store is required")
	}

	now = now.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, archive := range s.archives {
		if !archive.ExpiresAt.IsZero() && !archive.ExpiresAt.After(now) {
			delete(s.archives, id)
			delete(s.archiveMessages, id)
		}
	}

	return nil
}

func (s *SessionStore) ClearMessages(_ context.Context, id string, opts MessageQueryOptions) error {
	if s == nil {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if opts.Archived {
		if _, ok := s.archives[id]; !ok {
			return errors.New("archive not found")
		}
		delete(s.archiveMessages, id)
		return nil
	}

	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}

	delete(s.messages, id)
	delete(s.summaries, id)
	session.Compaction = base.SessionCompaction{}
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return nil
}

func (s *SessionStore) SaveSummary(_ context.Context, summary SessionSummary) error {
	if s == nil {
		return errors.New("session store is required")
	}

	normalized, err := common.NormalizeSessionSummary(summary)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[normalized.SessionID]; !ok {
		return errors.New("session not found")
	}

	s.summaries[normalized.SessionID] = common.CloneSessionSummary(normalized)
	return nil
}

func (s *SessionStore) GetSummary(_ context.Context, sessionID string) (SessionSummary, bool, error) {
	if s == nil {
		return SessionSummary{}, false, errors.New("session store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionSummary{}, false, nil
	}

	if err := common.ValidateSessionID(sessionID); err != nil {
		return SessionSummary{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	summary, ok := s.summaries[sessionID]
	if !ok {
		return SessionSummary{}, false, nil
	}

	return common.CloneSessionSummary(summary), true, nil
}

func (s *SessionStore) DeleteSummary(_ context.Context, sessionID string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if err := common.ValidateSessionID(sessionID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.summaries, sessionID)
	return nil
}

func (s *SessionStore) SetCurrent(_ context.Context, id string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return errors.New("session not found")
	}

	s.currentSession = id
	return nil
}

func (s *SessionStore) Current(_ context.Context) (string, bool, error) {
	if s == nil {
		return "", false, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if strings.TrimSpace(s.currentSession) == "" {
		return "", false, nil
	}

	return s.currentSession, true, nil
}

func cloneMessages(messages []handmsg.Message) []handmsg.Message {
	return common.CloneMessages(messages)
}

func queryMessages(messages []handmsg.Message, opts MessageQueryOptions) []handmsg.Message {
	filtered := filterMessages(messages, opts)
	if messageQueryOrder(opts) == base.MessageOrderDesc {
		filtered = reverseMessages(filtered)
	}
	offset := max(opts.Offset, 0)
	if offset >= len(filtered) {
		return nil
	}

	end := len(filtered)
	if opts.Limit > 0 && offset+opts.Limit < end {
		end = offset + opts.Limit
	}

	return cloneMessages(filtered[offset:end])
}

func reverseMessages(messages []handmsg.Message) []handmsg.Message {
	if len(messages) == 0 {
		return nil
	}

	reversed := make([]handmsg.Message, len(messages))
	for idx := range messages {
		reversed[len(messages)-1-idx] = messages[idx]
	}

	return reversed
}

func filterMessages(messages []handmsg.Message, opts MessageQueryOptions) []handmsg.Message {
	role := handmsg.Role(strings.TrimSpace(string(opts.Role)))
	name := strings.TrimSpace(opts.Name)
	if role == "" && name == "" {
		return messages
	}

	filtered := make([]handmsg.Message, 0, len(messages))
	for _, message := range messages {
		if role != "" && message.Role != role {
			continue
		}
		if name != "" && strings.TrimSpace(message.Name) != name {
			continue
		}
		filtered = append(filtered, message)
	}

	return filtered
}

func messageQueryOrder(opts MessageQueryOptions) string {
	order, err := base.NormalizeMessageQueryOrder(opts.Order)
	if err != nil {
		return base.MessageOrderAsc
	}

	return order
}

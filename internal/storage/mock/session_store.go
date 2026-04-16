package mock

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
)

type SessionStore struct {
	GetFunc                   func(context.Context, string) (storage.Session, bool, error)
	GetSummaryFunc            func(context.Context, string) (storage.SessionSummary, bool, error)
	ListFunc                  func(context.Context) ([]storage.Session, error)
	SaveFunc                  func(context.Context, storage.Session) error
	SaveSummaryFunc           func(context.Context, storage.SessionSummary) error
	DeleteFunc                func(context.Context, string) error
	DeleteSummaryFunc         func(context.Context, string) error
	SetCurrentFunc            func(context.Context, string) error
	CurrentFunc               func(context.Context) (string, bool, error)
	AppendMessagesFunc        func(context.Context, string, []handmsg.Message) error
	CountMessagesFunc         func(context.Context, string, storage.MessageQueryOptions) (int, error)
	GetMessageFunc            func(context.Context, string, int, storage.MessageQueryOptions) (handmsg.Message, bool, error)
	GetMessagesFunc           func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	SearchMessagesFunc        func(context.Context, string, storage.SearchMessageOptions) ([]handmsg.Message, error)
	ClearMessagesFunc         func(context.Context, string, storage.MessageQueryOptions) error
	CreateArchiveFunc         func(context.Context, storage.ArchivedSession) error
	GetArchiveFunc            func(context.Context, string) (storage.ArchivedSession, bool, error)
	ListArchivesFunc          func(context.Context, string) ([]storage.ArchivedSession, error)
	DeleteArchiveFunc         func(context.Context, string) error
	DeleteExpiredArchivesFunc func(context.Context, time.Time) error
}

func (s *SessionStore) Save(ctx context.Context, session storage.Session) error {
	if s.SaveFunc != nil {
		return s.SaveFunc(ctx, session)
	}

	return nil
}

func (s *SessionStore) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	if s.GetFunc != nil {
		return s.GetFunc(ctx, id)
	}

	return storage.Session{}, false, nil
}

func (s *SessionStore) SaveSummary(ctx context.Context, summary storage.SessionSummary) error {
	if s.SaveSummaryFunc != nil {
		return s.SaveSummaryFunc(ctx, summary)
	}

	return nil
}

func (s *SessionStore) GetSummary(ctx context.Context, sessionID string) (storage.SessionSummary, bool, error) {
	if s.GetSummaryFunc != nil {
		return s.GetSummaryFunc(ctx, sessionID)
	}

	return storage.SessionSummary{}, false, nil
}

func (s *SessionStore) DeleteSummary(ctx context.Context, sessionID string) error {
	if s.DeleteSummaryFunc != nil {
		return s.DeleteSummaryFunc(ctx, sessionID)
	}

	return nil
}

func (s *SessionStore) List(ctx context.Context) ([]storage.Session, error) {
	if s.ListFunc != nil {
		return s.ListFunc(ctx)
	}

	return nil, nil
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if s.DeleteFunc != nil {
		return s.DeleteFunc(ctx, id)
	}

	return nil
}

func (s *SessionStore) CreateArchive(ctx context.Context, archive storage.ArchivedSession) error {
	if s.CreateArchiveFunc != nil {
		return s.CreateArchiveFunc(ctx, archive)
	}

	return nil
}

func (s *SessionStore) GetArchive(ctx context.Context, id string) (storage.ArchivedSession, bool, error) {
	if s.GetArchiveFunc != nil {
		return s.GetArchiveFunc(ctx, id)
	}

	return storage.ArchivedSession{}, false, nil
}

func (s *SessionStore) ListArchives(ctx context.Context, sourceSessionID string) ([]storage.ArchivedSession, error) {
	if s.ListArchivesFunc != nil {
		return s.ListArchivesFunc(ctx, sourceSessionID)
	}

	return nil, nil
}

func (s *SessionStore) DeleteArchive(ctx context.Context, archiveID string) error {
	if s.DeleteArchiveFunc != nil {
		return s.DeleteArchiveFunc(ctx, archiveID)
	}

	return nil
}

func (s *SessionStore) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s.DeleteExpiredArchivesFunc != nil {
		return s.DeleteExpiredArchivesFunc(ctx, now)
	}

	return nil
}

func (s *SessionStore) SetCurrent(ctx context.Context, id string) error {
	if s.SetCurrentFunc != nil {
		return s.SetCurrentFunc(ctx, id)
	}

	return nil
}

func (s *SessionStore) Current(ctx context.Context) (string, bool, error) {
	if s.CurrentFunc != nil {
		return s.CurrentFunc(ctx)
	}

	return "", false, nil
}

func (s *SessionStore) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s.AppendMessagesFunc != nil {
		return s.AppendMessagesFunc(ctx, id, messages)
	}

	return nil
}

func (s *SessionStore) CountMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) (int, error) {
	if s.CountMessagesFunc != nil {
		return s.CountMessagesFunc(ctx, id, opts)
	}

	return 0, nil
}

func (s *SessionStore) GetMessage(ctx context.Context, id string, index int, opts storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	if s.GetMessageFunc != nil {
		return s.GetMessageFunc(ctx, id, index, opts)
	}

	return handmsg.Message{}, false, nil
}

func (s *SessionStore) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	if s.GetMessagesFunc != nil {
		return s.GetMessagesFunc(ctx, id, opts)
	}

	return nil, nil
}

func (s *SessionStore) SearchMessages(ctx context.Context, id string, opts storage.SearchMessageOptions) ([]handmsg.Message, error) {
	if s.SearchMessagesFunc != nil {
		return s.SearchMessagesFunc(ctx, id, opts)
	}

	return nil, nil
}

func (s *SessionStore) ClearMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) error {
	if s.ClearMessagesFunc != nil {
		return s.ClearMessagesFunc(ctx, id, opts)
	}

	return nil
}

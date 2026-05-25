package mock

import (
	"context"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// Store records state-store calls for tests.
type Store struct {
	GetFunc                   func(context.Context, string) (storage.Session, bool, error)
	GetSummaryFunc            func(context.Context, string) (storage.SessionSummary, bool, error)
	ListFunc                  func(context.Context) ([]storage.Session, error)
	SaveFunc                  func(context.Context, storage.Session) error
	SaveSummaryFunc           func(context.Context, storage.SessionSummary) error
	DeleteFunc                func(context.Context, string) error
	DeleteSummaryFunc         func(context.Context, string) error
	UpdateCheckpointsFunc     func(context.Context, string, storage.CheckpointPatch) error
	SetCurrentFunc            func(context.Context, string) error
	CurrentFunc               func(context.Context) (string, bool, error)
	AppendMessagesFunc        func(context.Context, string, []handmsg.Message) error
	CountMessagesFunc         func(context.Context, string, storage.MessageQueryOptions) (int, error)
	GetMessageFunc            func(context.Context, string, int, storage.MessageQueryOptions) (handmsg.Message, bool, error)
	GetMessagesFunc           func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	GetMessagesByIDsFunc      func(context.Context, string, []uint) ([]storage.MessageRecord, error)
	GetMessageWindowFunc      func(context.Context, string, uint, int, int) ([]storage.MessageRecord, error)
	SearchMessagesFunc        func(context.Context, string, storage.SearchMessageOptions) ([]storage.SearchMessageResult, error)
	ClearMessagesFunc         func(context.Context, string, storage.MessageQueryOptions) error
	CreateArchiveFunc         func(context.Context, storage.ArchivedSession) error
	GetArchiveFunc            func(context.Context, string) (storage.ArchivedSession, bool, error)
	ListArchivesFunc          func(context.Context, string) ([]storage.ArchivedSession, error)
	DeleteArchiveFunc         func(context.Context, string) error
	DeleteExpiredArchivesFunc func(context.Context, time.Time) error
}

func (s *Store) Save(ctx context.Context, session storage.Session) error {
	if s.SaveFunc != nil {
		return s.SaveFunc(ctx, session)
	}

	return nil
}

func (s *Store) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	if s.GetFunc != nil {
		return s.GetFunc(ctx, id)
	}

	return storage.Session{}, false, nil
}

func (s *Store) SaveSummary(ctx context.Context, summary storage.SessionSummary) error {
	if s.SaveSummaryFunc != nil {
		return s.SaveSummaryFunc(ctx, summary)
	}

	return nil
}

func (s *Store) GetSummary(ctx context.Context, sessionID string) (storage.SessionSummary, bool, error) {
	if s.GetSummaryFunc != nil {
		return s.GetSummaryFunc(ctx, sessionID)
	}

	return storage.SessionSummary{}, false, nil
}

func (s *Store) DeleteSummary(ctx context.Context, sessionID string) error {
	if s.DeleteSummaryFunc != nil {
		return s.DeleteSummaryFunc(ctx, sessionID)
	}

	return nil
}

func (s *Store) List(ctx context.Context) ([]storage.Session, error) {
	if s.ListFunc != nil {
		return s.ListFunc(ctx)
	}

	return nil, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if s.DeleteFunc != nil {
		return s.DeleteFunc(ctx, id)
	}

	return nil
}

func (s *Store) UpdateCheckpoints(ctx context.Context, id string, patch storage.CheckpointPatch) error {
	if s.UpdateCheckpointsFunc != nil {
		return s.UpdateCheckpointsFunc(ctx, id, patch)
	}

	return nil
}

func (s *Store) CreateArchive(ctx context.Context, archive storage.ArchivedSession) error {
	if s.CreateArchiveFunc != nil {
		return s.CreateArchiveFunc(ctx, archive)
	}

	return nil
}

func (s *Store) GetArchive(ctx context.Context, id string) (storage.ArchivedSession, bool, error) {
	if s.GetArchiveFunc != nil {
		return s.GetArchiveFunc(ctx, id)
	}

	return storage.ArchivedSession{}, false, nil
}

func (s *Store) ListArchives(ctx context.Context, sourceSessionID string) ([]storage.ArchivedSession, error) {
	if s.ListArchivesFunc != nil {
		return s.ListArchivesFunc(ctx, sourceSessionID)
	}

	return nil, nil
}

func (s *Store) DeleteArchive(ctx context.Context, archiveID string) error {
	if s.DeleteArchiveFunc != nil {
		return s.DeleteArchiveFunc(ctx, archiveID)
	}

	return nil
}

func (s *Store) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s.DeleteExpiredArchivesFunc != nil {
		return s.DeleteExpiredArchivesFunc(ctx, now)
	}

	return nil
}

func (s *Store) SetCurrent(ctx context.Context, id string) error {
	if s.SetCurrentFunc != nil {
		return s.SetCurrentFunc(ctx, id)
	}

	return nil
}

func (s *Store) Current(ctx context.Context) (string, bool, error) {
	if s.CurrentFunc != nil {
		return s.CurrentFunc(ctx)
	}

	return "", false, nil
}

func (s *Store) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s.AppendMessagesFunc != nil {
		return s.AppendMessagesFunc(ctx, id, messages)
	}

	return nil
}

func (s *Store) CountMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) (int, error) {
	if s.CountMessagesFunc != nil {
		return s.CountMessagesFunc(ctx, id, opts)
	}

	return 0, nil
}

func (s *Store) GetMessage(ctx context.Context, id string, index int, opts storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	if s.GetMessageFunc != nil {
		return s.GetMessageFunc(ctx, id, index, opts)
	}

	return handmsg.Message{}, false, nil
}

func (s *Store) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	if s.GetMessagesFunc != nil {
		return s.GetMessagesFunc(ctx, id, opts)
	}

	return nil, nil
}

func (s *Store) GetMessagesByIDs(ctx context.Context, id string, messageIDs []uint) ([]storage.MessageRecord, error) {
	if s.GetMessagesByIDsFunc != nil {
		return s.GetMessagesByIDsFunc(ctx, id, messageIDs)
	}

	return nil, nil
}

func (s *Store) GetMessageWindow(ctx context.Context, id string, anchorMessageID uint, before int, after int) ([]storage.MessageRecord, error) {
	if s.GetMessageWindowFunc != nil {
		return s.GetMessageWindowFunc(ctx, id, anchorMessageID, before, after)
	}

	return nil, nil
}

func (s *Store) SearchMessages(ctx context.Context, id string, opts storage.SearchMessageOptions) ([]storage.SearchMessageResult, error) {
	if s.SearchMessagesFunc != nil {
		return s.SearchMessagesFunc(ctx, id, opts)
	}

	return nil, nil
}

func (s *Store) ClearMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) error {
	if s.ClearMessagesFunc != nil {
		return s.ClearMessagesFunc(ctx, id, opts)
	}

	return nil
}

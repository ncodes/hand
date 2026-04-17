package e2e

import (
	"context"
	"time"

	"github.com/wandxy/hand/internal/agent"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
)

type harnessAgentStub struct {
	reply      string
	respondErr error
	current    string
	currentErr error
	events     []agent.Event
}

func (s harnessAgentStub) Respond(_ context.Context, _ string, opts agent.RespondOptions) (string, error) {
	if s.respondErr != nil {
		return "", s.respondErr
	}
	if opts.OnEvent != nil {
		for _, event := range s.events {
			opts.OnEvent(event)
		}
	}
	return s.reply, nil
}

func (s harnessAgentStub) CurrentSession(context.Context) (string, error) {
	if s.currentErr != nil {
		return "", s.currentErr
	}
	return s.current, nil
}

type storageStoreStub struct {
	messages []handmsg.Message
}

func (s *storageStoreStub) Save(context.Context, storage.Session) error { return nil }
func (s *storageStoreStub) Get(context.Context, string) (storage.Session, bool, error) {
	return storage.Session{}, false, nil
}
func (s *storageStoreStub) List(context.Context) ([]storage.Session, error) { return nil, nil }
func (s *storageStoreStub) Delete(context.Context, string) error            { return nil }
func (s *storageStoreStub) AppendMessages(context.Context, string, []handmsg.Message) error {
	return nil
}
func (s *storageStoreStub) GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
	return s.messages, nil
}
func (s *storageStoreStub) SearchMessages(context.Context, string, storage.SearchMessageOptions) ([]handmsg.Message, error) {
	return nil, nil
}
func (s *storageStoreStub) CountMessages(context.Context, string, storage.MessageQueryOptions) (int, error) {
	return 0, nil
}
func (s *storageStoreStub) GetMessage(context.Context, string, int, storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	return handmsg.Message{}, false, nil
}
func (s *storageStoreStub) SaveSummary(context.Context, storage.SessionSummary) error { return nil }
func (s *storageStoreStub) GetSummary(context.Context, string) (storage.SessionSummary, bool, error) {
	return storage.SessionSummary{}, false, nil
}
func (s *storageStoreStub) DeleteSummary(context.Context, string) error { return nil }
func (s *storageStoreStub) CreateArchive(context.Context, storage.ArchivedSession) error {
	return nil
}
func (s *storageStoreStub) GetArchive(context.Context, string) (storage.ArchivedSession, bool, error) {
	return storage.ArchivedSession{}, false, nil
}
func (s *storageStoreStub) ListArchives(context.Context, string) ([]storage.ArchivedSession, error) {
	return nil, nil
}
func (s *storageStoreStub) DeleteArchive(context.Context, string) error { return nil }
func (s *storageStoreStub) DeleteExpiredArchives(context.Context, time.Time) error {
	return nil
}
func (s *storageStoreStub) ClearMessages(context.Context, string, storage.MessageQueryOptions) error {
	return nil
}
func (s *storageStoreStub) SetCurrent(context.Context, string) error { return nil }
func (s *storageStoreStub) Current(context.Context) (string, bool, error) {
	return "", false, nil
}

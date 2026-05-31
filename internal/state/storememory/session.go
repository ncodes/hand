package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	base "github.com/wandxy/hand/internal/state/core"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// Session aliases base.Session at this package boundary.
type Session = base.Session

// MessageQueryOptions aliases base.MessageQueryOptions at this package boundary.
type MessageQueryOptions = base.MessageQueryOptions

// SessionSummary aliases base.SessionSummary at this package boundary.
type SessionSummary = base.SessionSummary

// MessageRecord aliases base.MessageRecord at this package boundary.
type MessageRecord = base.MessageRecord

// CheckpointPatch aliases base.CheckpointPatch at this package boundary.
type CheckpointPatch = base.CheckpointPatch

func (s *Store) Save(_ context.Context, session Session) error {
	if s == nil {
		return errors.New("store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session.ID = strings.TrimSpace(session.ID)
	if err := base.ValidateSessionID(session.ID); err != nil {
		return err
	}

	if existing, ok := s.sessions[session.ID]; ok {
		session.CreatedAt = existing.CreatedAt
		if !session.Archived && session.ArchivedAt.IsZero() && session.ExpiresAt.IsZero() {
			session.Archived = existing.Archived
			session.ArchivedAt = existing.ArchivedAt
			session.ExpiresAt = existing.ExpiresAt
		}
		if session.Compaction == (base.SessionCompaction{}) {
			session.Compaction = existing.Compaction
		}
		if session.EpisodicCheckpointOffset == 0 {
			session.EpisodicCheckpointOffset = existing.EpisodicCheckpointOffset
		}
		if session.ReflectionCheckpointOffset == 0 {
			session.ReflectionCheckpointOffset = existing.ReflectionCheckpointOffset
		}
		if strings.TrimSpace(session.Title) == "" {
			session.Title = existing.Title
			session.TitleSource = existing.TitleSource
		}
		session.UpdatedAt = time.Now().UTC()
	}
	session.Title, session.TitleSource = base.NormalizeSessionTitleMetadata(session.Title, session.TitleSource)

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
	if !session.ArchivedAt.IsZero() {
		session.ArchivedAt = session.ArchivedAt.UTC()
	}
	if !session.ExpiresAt.IsZero() {
		session.ExpiresAt = session.ExpiresAt.UTC()
	}

	s.sessions[session.ID] = session
	return nil
}

func (s *Store) UpdateCheckpoints(_ context.Context, id string, patch CheckpointPatch) error {
	if s == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return err
	}
	if patch.EpisodicOffset != nil && *patch.EpisodicOffset < 0 {
		return errors.New("episodic checkpoint offset must be greater than or equal to zero")
	}
	if patch.ReflectionOffset != nil && *patch.ReflectionOffset < 0 {
		return errors.New("reflection checkpoint offset must be greater than or equal to zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}

	changed := false
	if patch.EpisodicOffset != nil && *patch.EpisodicOffset > session.EpisodicCheckpointOffset {
		session.EpisodicCheckpointOffset = *patch.EpisodicOffset
		changed = true
	}
	if patch.ReflectionOffset != nil && *patch.ReflectionOffset > session.ReflectionCheckpointOffset {
		session.ReflectionCheckpointOffset = *patch.ReflectionOffset
		changed = true
	}
	if changed {
		s.sessions[id] = session
	}

	return nil
}

func (s *Store) Get(_ context.Context, id string, opts base.SessionGetOptions) (Session, bool, error) {
	if s == nil {
		return Session{}, false, errors.New("store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[strings.TrimSpace(id)]
	if !ok || !sessionMatchesGetOptions(session, opts) {
		return Session{}, false, nil
	}

	return session, ok, nil
}

func (s *Store) List(_ context.Context, opts base.SessionListOptions) ([]Session, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if !sessionMatchesListOptions(session, opts) {
			continue
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

func sessionMatchesGetOptions(session Session, opts base.SessionGetOptions) bool {
	return opts.Archived == nil || session.Archived == *opts.Archived
}

func sessionMatchesListOptions(session Session, opts base.SessionListOptions) bool {
	return opts.Archived == nil || session.Archived == *opts.Archived
}

func (s *Store) Rename(_ context.Context, req base.SessionRenameRequest) (Session, error) {
	if s == nil {
		return Session{}, errors.New("store is required")
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	if err := base.ValidateSessionID(req.SessionID); err != nil {
		return Session{}, err
	}

	title, titleSource := base.NormalizeSessionTitleMetadata(req.Title, req.TitleSource)
	if title == "" {
		return Session{}, errors.New("session title is required")
	}

	renamedAt := req.RenamedAt.UTC()
	if renamedAt.IsZero() {
		renamedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[req.SessionID]
	if !ok {
		return Session{}, errors.New("session not found")
	}

	session.Title = title
	session.TitleSource = titleSource
	session.UpdatedAt = renamedAt
	s.sessions[req.SessionID] = session

	return session, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return err
	}

	if id == base.DefaultSessionID {
		return errors.New("default session cannot be deleted")
	}

	s.mu.Lock()

	if _, ok := s.sessions[id]; !ok {
		s.mu.Unlock()
		return errors.New("session not found")
	}

	delete(s.sessions, id)
	delete(s.messages, id)
	delete(s.summaries, id)
	if s.currentSession == id {
		s.currentSession = ""
	}
	s.mu.Unlock()

	return s.handleVectorStoreError(s.deleteVectorRowsBySession(ctx, id))
}

func (s *Store) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	s.mu.Lock()

	session, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("session not found")
	}

	copied := cloneMessages(messages)
	for i := range copied {
		if copied[i].ID == 0 {
			s.nextMessageID++
			copied[i].ID = s.nextMessageID
		}
	}
	s.messages[id] = append(s.messages[id], copied...)
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	s.mu.Unlock()

	return s.handleVectorStoreError(s.indexVectors(ctx, id, copied))
}

func (s *Store) GetMessages(
	_ context.Context,
	id string,
	opts MessageQueryOptions,
) ([]handmsg.Message, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return nil, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}

	if err := base.ValidateSessionID(id); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return getMessagesForQuery(s.messages[id], opts), nil
}

func (s *Store) GetMessagesByIDs(
	_ context.Context,
	id string,
	messageIDs []uint,
) ([]base.MessageRecord, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	if err := base.ValidateSessionID(id); err != nil {
		return nil, err
	}
	if len(messageIDs) == 0 {
		return nil, nil
	}

	selected := make(map[uint]struct{}, len(messageIDs))
	for _, messageID := range messageIDs {
		if messageID == 0 {
			continue
		}
		selected[messageID] = struct{}{}
	}
	if len(selected) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := s.messages[id]
	records := make([]base.MessageRecord, 0, len(selected))
	for idx, message := range messages {
		if _, ok := selected[message.ID]; !ok {
			continue
		}

		records = append(records, base.MessageRecord{
			Offset:  idx,
			Message: cloneMessages([]handmsg.Message{message})[0],
		})
	}

	return records, nil
}

func (s *Store) GetMessageWindow(
	_ context.Context,
	id string,
	anchorMessageID uint,
	before int,
	after int,
) ([]base.MessageRecord, error) {
	if s == nil {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := s.messages[id]
	anchorIndex := -1
	for idx, message := range messages {
		if message.ID == anchorMessageID {
			anchorIndex = idx
			break
		}
	}
	if anchorIndex < 0 {
		return nil, nil
	}

	start := max(anchorIndex-before, 0)
	end := min(anchorIndex+after+1, len(messages))
	records := make([]base.MessageRecord, 0, end-start)
	for idx := start; idx < end; idx++ {
		records = append(records, base.MessageRecord{
			Offset:  idx,
			Message: cloneMessages([]handmsg.Message{messages[idx]})[0],
		})
	}

	return records, nil
}

func (s *Store) SearchMessages(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
) ([]base.SearchMessageResult, error) {
	if s == nil {
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

	query := strings.TrimSpace(strings.ToLower(opts.Query))
	if query == "" {
		return nil, nil
	}

	if s.vectors != nil {
		return s.searchMessagesHybrid(ctx, id, opts, query)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if id != "" {
		results := searchMessageResults(id, s.messages[id], query, opts)
		if len(results) == 0 {
			return nil, nil
		}
		return cloneSearchMessageResults(results), nil
	}

	results := make([]base.SearchMessageResult, 0, len(s.messages))
	for sessionID, messages := range s.messages {
		if sessionID == opts.IgnoreSessionID {
			continue
		}
		results = append(results, searchMessageResults(sessionID, messages, query, opts)...)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].LastMatchedAt.Equal(results[j].LastMatchedAt) {
			return results[i].SessionID < results[j].SessionID
		}
		return results[i].LastMatchedAt.After(results[j].LastMatchedAt)
	})

	if opts.MaxSessions > 0 && len(results) > opts.MaxSessions {
		results = results[:opts.MaxSessions]
	}

	return cloneSearchMessageResults(results), nil
}

func searchMessageResults(
	sessionID string,
	messages []handmsg.Message,
	query string,
	opts base.SearchMessageOptions,
) []base.SearchMessageResult {
	hitOpts := base.SearchMessageOptions{
		Query:    query,
		Role:     opts.Role,
		ToolName: opts.ToolName,
	}
	hits := getMatchingMessageHits(sessionID, messages, query, hitOpts)
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Message.CreatedAt.Equal(hits[j].Message.CreatedAt) {
			return hits[i].Message.ID > hits[j].Message.ID
		}
		return hits[i].Message.CreatedAt.After(hits[j].Message.CreatedAt)
	})

	result := base.SearchMessageResult{
		SessionID:     sessionID,
		LastMatchedAt: hits[0].Message.CreatedAt,
		MatchCount:    len(hits),
		Messages:      hits,
	}
	if opts.MaxMessagesPerSession > 0 && len(result.Messages) > opts.MaxMessagesPerSession {
		result.Messages = result.Messages[:opts.MaxMessagesPerSession]
	}

	return []base.SearchMessageResult{result}
}

func getMatchingMessageHits(
	sessionID string,
	messages []handmsg.Message,
	query string,
	opts base.SearchMessageOptions,
) []base.SearchMessageHit {
	results := make([]base.SearchMessageHit, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		hit, ok := getMatchedMessageHit(sessionID, messages[i], query, opts)
		if !ok {
			continue
		}
		results = append(results, hit)
	}

	return results
}

func getMatchedMessageHit(
	sessionID string,
	message handmsg.Message,
	query string,
	opts base.SearchMessageOptions,
) (base.SearchMessageHit, bool) {
	if opts.Role != "" && message.Role != opts.Role {
		return base.SearchMessageHit{}, false
	}

	makeHit := func(matchedText string, matchedToolName string) (base.SearchMessageHit, bool) {
		matchedText = strings.TrimSpace(matchedText)
		if matchedText == "" {
			return base.SearchMessageHit{}, false
		}
		if !strings.Contains(strings.ToLower(matchedText), query) {
			return base.SearchMessageHit{}, false
		}

		return base.SearchMessageHit{
			SessionID:       sessionID,
			Message:         message,
			MatchedText:     matchedText,
			MatchedToolName: strings.TrimSpace(matchedToolName),
		}, true
	}

	switch message.Role {
	case handmsg.RoleAssistant:
		if opts.ToolName != "" {
			for _, toolCall := range message.ToolCalls {
				if !strings.EqualFold(strings.TrimSpace(toolCall.Name), opts.ToolName) {
					continue
				}
				return makeHit(handmsg.ToolCallSearchText(toolCall), toolCall.Name)
			}
			return base.SearchMessageHit{}, false
		}

		for _, toolCall := range message.ToolCalls {
			if hit, ok := makeHit(handmsg.ToolCallSearchText(toolCall), toolCall.Name); ok {
				return hit, true
			}
		}

		if hit, ok := makeHit(message.Content, ""); ok {
			return hit, true
		}

		return base.SearchMessageHit{}, false
	case handmsg.RoleTool:
		if opts.ToolName != "" && !strings.EqualFold(strings.TrimSpace(message.Name), opts.ToolName) {
			return base.SearchMessageHit{}, false
		}
		if hit, ok := makeHit(handmsg.MessageSearchText(message), message.Name); ok {
			return hit, true
		}
		return makeHit(message.Content, message.Name)
	default:
		if opts.ToolName != "" {
			return base.SearchMessageHit{}, false
		}
		return makeHit(message.Content, "")
	}
}

func (s *Store) CountMessages(_ context.Context, id string, opts MessageQueryOptions) (int, error) {
	if s == nil {
		return 0, errors.New("store is required")
	}

	if _, err := base.NormalizeMessageQueryOrder(opts.Order); err != nil {
		return 0, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil
	}

	if err := base.ValidateSessionID(id); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(filterMessages(s.messages[id], opts)), nil
}

func (s *Store) GetMessage(_ context.Context, id string, index int) (handmsg.Message, bool, error) {
	if s == nil {
		return handmsg.Message{}, false, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" || index < 0 {
		return handmsg.Message{}, false, nil
	}

	if err := base.ValidateSessionID(id); err != nil {
		return handmsg.Message{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := s.messages[id]
	if index >= len(messages) {
		return handmsg.Message{}, false, nil
	}

	return cloneMessages(messages[index : index+1])[0], true, nil
}

func (s *Store) Archive(_ context.Context, id string, req base.SessionArchiveRequest) (Session, error) {
	if s == nil {
		return Session{}, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sessions[id]
	if !ok {
		return Session{}, errors.New("session not found")
	}
	sourceMessages := s.messages[id]
	if len(sourceMessages) == 0 {
		return Session{}, errors.New("source session has no messages")
	}

	session, err := base.MarkSessionArchived(source, req.ArchivedAt, req.ExpiresAt)
	if err != nil {
		return Session{}, err
	}
	s.sessions[session.ID] = session
	if s.currentSession == id {
		s.currentSession = ""
	}

	return session, nil
}

func (s *Store) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s == nil {
		return errors.New("store is required")
	}

	now = now.UTC()

	var expiredSessionIDs []string
	s.mu.Lock()

	for id, session := range s.sessions {
		if session.Archived && !session.ExpiresAt.IsZero() && !session.ExpiresAt.After(now) {
			expiredSessionIDs = append(expiredSessionIDs, id)
			delete(s.sessions, id)
			delete(s.messages, id)
			delete(s.summaries, id)
			if s.currentSession == id {
				s.currentSession = ""
			}
		}
	}
	s.mu.Unlock()

	for _, sessionID := range expiredSessionIDs {
		if err := s.handleVectorStoreError(s.deleteVectorRowsBySession(ctx, sessionID)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) Unarchive(_ context.Context, id string) (Session, error) {
	if s == nil {
		return Session{}, errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, errors.New("session not found")
	}

	session, err := base.ClearSessionArchive(session)
	if err != nil {
		return Session{}, err
	}

	s.sessions[id] = session
	return session, nil
}

func (s *Store) ClearMessages(ctx context.Context, id string) error {
	if s == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return err
	}

	s.mu.Lock()

	session, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("session not found")
	}

	delete(s.messages, id)
	delete(s.summaries, id)
	session.Compaction = base.SessionCompaction{}
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	s.mu.Unlock()

	return s.handleVectorStoreError(s.deleteVectorRowsBySession(ctx, id))
}

func (s *Store) SaveSummary(_ context.Context, summary SessionSummary) error {
	if s == nil {
		return errors.New("store is required")
	}

	normalized, err := base.NormalizeSessionSummary(summary)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[normalized.SessionID]; !ok {
		return errors.New("session not found")
	}

	s.summaries[normalized.SessionID] = base.CloneSessionSummary(normalized)
	return nil
}

func (s *Store) GetSummary(_ context.Context, sessionID string) (SessionSummary, bool, error) {
	if s == nil {
		return SessionSummary{}, false, errors.New("store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionSummary{}, false, nil
	}

	if err := base.ValidateSessionID(sessionID); err != nil {
		return SessionSummary{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	summary, ok := s.summaries[sessionID]
	if !ok {
		return SessionSummary{}, false, nil
	}

	return base.CloneSessionSummary(summary), true, nil
}

func (s *Store) DeleteSummary(_ context.Context, sessionID string) error {
	if s == nil {
		return errors.New("store is required")
	}

	sessionID = strings.TrimSpace(sessionID)
	if err := base.ValidateSessionID(sessionID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.summaries, sessionID)
	return nil
}

func (s *Store) SetCurrent(_ context.Context, id string) error {
	if s == nil {
		return errors.New("store is required")
	}

	id = strings.TrimSpace(id)
	if err := base.ValidateSessionID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok || session.Archived {
		return errors.New("session not found")
	}

	s.currentSession = id
	return nil
}

func (s *Store) Current(_ context.Context) (string, bool, error) {
	if s == nil {
		return "", false, errors.New("store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if strings.TrimSpace(s.currentSession) == "" {
		return "", false, nil
	}

	return s.currentSession, true, nil
}

func (s *Store) ClearCurrent(_ context.Context) error {
	if s == nil {
		return errors.New("store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentSession = ""
	return nil
}

func cloneMessages(messages []handmsg.Message) []handmsg.Message {
	return base.CloneMessages(messages)
}

func cloneSearchMessageHits(hits []base.SearchMessageHit) []base.SearchMessageHit {
	if len(hits) == 0 {
		return nil
	}

	cloned := make([]base.SearchMessageHit, len(hits))
	for i, hit := range hits {
		cloned[i] = hit
		cloned[i].Message = base.CloneMessages([]handmsg.Message{hit.Message})[0]
	}

	return cloned
}

func cloneSearchMessageResults(results []base.SearchMessageResult) []base.SearchMessageResult {
	if len(results) == 0 {
		return nil
	}

	cloned := make([]base.SearchMessageResult, len(results))
	for i, result := range results {
		cloned[i] = result
		cloned[i].Messages = cloneSearchMessageHits(result.Messages)
	}

	return cloned
}

func getMessagesForQuery(messages []handmsg.Message, opts MessageQueryOptions) []handmsg.Message {
	filtered := filterMessages(messages, opts)
	if getMessageQueryOrder(opts) == base.MessageOrderDesc {
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

func getMessageQueryOrder(opts MessageQueryOptions) string {
	order, err := base.NormalizeMessageQueryOrder(opts.Order)
	if err != nil {
		return base.MessageOrderAsc
	}

	return order
}

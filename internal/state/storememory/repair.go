package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/messages"
	state "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
)

func (s *Store) RepairVectorStore(
	ctx context.Context,
	opts search.VectorRepairOptions,
) (search.VectorRepairResult, error) {
	if s == nil {
		return search.VectorRepairResult{}, errors.New("store is required")
	}
	if s.vectors == nil {
		return search.VectorRepairResult{}, nil
	}

	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID != "" {
		if err := state.ValidateSessionID(sessionID); err != nil {
			return search.VectorRepairResult{}, err
		}
	}

	lister, err := search.RequireVectorRecordLister(s.vectors.Store)
	if err != nil {
		return search.VectorRepairResult{}, err
	}

	batchSize := opts.BatchSize
	if batchSize < 0 {
		return search.VectorRepairResult{}, errors.New("vector repair batch size must be greater than or equal to zero")
	}
	if batchSize == 0 {
		batchSize = search.DefaultVectorRepairBatchSize
	}

	sessions, err := s.repairSessionIDs(sessionID)
	if err != nil {
		return search.VectorRepairResult{}, err
	}

	var result search.VectorRepairResult
	result.SessionsScanned = len(sessions)
	for _, id := range sessions {
		messages := s.repairMessages(id)
		for start := 0; start < len(messages); start += batchSize {
			end := min(start+batchSize, len(messages))
			batch := messages[start:end]
			batchResult, err := s.repairVectorBatch(ctx, lister, id, batch, opts.Full)
			result.Add(batchResult)
			if err != nil {
				if requiredErr := s.handleVectorStoreError(err); requiredErr != nil {
					return result, requiredErr
				}
			}
		}
	}

	return result, nil
}

func (s *Store) repairSessionIDs(sessionID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if sessionID != "" {
		if _, ok := s.sessions[sessionID]; !ok {
			return nil, errors.New("session not found")
		}
		return []string{sessionID}, nil
	}

	sessionIDs := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)

	return sessionIDs, nil
}

func (s *Store) repairMessages(sessionID string) []messages.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneMessages(s.messages[sessionID])
}

func (s *Store) repairVectorBatch(
	ctx context.Context,
	lister search.VectorRecordLister,
	sessionID string,
	messages []messages.Message,
	full bool,
) (search.VectorRepairResult, error) {
	var result search.VectorRepairResult
	if len(messages) == 0 {
		return result, nil
	}

	rows := make([]search.MessageIndexRow, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, search.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	inputs := search.VectorInputsFromIndexRows(rows)
	result.MessagesScanned = len(messages)
	result.RowsScanned = len(inputs)
	if len(inputs) == 0 {
		return result, nil
	}

	dirtySources, batchResult, err := search.DirtyVectorSources(
		ctx,
		lister,
		s.vectors.Model,
		inputs,
		full,
	)
	result.Add(batchResult)
	if err != nil || len(dirtySources) == 0 {
		return result, err
	}

	dirtyMessages := search.MessagesBySourceID(sessionID, messages, dirtySources)
	records, err := s.vectorRecordsForMessages(ctx, sessionID, dirtyMessages)
	if err != nil {
		return result, err
	}

	deleteErr := s.deleteVectorRows(ctx, dirtySources)
	if err := s.upsertVectorRecords(ctx, records); err != nil {
		return result, err
	}

	dirtyRows := make([]search.MessageIndexRow, 0, len(dirtyMessages))
	for _, message := range dirtyMessages {
		dirtyRows = append(dirtyRows, search.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	result.DeletedSources = len(dirtySources)
	result.RebuiltRows = len(search.VectorInputsFromIndexRows(dirtyRows))
	result.Batches = 1

	return result, deleteErr
}

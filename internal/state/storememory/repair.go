package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state"
	"github.com/wandxy/hand/internal/state/retrieval"
)

func (s *Store) RepairVectorStore(
	ctx context.Context,
	opts state.VectorRepairOptions,
) (state.VectorRepairResult, error) {
	if s == nil {
		return state.VectorRepairResult{}, errors.New("store is required")
	}
	if s.vectors == nil {
		return state.VectorRepairResult{}, nil
	}

	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID != "" {
		if err := state.ValidateSessionID(sessionID); err != nil {
			return state.VectorRepairResult{}, err
		}
	}

	lister, err := state.VectorRecordLister(s.vectors.Store)
	if err != nil {
		return state.VectorRepairResult{}, err
	}

	batchSize := opts.BatchSize
	if batchSize < 0 {
		return state.VectorRepairResult{}, errors.New("vector repair batch size must be greater than or equal to zero")
	}
	if batchSize == 0 {
		batchSize = state.DefaultVectorRepairBatchSize
	}

	sessions, err := s.repairSessionIDs(sessionID)
	if err != nil {
		return state.VectorRepairResult{}, err
	}

	var result state.VectorRepairResult
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
	lister retrieval.VectorRecordLister,
	sessionID string,
	messages []messages.Message,
	full bool,
) (state.VectorRepairResult, error) {
	var result state.VectorRepairResult
	if len(messages) == 0 {
		return result, nil
	}

	rows := make([]state.MessageIndexRow, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, state.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	inputs := state.VectorInputsFromIndexRows(rows)
	result.MessagesScanned = len(messages)
	result.RowsScanned = len(inputs)
	if len(inputs) == 0 {
		return result, nil
	}

	dirtySources, batchResult, err := state.DirtyVectorSources(
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

	dirtyMessages := state.MessagesBySourceID(sessionID, messages, dirtySources)
	records, err := s.vectorRecordsForMessages(ctx, sessionID, dirtyMessages)
	if err != nil {
		return result, err
	}

	deleteErr := s.deleteVectorRows(ctx, dirtySources)
	if err := s.upsertVectorRecords(ctx, records); err != nil {
		return result, err
	}

	dirtyRows := make([]state.MessageIndexRow, 0, len(dirtyMessages))
	for _, message := range dirtyMessages {
		dirtyRows = append(dirtyRows, state.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	result.DeletedSources = len(dirtySources)
	result.RebuiltRows = len(state.VectorInputsFromIndexRows(dirtyRows))
	result.Batches = 1

	return result, deleteErr
}

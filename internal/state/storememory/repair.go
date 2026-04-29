package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/messages"
	state "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/indexing"
	"github.com/wandxy/hand/internal/state/retrieval"
	statevector "github.com/wandxy/hand/internal/state/vector"
)

func (s *Store) RepairVectorStore(
	ctx context.Context,
	opts statevector.VectorRepairOptions,
) (statevector.VectorRepairResult, error) {
	if s == nil {
		return statevector.VectorRepairResult{}, errors.New("store is required")
	}
	if s.vectors == nil {
		return statevector.VectorRepairResult{}, nil
	}

	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID != "" {
		if err := state.ValidateSessionID(sessionID); err != nil {
			return statevector.VectorRepairResult{}, err
		}
	}

	lister, err := statevector.VectorRecordLister(s.vectors.Store)
	if err != nil {
		return statevector.VectorRepairResult{}, err
	}

	batchSize := opts.BatchSize
	if batchSize < 0 {
		return statevector.VectorRepairResult{}, errors.New("vector repair batch size must be greater than or equal to zero")
	}
	if batchSize == 0 {
		batchSize = statevector.DefaultVectorRepairBatchSize
	}

	sessions, err := s.repairSessionIDs(sessionID)
	if err != nil {
		return statevector.VectorRepairResult{}, err
	}

	var result statevector.VectorRepairResult
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
) (statevector.VectorRepairResult, error) {
	var result statevector.VectorRepairResult
	if len(messages) == 0 {
		return result, nil
	}

	rows := make([]indexing.MessageIndexRow, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, indexing.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	inputs := statevector.VectorInputsFromIndexRows(rows)
	result.MessagesScanned = len(messages)
	result.RowsScanned = len(inputs)
	if len(inputs) == 0 {
		return result, nil
	}

	dirtySources, batchResult, err := statevector.DirtyVectorSources(
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

	dirtyMessages := statevector.MessagesBySourceID(sessionID, messages, dirtySources)
	records, err := s.vectorRecordsForMessages(ctx, sessionID, dirtyMessages)
	if err != nil {
		return result, err
	}

	deleteErr := s.deleteVectorRows(ctx, dirtySources)
	if err := s.upsertVectorRecords(ctx, records); err != nil {
		return result, err
	}

	dirtyRows := make([]indexing.MessageIndexRow, 0, len(dirtyMessages))
	for _, message := range dirtyMessages {
		dirtyRows = append(dirtyRows, indexing.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	result.DeletedSources = len(dirtySources)
	result.RebuiltRows = len(statevector.VectorInputsFromIndexRows(dirtyRows))
	result.Batches = 1

	return result, deleteErr
}

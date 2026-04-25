package retrieval

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type SourceKind string

const (
	SourceKindSessionMessage SourceKind = "session_message"
	SourceKindMemoryItem     SourceKind = "memory_item"
)

type Candidate struct {
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Metadata     map[string]string
	ID           string
	SourceKind   SourceKind
	SessionID    string
	MemoryID     string
	Text         string
	LexicalScore float64
	VectorScore  float64
	FusedScore   float64
	MessageID    uint
}

type ScoreDirection int

const (
	ScoreHigherIsBetter ScoreDirection = iota
	ScoreLowerIsBetter
)

func ValidateCandidate(candidate Candidate) error {
	if strings.TrimSpace(candidate.ID) == "" {
		return errors.New("candidate id is required")
	}
	if strings.TrimSpace(string(candidate.SourceKind)) == "" {
		return errors.New("candidate source kind is required")
	}
	if strings.TrimSpace(candidate.Text) == "" {
		return errors.New("candidate text is required")
	}
	if !finite(candidate.LexicalScore) {
		return errors.New("candidate lexical score must be finite")
	}
	if !finite(candidate.VectorScore) {
		return errors.New("candidate vector score must be finite")
	}
	if !finite(candidate.FusedScore) {
		return errors.New("candidate fused score must be finite")
	}

	switch candidate.SourceKind {
	case SourceKindSessionMessage:
		if strings.TrimSpace(candidate.SessionID) == "" {
			return errors.New("candidate session id is required")
		}
		if candidate.MessageID == 0 {
			return errors.New("candidate message id is required")
		}
	case SourceKindMemoryItem:
		if strings.TrimSpace(candidate.MemoryID) == "" {
			return errors.New("candidate memory id is required")
		}
	default:
		return fmt.Errorf("candidate source kind %q is not supported", candidate.SourceKind)
	}

	return nil
}

func StableSessionMessageID(sessionID string, messageID uint) string {
	return fmt.Sprintf("%s:%s:%d", SourceKindSessionMessage, strings.TrimSpace(sessionID), messageID)
}

func StableMemoryItemID(memoryID string) string {
	return fmt.Sprintf("%s:%s", SourceKindMemoryItem, strings.TrimSpace(memoryID))
}

func SortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i int, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.FusedScore != right.FusedScore {
			return left.FusedScore > right.FusedScore
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		if left.SourceKind != right.SourceKind {
			return left.SourceKind < right.SourceKind
		}

		return left.ID < right.ID
	})
}

func NormalizeScores(scores []float64, direction ScoreDirection) ([]float64, error) {
	if len(scores) == 0 {
		return nil, nil
	}
	if direction != ScoreHigherIsBetter && direction != ScoreLowerIsBetter {
		return nil, errors.New("score direction is not supported")
	}

	minScore := scores[0]
	maxScore := scores[0]
	for _, score := range scores {
		if !finite(score) {
			return nil, errors.New("score must be finite")
		}
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	normalized := make([]float64, len(scores))
	if minScore == maxScore {
		for idx := range normalized {
			normalized[idx] = 1
		}
		return normalized, nil
	}

	spread := maxScore - minScore
	for idx, score := range scores {
		switch direction {
		case ScoreHigherIsBetter:
			normalized[idx] = (score - minScore) / spread
		case ScoreLowerIsBetter:
			normalized[idx] = (maxScore - score) / spread
		}
	}

	return normalized, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

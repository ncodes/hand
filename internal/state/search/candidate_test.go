package search

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateCandidate(t *testing.T) {
	validSessionCandidate := Candidate{
		ID:         StableSessionMessageID("ses_test", 12),
		SourceKind: SourceKindSessionMessage,
		SessionID:  "ses_test",
		MessageID:  12,
		Text:       "searchable text",
	}
	require.NoError(t, ValidateCandidate(validSessionCandidate))

	validMemoryCandidate := Candidate{
		ID:         StableMemoryItemID("mem_test"),
		SourceKind: SourceKindMemoryItem,
		MemoryID:   "mem_test",
		Text:       "memory text",
	}
	require.NoError(t, ValidateCandidate(validMemoryCandidate))

	tests := []struct {
		candidate Candidate
		name      string
		want      string
	}{
		{
			name:      "missing id",
			candidate: Candidate{SourceKind: SourceKindSessionMessage, SessionID: "ses_test", MessageID: 1, Text: "text"},
			want:      "candidate id is required",
		},
		{
			name:      "missing source kind",
			candidate: Candidate{ID: "candidate", Text: "text"},
			want:      "candidate source kind is required",
		},
		{
			name:      "missing text",
			candidate: Candidate{ID: "candidate", SourceKind: SourceKindSessionMessage, SessionID: "ses_test", MessageID: 1},
			want:      "candidate text is required",
		},
		{
			name: "non-finite lexical score",
			candidate: Candidate{
				ID:           "candidate",
				SourceKind:   SourceKindSessionMessage,
				SessionID:    "ses_test",
				MessageID:    1,
				Text:         "text",
				LexicalScore: math.NaN(),
			},
			want: "candidate lexical score must be finite",
		},
		{
			name: "non-finite vector score",
			candidate: Candidate{
				ID:          "candidate",
				SourceKind:  SourceKindSessionMessage,
				SessionID:   "ses_test",
				MessageID:   1,
				Text:        "text",
				VectorScore: math.Inf(1),
			},
			want: "candidate vector score must be finite",
		},
		{
			name: "non-finite fused score",
			candidate: Candidate{
				ID:         "candidate",
				SourceKind: SourceKindSessionMessage,
				SessionID:  "ses_test",
				MessageID:  1,
				Text:       "text",
				FusedScore: math.Inf(-1),
			},
			want: "candidate fused score must be finite",
		},
		{
			name:      "missing session id",
			candidate: Candidate{ID: "candidate", SourceKind: SourceKindSessionMessage, MessageID: 1, Text: "text"},
			want:      "candidate session id is required",
		},
		{
			name:      "missing message id",
			candidate: Candidate{ID: "candidate", SourceKind: SourceKindSessionMessage, SessionID: "ses_test", Text: "text"},
			want:      "candidate message id is required",
		},
		{
			name:      "missing memory id",
			candidate: Candidate{ID: "candidate", SourceKind: SourceKindMemoryItem, Text: "text"},
			want:      "candidate memory id is required",
		},
		{
			name:      "unsupported source kind",
			candidate: Candidate{ID: "candidate", SourceKind: SourceKind("other"), Text: "text"},
			want:      `candidate source kind "other" is not supported`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCandidate(tt.candidate)
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestSortCandidatesDeterministically(t *testing.T) {
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{ID: "candidate-c", SourceKind: SourceKindSessionMessage, FusedScore: 0.5, UpdatedAt: now, CreatedAt: now},
		{ID: "candidate-b", SourceKind: SourceKindSessionMessage, FusedScore: 0.8, UpdatedAt: now, CreatedAt: now},
		{ID: "candidate-a", SourceKind: SourceKindSessionMessage, FusedScore: 0.8, UpdatedAt: now.Add(time.Second), CreatedAt: now},
		{ID: "candidate-d", SourceKind: SourceKindMemoryItem, FusedScore: 0.8, UpdatedAt: now, CreatedAt: now},
		{ID: "candidate-e", SourceKind: SourceKindSessionMessage, FusedScore: 0.8, UpdatedAt: now, CreatedAt: now.Add(time.Second)},
	}

	SortCandidates(candidates)

	require.Equal(t, []string{
		"candidate-a",
		"candidate-e",
		"candidate-d",
		"candidate-b",
		"candidate-c",
	}, []string{
		candidates[0].ID,
		candidates[1].ID,
		candidates[2].ID,
		candidates[3].ID,
		candidates[4].ID,
	})
}

func TestSortCandidatesUsesIDAsFinalTieBreaker(t *testing.T) {
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{ID: "candidate-b", SourceKind: SourceKindSessionMessage, FusedScore: 0.8, UpdatedAt: now, CreatedAt: now},
		{ID: "candidate-a", SourceKind: SourceKindSessionMessage, FusedScore: 0.8, UpdatedAt: now, CreatedAt: now},
	}

	SortCandidates(candidates)

	require.Equal(t, []string{"candidate-a", "candidate-b"}, []string{
		candidates[0].ID,
		candidates[1].ID,
	})
}

func TestNormalizeScores(t *testing.T) {
	normalized, err := NormalizeScores([]float64{-3, -2, -1}, ScoreLowerIsBetter)
	require.NoError(t, err)
	require.Equal(t, []float64{1, 0.5, 0}, normalized)

	normalized, err = NormalizeScores([]float64{0.6, 0.2, 1.0}, ScoreHigherIsBetter)
	require.NoError(t, err)
	require.InDeltaSlice(t, []float64{0.5, 0, 1}, normalized, 0.000000001)

	normalized, err = NormalizeScores([]float64{7, 7}, ScoreLowerIsBetter)
	require.NoError(t, err)
	require.Equal(t, []float64{1, 1}, normalized)

	normalized, err = NormalizeScores(nil, ScoreHigherIsBetter)
	require.NoError(t, err)
	require.Nil(t, normalized)

	_, err = NormalizeScores([]float64{math.Inf(1)}, ScoreHigherIsBetter)
	require.EqualError(t, err, "score must be finite")

	_, err = NormalizeScores([]float64{1}, ScoreDirection(99))
	require.EqualError(t, err, "score direction is not supported")
}

package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EmbeddingProvider interface {
	Embed(context.Context, EmbeddingRequest) (EmbeddingResult, error)
}

type EmbeddingRequest struct {
	Model  string
	Inputs []EmbeddingInput
}

type EmbeddingInput struct {
	ID         string
	Text       string
	SourceKind SourceKind
}

type EmbeddingResult struct {
	Model      string
	Items      []Embedding
	Dimensions int
}

type Embedding struct {
	ID          string
	ContentHash string
	Vector      []float64
}

type VectorStore interface {
	Upsert(context.Context, []VectorRecord) error
	Delete(context.Context, VectorDeleteRequest) error
	Search(context.Context, VectorSearchRequest) (VectorSearchResult, error)
	Metadata(context.Context) (VectorStoreMetadata, error)
}

type VectorRecord struct {
	ID             string
	SourceKind     SourceKind
	SourceID       string
	EmbeddingModel string
	ContentHash    string
	Vector         []float64
	Dimensions     int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type VectorDeleteRequest struct {
	SourceKind SourceKind
	SourceID   string
}

type VectorSearchRequest struct {
	Filter         VectorFilter
	QueryVector    []float64
	EmbeddingModel string
	Limit          int
	Dimensions     int
}

type VectorFilter struct {
	SourceKind SourceKind
	SourceIDs  []string
}

type VectorSearchResult struct {
	Matches []VectorSearchMatch
}

type VectorSearchMatch struct {
	Record VectorRecord
	Score  float64
}

type VectorStoreMetadata struct {
	Models []VectorModelMetadata
}

type VectorModelMetadata struct {
	Model      string
	Dimensions int
	Count      int
}

func ValidateEmbeddingRequest(req EmbeddingRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return errors.New("embedding model is required")
	}

	if len(req.Inputs) == 0 {
		return errors.New("embedding inputs are required")
	}

	seenIDs := make(map[string]struct{}, len(req.Inputs))
	for _, input := range req.Inputs {
		inputID := strings.TrimSpace(input.ID)
		if inputID == "" {
			return errors.New("embedding input id is required")
		}
		if inputID != input.ID {
			return errors.New("embedding input id must be trimmed")
		}
		if _, ok := seenIDs[inputID]; ok {
			return fmt.Errorf("embedding input id %q is duplicated", inputID)
		}
		seenIDs[inputID] = struct{}{}
		if err := validateOptionalSourceKind(input.SourceKind, "embedding input source kind"); err != nil {
			return err
		}
		if strings.TrimSpace(input.Text) == "" {
			return errors.New("embedding input text is required")
		}
	}

	return nil
}

func ValidateEmbeddingResult(req EmbeddingRequest, result EmbeddingResult) error {
	if err := ValidateEmbeddingRequest(req); err != nil {
		return err
	}
	if strings.TrimSpace(result.Model) == "" {
		return errors.New("embedding result model is required")
	}
	if strings.TrimSpace(result.Model) != strings.TrimSpace(req.Model) {
		return errors.New("embedding result model must match request model")
	}
	if result.Dimensions <= 0 {
		return errors.New("embedding result dimensions must be greater than zero")
	}
	if len(result.Items) != len(req.Inputs) {
		return errors.New("embedding result count must match input count")
	}
	inputHashes := make(map[string]string, len(req.Inputs))
	for _, input := range req.Inputs {
		inputHashes[input.ID] = VectorContentHash(input.Text)
	}
	seenIDs := make(map[string]struct{}, len(result.Items))
	for _, item := range result.Items {
		itemID := strings.TrimSpace(item.ID)
		if itemID == "" {
			return errors.New("embedding id is required")
		}
		if itemID != item.ID {
			return errors.New("embedding id must be trimmed")
		}
		expectedHash, ok := inputHashes[itemID]
		if !ok {
			return fmt.Errorf("embedding id %q is unknown", itemID)
		}
		if _, ok := seenIDs[itemID]; ok {
			return fmt.Errorf("embedding id %q is duplicated", itemID)
		}
		seenIDs[itemID] = struct{}{}
		if len(item.Vector) != result.Dimensions {
			return errors.New("embedding vector dimensions do not match result dimensions")
		}
		for _, value := range item.Vector {
			if !finite(value) {
				return errors.New("embedding vector value must be finite")
			}
		}
		if strings.TrimSpace(item.ContentHash) == "" {
			return errors.New("embedding content hash is required")
		}
		if item.ContentHash != expectedHash {
			return errors.New("embedding content hash must match input text")
		}
	}

	return nil
}

func ValidateVectorRecord(record VectorRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("vector id is required")
	}
	if err := validateRequiredSourceKind(record.SourceKind, "vector source kind"); err != nil {
		return err
	}
	if strings.TrimSpace(record.SourceID) == "" {
		return errors.New("vector source id is required")
	}
	if strings.TrimSpace(record.EmbeddingModel) == "" {
		return errors.New("vector embedding model is required")
	}
	if record.Dimensions <= 0 {
		return errors.New("vector dimensions must be greater than zero")
	}
	if len(record.Vector) != record.Dimensions {
		return errors.New("vector length must match dimensions")
	}
	for _, value := range record.Vector {
		if !finite(value) {
			return errors.New("vector value must be finite")
		}
	}
	if strings.TrimSpace(record.ContentHash) == "" {
		return errors.New("vector content hash is required")
	}

	return nil
}

func ValidateVectorSearchRequest(req VectorSearchRequest) error {
	if strings.TrimSpace(req.EmbeddingModel) == "" {
		return errors.New("vector search embedding model is required")
	}
	if err := validateOptionalSourceKind(req.Filter.SourceKind, "vector search source kind"); err != nil {
		return err
	}
	if req.Limit <= 0 {
		return errors.New("vector search limit must be greater than zero")
	}
	if req.Dimensions <= 0 {
		return errors.New("vector search dimensions must be greater than zero")
	}
	if len(req.QueryVector) != req.Dimensions {
		return errors.New("vector search query length must match dimensions")
	}
	for _, value := range req.QueryVector {
		if !finite(value) {
			return errors.New("vector search query value must be finite")
		}
	}

	return nil
}

func ValidateVectorDeleteRequest(req VectorDeleteRequest) error {
	if err := validateRequiredSourceKind(req.SourceKind, "source kind"); err != nil {
		return err
	}
	if strings.TrimSpace(req.SourceID) == "" {
		return errors.New("source id is required")
	}

	return nil
}

func VectorContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func IsVectorRecordStale(record VectorRecord, text string) bool {
	return record.ContentHash != VectorContentHash(text)
}

func validateRequiredSourceKind(sourceKind SourceKind, field string) error {
	if strings.TrimSpace(string(sourceKind)) == "" {
		return fmt.Errorf("%s is required", field)
	}

	return validateOptionalSourceKind(sourceKind, field)
}

func validateOptionalSourceKind(sourceKind SourceKind, field string) error {
	if strings.TrimSpace(string(sourceKind)) == "" {
		return nil
	}
	switch sourceKind {
	case SourceKindSessionMessage, SourceKindMemoryItem:
		return nil
	default:
		return fmt.Errorf("%s %q is not supported", field, sourceKind)
	}
}

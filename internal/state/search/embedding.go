package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Embedder interface {
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

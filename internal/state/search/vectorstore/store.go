package vectorstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type Store interface {
	Upsert(context.Context, []Record) error
	Delete(context.Context, DeleteRequest) error
	Search(context.Context, SearchRequest) (SearchResult, error)
	Metadata(context.Context) (StoreMetadata, error)
}

type RecordLister interface {
	List(context.Context, ListRequest) (ListResult, error)
}

type Record struct {
	ID             string
	SourceKind     SourceKind
	SourceID       string
	SessionID      string
	Role           string
	ToolName       string
	Tags           []string
	EmbeddingModel string
	ContentHash    string
	Vector         []float64
	Dimensions     int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DeleteRequest struct {
	SourceKind SourceKind
	SourceIDs  []string
}

type SearchRequest struct {
	Filter         Filter
	QueryVector    []float64
	EmbeddingModel string
	Limit          int
	Dimensions     int
}

type Filter struct {
	SourceKind      SourceKind
	SourceIDs       []string
	SessionID       string
	IgnoreSessionID string
	Role            string
	ToolName        string
	Tags            []string
	TagGroups       [][]string
}

type SearchResult struct {
	Matches []SearchMatch
}

type ListRequest struct {
	Filter         Filter
	EmbeddingModel string
}

type ListResult struct {
	Records []Record
}

type SearchMatch struct {
	Record Record
	Score  float64
}

type StoreMetadata struct {
	Models []ModelMetadata
}

type ModelMetadata struct {
	Model      string
	Dimensions int
	Count      int
}

func ValidateRecord(record Record) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("vector id is required")
	}
	if err := ValidateRequiredSourceKind(record.SourceKind, "vector source kind"); err != nil {
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
	if err := validateTags(record.Tags, "vector tag"); err != nil {
		return err
	}

	return nil
}

func ValidateSearchRequest(req SearchRequest) error {
	if strings.TrimSpace(req.EmbeddingModel) == "" {
		return errors.New("vector search embedding model is required")
	}
	if err := ValidateOptionalSourceKind(req.Filter.SourceKind, "vector search filter source kind"); err != nil {
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
	if err := validateTags(req.Filter.Tags, "vector search filter tag"); err != nil {
		return err
	}
	if err := validateTagGroups(req.Filter.TagGroups, "vector search filter tag group"); err != nil {
		return err
	}

	return nil
}

func ValidateListRequest(req ListRequest) error {
	if strings.TrimSpace(req.EmbeddingModel) == "" {
		return errors.New("vector list embedding model is required")
	}
	if err := ValidateOptionalSourceKind(req.Filter.SourceKind, "vector list filter source kind"); err != nil {
		return err
	}
	for _, sourceID := range req.Filter.SourceIDs {
		if strings.TrimSpace(sourceID) == "" {
			return errors.New("vector list filter source id is required")
		}
		if strings.TrimSpace(sourceID) != sourceID {
			return errors.New("vector list filter source id must be trimmed")
		}
	}
	if err := validateTags(req.Filter.Tags, "vector list filter tag"); err != nil {
		return err
	}
	if err := validateTagGroups(req.Filter.TagGroups, "vector list filter tag group"); err != nil {
		return err
	}

	return nil
}

func ValidateDeleteRequest(req DeleteRequest) error {
	if err := ValidateRequiredSourceKind(req.SourceKind, "source kind"); err != nil {
		return err
	}
	if len(req.SourceIDs) == 0 {
		return errors.New("source id is required")
	}
	for _, sourceID := range req.SourceIDs {
		if strings.TrimSpace(sourceID) == "" {
			return errors.New("source id is required")
		}
		if strings.TrimSpace(sourceID) != sourceID {
			return errors.New("source id must be trimmed")
		}
	}

	return nil
}

func ContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func IsRecordStale(record Record, text string) bool {
	return record.ContentHash != ContentHash(text)
}

func NormalizeTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized
}

func NormalizeTagGroups(groups [][]string) [][]string {
	normalized := make([][]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		tags := NormalizeTags(group)
		if len(tags) == 0 {
			continue
		}
		key := strings.Join(tags, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, tags)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return strings.Join(normalized[i], "\x00") < strings.Join(normalized[j], "\x00")
	})
	return normalized
}

func ValidateRequiredSourceKind(sourceKind SourceKind, field string) error {
	if strings.TrimSpace(string(sourceKind)) == "" {
		return fmt.Errorf("%s is required", field)
	}

	return ValidateOptionalSourceKind(sourceKind, field)
}

func ValidateOptionalSourceKind(sourceKind SourceKind, field string) error {
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

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validateTags(tags []string, field string) error {
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("%s is required", field)
		}
		if strings.TrimSpace(tag) != tag {
			return fmt.Errorf("%s must be trimmed", field)
		}
		if strings.ToLower(tag) != tag {
			return fmt.Errorf("%s must be lowercase", field)
		}
	}

	return nil
}

func validateTagGroups(groups [][]string, field string) error {
	for _, group := range groups {
		if len(group) == 0 {
			return fmt.Errorf("%s is required", field)
		}
		if err := validateTags(group, field+" tag"); err != nil {
			return err
		}
	}

	return nil
}

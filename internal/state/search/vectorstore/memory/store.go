package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"sync"

	"github.com/coder/hnsw"

	"github.com/wandxy/hand/internal/state/search/vectorstore"
)

type Record = vectorstore.Record
type DeleteRequest = vectorstore.DeleteRequest
type SearchRequest = vectorstore.SearchRequest
type SearchResult = vectorstore.SearchResult
type SearchMatch = vectorstore.SearchMatch
type ListRequest = vectorstore.ListRequest
type ListResult = vectorstore.ListResult
type Filter = vectorstore.Filter
type StoreMetadata = vectorstore.StoreMetadata
type ModelMetadata = vectorstore.ModelMetadata
type SourceKind = vectorstore.SourceKind

const SourceKindSessionMessage = vectorstore.SourceKindSessionMessage
const SourceKindMemoryItem = vectorstore.SourceKindMemoryItem

type Store struct {
	mu      sync.RWMutex
	records map[string]Record
	indexes map[indexKey]*hnsw.Graph[string]
}

func NewStore() *Store {
	return &Store{
		records: make(map[string]Record),
		indexes: make(map[indexKey]*hnsw.Graph[string]),
	}
}

func (s *Store) Upsert(_ context.Context, records []Record) error {
	if s == nil {
		return errors.New("vector store is required")
	}
	if len(records) == 0 {
		return nil
	}

	cloned := make([]Record, 0, len(records))
	for _, record := range records {
		if err := vectorstore.ValidateRecord(record); err != nil {
			return err
		}
		if _, err := float32Vector(record.Vector); err != nil {
			return err
		}
		cloned = append(cloned, cloneRecord(record))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.records == nil {
		s.records = make(map[string]Record)
	}
	if s.indexes == nil {
		s.indexes = make(map[indexKey]*hnsw.Graph[string])
	}
	for _, record := range cloned {
		if existing, ok := s.records[record.ID]; ok {
			s.removeFromIndex(existing)
		}
		s.records[record.ID] = record
		s.addToIndex(record)
	}

	return nil
}

func (s *Store) Delete(_ context.Context, req DeleteRequest) error {
	if s == nil {
		return errors.New("vector store is required")
	}
	if err := vectorstore.ValidateDeleteRequest(req); err != nil {
		return err
	}

	sourceIDs := sourceIDSet(req.SourceIDs)
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, record := range s.records {
		if record.SourceKind == req.SourceKind {
			if _, ok := sourceIDs[record.SourceID]; ok {
				delete(s.records, id)
				s.removeFromIndex(record)
			}
		}
	}

	return nil
}

func (s *Store) Search(_ context.Context, req SearchRequest) (SearchResult, error) {
	if s == nil {
		return SearchResult{}, errors.New("vector store is required")
	}
	if err := vectorstore.ValidateSearchRequest(req); err != nil {
		return SearchResult{}, err
	}
	if strings.TrimSpace(string(req.Filter.SourceKind)) == "" {
		return SearchResult{}, errors.New("vector search filter source kind is required")
	}
	if err := validateSearchSourceIDs(req.Filter.SourceIDs); err != nil {
		return SearchResult{}, err
	}
	queryVector, err := float32Vector(req.QueryVector)
	if err != nil {
		return SearchResult{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	filter := searchFilter{
		embeddingModel:  strings.TrimSpace(req.EmbeddingModel),
		dimensions:      req.Dimensions,
		sourceKind:      req.Filter.SourceKind,
		sourceIDs:       sourceIDSet(req.Filter.SourceIDs),
		sessionID:       strings.TrimSpace(req.Filter.SessionID),
		ignoreSessionID: strings.TrimSpace(req.Filter.IgnoreSessionID),
		role:            strings.TrimSpace(req.Filter.Role),
		toolName:        strings.TrimSpace(req.Filter.ToolName),
	}

	graph, records := s.searchGraph(filter)
	if graph == nil || graph.Len() == 0 {
		return SearchResult{}, nil
	}

	nodes := graph.Search(queryVector, min(req.Limit, graph.Len()))
	matches := make([]SearchMatch, 0, len(nodes))
	for _, node := range nodes {
		record := records[node.Key]
		matches = append(matches, SearchMatch{
			Record: cloneRecord(record),
			Score:  scoreFromDistance(cosineDistance(queryVector, node.Value)),
		})
	}
	slices.SortStableFunc(matches, compareMatches)
	return SearchResult{Matches: matches}, nil
}

func (s *Store) Metadata(context.Context) (StoreMetadata, error) {
	if s == nil {
		return StoreMetadata{}, errors.New("vector store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]ModelMetadata)
	for _, record := range s.records {
		key := modelMetadataKey(record.EmbeddingModel, record.Dimensions)
		metadata := counts[key]
		metadata.Model = record.EmbeddingModel
		metadata.Dimensions = record.Dimensions
		metadata.Count++
		counts[key] = metadata
	}

	models := make([]ModelMetadata, 0, len(counts))
	for _, metadata := range counts {
		models = append(models, metadata)
	}
	slices.SortStableFunc(models, compareModelMetadata)

	return StoreMetadata{Models: models}, nil
}

func (s *Store) List(_ context.Context, req ListRequest) (ListResult, error) {
	if s == nil {
		return ListResult{}, errors.New("vector store is required")
	}
	if err := vectorstore.ValidateListRequest(req); err != nil {
		return ListResult{}, err
	}

	filter := searchFilter{
		embeddingModel:  strings.TrimSpace(req.EmbeddingModel),
		sourceKind:      req.Filter.SourceKind,
		sourceIDs:       sourceIDSet(req.Filter.SourceIDs),
		sessionID:       strings.TrimSpace(req.Filter.SessionID),
		ignoreSessionID: strings.TrimSpace(req.Filter.IgnoreSessionID),
		role:            strings.TrimSpace(req.Filter.Role),
		toolName:        strings.TrimSpace(req.Filter.ToolName),
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]Record, 0)
	for _, record := range s.records {
		if recordMatchesList(record, filter) {
			records = append(records, cloneRecord(record))
		}
	}
	slices.SortStableFunc(records, compareRecords)

	return ListResult{Records: records}, nil
}

type searchFilter struct {
	embeddingModel  string
	dimensions      int
	sourceKind      SourceKind
	sourceIDs       map[string]struct{}
	sessionID       string
	ignoreSessionID string
	role            string
	toolName        string
}

type indexKey struct {
	model      string
	dimensions int
	sourceKind SourceKind
}

func (f searchFilter) indexKey() indexKey {
	return indexKey{
		model:      f.embeddingModel,
		dimensions: f.dimensions,
		sourceKind: f.sourceKind,
	}
}

func (f searchFilter) usesOnlyIndexFilters() bool {
	return len(f.sourceIDs) == 0 &&
		f.sessionID == "" &&
		f.ignoreSessionID == "" &&
		f.role == "" &&
		f.toolName == ""
}

func (s *Store) searchGraph(filter searchFilter) (*hnsw.Graph[string], map[string]Record) {
	if filter.usesOnlyIndexFilters() {
		return s.indexes[filter.indexKey()], s.records
	}

	records := make([]Record, 0)
	for _, record := range s.records {
		if recordMatchesSearch(record, filter) {
			records = append(records, record)
		}
	}
	if len(records) == 0 {
		return nil, nil
	}
	slices.SortStableFunc(records, compareRecords)

	graph := newGraph()
	recordsByID := make(map[string]Record, len(records))
	for _, record := range records {
		vector, _ := float32Vector(record.Vector)
		recordsByID[record.ID] = record
		graph.Add(hnsw.MakeNode(record.ID, vector))
	}

	return graph, recordsByID
}

func recordMatchesSearch(record Record, filter searchFilter) bool {
	if record.EmbeddingModel != filter.embeddingModel {
		return false
	}
	if record.Dimensions != filter.dimensions {
		return false
	}
	if record.SourceKind != filter.sourceKind {
		return false
	}
	if len(filter.sourceIDs) > 0 {
		if _, ok := filter.sourceIDs[record.SourceID]; !ok {
			return false
		}
	}
	if filter.sessionID != "" && record.SessionID != filter.sessionID {
		return false
	}
	if filter.ignoreSessionID != "" && record.SessionID == filter.ignoreSessionID {
		return false
	}
	if filter.role != "" && record.Role != filter.role {
		return false
	}
	if filter.toolName != "" && record.ToolName != filter.toolName {
		return false
	}
	return true
}

func recordMatchesList(record Record, filter searchFilter) bool {
	if record.EmbeddingModel != filter.embeddingModel {
		return false
	}
	if filter.sourceKind != "" && record.SourceKind != filter.sourceKind {
		return false
	}
	if len(filter.sourceIDs) > 0 {
		if _, ok := filter.sourceIDs[record.SourceID]; !ok {
			return false
		}
	}
	if filter.sessionID != "" && record.SessionID != filter.sessionID {
		return false
	}
	if filter.ignoreSessionID != "" && record.SessionID == filter.ignoreSessionID {
		return false
	}
	if filter.role != "" && record.Role != filter.role {
		return false
	}
	if filter.toolName != "" && record.ToolName != filter.toolName {
		return false
	}

	return true
}

func (s *Store) addToIndex(record Record) {
	vector, _ := float32Vector(record.Vector)
	key := indexKey{
		model:      strings.TrimSpace(record.EmbeddingModel),
		dimensions: record.Dimensions,
		sourceKind: record.SourceKind,
	}
	graph := s.indexes[key]
	if graph == nil {
		graph = newGraph()
		s.indexes[key] = graph
	}
	graph.Add(hnsw.MakeNode(record.ID, vector))
}

func (s *Store) removeFromIndex(record Record) {
	key := indexKey{
		model:      strings.TrimSpace(record.EmbeddingModel),
		dimensions: record.Dimensions,
		sourceKind: record.SourceKind,
	}
	graph := s.indexes[key]
	if graph == nil {
		return
	}
	graph.Delete(record.ID)
	if graph.Len() == 0 {
		delete(s.indexes, key)
	}
}

func validateSearchSourceIDs(sourceIDs []string) error {
	for _, sourceID := range sourceIDs {
		trimmed := strings.TrimSpace(sourceID)
		if trimmed == "" {
			return errors.New("vector search filter source id is required")
		}
		if trimmed != sourceID {
			return errors.New("vector search filter source id must be trimmed")
		}
	}

	return nil
}

func sourceIDSet(sourceIDs []string) map[string]struct{} {
	set := make(map[string]struct{}, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID != "" {
			set[sourceID] = struct{}{}
		}
	}
	return set
}

func newGraph() *hnsw.Graph[string] {
	graph := hnsw.NewGraph[string]()
	graph.Distance = cosineDistance
	graph.Rng = rand.New(rand.NewSource(1))

	return graph
}

func float32Vector(values []float64) ([]float32, error) {
	vector := make([]float32, len(values))
	for idx, value := range values {
		converted := float32(value)
		if math.IsInf(float64(converted), 0) {
			return nil, errors.New("vector value exceeds float32 range")
		}
		vector[idx] = converted
	}

	return vector, nil
}

func cosineDistance(left []float32, right []float32) float32 {
	distance := hnsw.CosineDistance(left, right)
	if math.IsNaN(float64(distance)) || math.IsInf(float64(distance), 0) {
		return 1
	}

	return distance
}

func scoreFromDistance(distance float32) float64 {
	return 1 - float64(distance)
}

func compareMatches(left SearchMatch, right SearchMatch) int {
	if left.Score > right.Score {
		return -1
	}
	if left.Score < right.Score {
		return 1
	}
	return strings.Compare(left.Record.ID, right.Record.ID)
}

func compareModelMetadata(left ModelMetadata, right ModelMetadata) int {
	if left.Model != right.Model {
		return strings.Compare(left.Model, right.Model)
	}
	if left.Dimensions < right.Dimensions {
		return -1
	}
	if left.Dimensions > right.Dimensions {
		return 1
	}
	return 0
}

func compareRecords(left Record, right Record) int {
	return strings.Compare(left.ID, right.ID)
}

func cloneRecord(record Record) Record {
	record.Vector = append([]float64(nil), record.Vector...)
	return record
}

func modelMetadataKey(model string, dimensions int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(model), dimensions)
}

var _ vectorstore.Store = (*Store)(nil)

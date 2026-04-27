package sqlite

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wandxy/hand/internal/storage/retrieval"
)

const recordsTable = "vector_records"

type Record = retrieval.VectorRecord
type DeleteRequest = retrieval.VectorDeleteRequest
type SearchRequest = retrieval.VectorSearchRequest
type SearchResult = retrieval.VectorSearchResult
type SearchMatch = retrieval.VectorSearchMatch
type Filter = retrieval.VectorFilter
type StoreMetadata = retrieval.VectorStoreMetadata
type ModelMetadata = retrieval.VectorModelMetadata
type SourceKind = retrieval.SourceKind

const SourceKindSessionMessage = retrieval.SourceKindSessionMessage
const SourceKindMemoryItem = retrieval.SourceKindMemoryItem

func init() {
	sqlitevec.Auto()
}

type Store struct {
	db *gorm.DB
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("vector sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create vector db directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open vector db: %w", err)
	}

	return NewStoreFromDB(db)
}

func NewStoreFromDB(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("vector db is required")
	}
	if err := ensureSQLiteStorage(db); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Upsert(ctx context.Context, records []Record) error {
	if s == nil || s.db == nil {
		return errors.New("vector store is required")
	}
	if len(records) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ensuredDimensions := make(map[int]struct{}, len(records))
		for _, record := range records {
			if err := retrieval.ValidateVectorRecord(record); err != nil {
				return err
			}
			if _, ok := ensuredDimensions[record.Dimensions]; !ok {
				if err := ensureIndexTable(tx, record.Dimensions); err != nil {
					return err
				}
				ensuredDimensions[record.Dimensions] = struct{}{}
			}
			if err := upsertRecord(tx, record); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) Delete(ctx context.Context, req DeleteRequest) error {
	if s == nil || s.db == nil {
		return errors.New("vector store is required")
	}
	if err := retrieval.ValidateVectorDeleteRequest(req); err != nil {
		return err
	}
	sourceIDs := normalizeDeleteSourceIDs(req)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := vectorRowsForSources(tx, req.SourceKind, sourceIDs)
		if err != nil {
			return err
		}
		rowIDsByDimension := make(map[int][]int64, len(rows))
		for _, row := range rows {
			rowIDsByDimension[row.Dimensions] = append(rowIDsByDimension[row.Dimensions], row.RowID)
		}
		for dimensions, rowIDs := range rowIDsByDimension {
			if err := deleteIndexRows(tx, dimensions, rowIDs); err != nil {
				return err
			}
		}
		if err := tx.Exec(
			`DELETE FROM `+recordsTable+` WHERE source_kind = ? AND source_id IN ?`,
			req.SourceKind,
			sourceIDs,
		).Error; err != nil {
			return fmt.Errorf("failed to delete vector records: %w", err)
		}

		return nil
	})
}

func (s *Store) Search(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if s == nil || s.db == nil {
		return SearchResult{}, errors.New("vector store is required")
	}
	if err := retrieval.ValidateVectorSearchRequest(req); err != nil {
		return SearchResult{}, err
	}
	if strings.TrimSpace(string(req.Filter.SourceKind)) == "" {
		return SearchResult{}, errors.New("vector search filter source kind is required")
	}
	if err := validateSearchSourceIDs(req.Filter.SourceIDs); err != nil {
		return SearchResult{}, err
	}

	queryBlob, err := serialize(req.QueryVector)
	if err != nil {
		return SearchResult{}, err
	}

	db := s.db.WithContext(ctx)
	exists, err := indexTableExists(db, req.Dimensions)
	if err != nil {
		return SearchResult{}, err
	}
	if !exists {
		return SearchResult{}, nil
	}

	var sqlText strings.Builder
	sqlText.WriteString(`SELECT
	rv.vector_rowid,
	rv.id,
	rv.source_kind,
	rv.source_id,
	rv.session_id,
	rv.role,
	rv.tool_name,
	rv.embedding_model,
	rv.dimensions,
	rv.content_hash,
	rv.vector,
	rv.created_at,
	rv.updated_at,
	vec.distance
FROM ` + indexTableName(req.Dimensions) + ` AS vec
JOIN ` + recordsTable + ` AS rv ON rv.vector_rowid = vec.rowid
WHERE vec.vector MATCH ?
	AND k = ?
	AND vec.source_kind = ?
	AND vec.embedding_model = ?`)
	args := []any{
		queryBlob,
		req.Limit,
		string(req.Filter.SourceKind),
		strings.TrimSpace(req.EmbeddingModel),
	}
	if len(req.Filter.SourceIDs) == 1 {
		sqlText.WriteString(`
	AND vec.source_id = ?`)
		args = append(args, req.Filter.SourceIDs[0])
	} else if len(req.Filter.SourceIDs) > 1 {
		sqlText.WriteString(`
	AND (`)
		for idx, sourceID := range req.Filter.SourceIDs {
			if idx > 0 {
				sqlText.WriteString(` OR `)
			}
			sqlText.WriteString(`vec.source_id = ?`)
			args = append(args, sourceID)
		}
		sqlText.WriteString(`)`)
	}
	if sessionID := strings.TrimSpace(req.Filter.SessionID); sessionID != "" {
		sqlText.WriteString(`
	AND vec.session_id = ?`)
		args = append(args, sessionID)
	}
	if ignoreSessionID := strings.TrimSpace(req.Filter.IgnoreSessionID); ignoreSessionID != "" {
		sqlText.WriteString(`
	AND vec.session_id <> ?`)
		args = append(args, ignoreSessionID)
	}
	if role := strings.TrimSpace(req.Filter.Role); role != "" {
		sqlText.WriteString(`
	AND vec.role = ?`)
		args = append(args, role)
	}
	if toolName := strings.TrimSpace(req.Filter.ToolName); toolName != "" {
		sqlText.WriteString(`
	AND vec.tool_name = ?`)
		args = append(args, toolName)
	}
	sqlText.WriteString(`
ORDER BY vec.distance ASC, rv.id ASC`)

	var rows []searchRow
	if err := db.Raw(sqlText.String(), args...).Scan(&rows).Error; err != nil {
		return SearchResult{}, fmt.Errorf("failed to search vectors: %w", err)
	}

	matches := make([]SearchMatch, 0, len(rows))
	for _, row := range rows {
		record, err := row.record()
		if err != nil {
			return SearchResult{}, err
		}
		matches = append(matches, SearchMatch{
			Record: record,
			Score:  1 - row.Distance,
		})
	}

	return SearchResult{Matches: matches}, nil
}

func (s *Store) Metadata(ctx context.Context) (StoreMetadata, error) {
	if s == nil || s.db == nil {
		return StoreMetadata{}, errors.New("vector store is required")
	}

	var models []ModelMetadata
	if err := s.db.WithContext(ctx).Raw(`SELECT
	embedding_model AS model,
	dimensions,
	COUNT(*) AS count
FROM ` + recordsTable + `
GROUP BY embedding_model, dimensions
ORDER BY embedding_model ASC, dimensions ASC`).Scan(&models).Error; err != nil {
		return StoreMetadata{}, fmt.Errorf("failed to load vector metadata: %w", err)
	}

	return StoreMetadata{Models: models}, nil
}

func ensureSQLiteStorage(db *gorm.DB) error {
	if db == nil {
		return errors.New("vector db is required")
	}
	if err := db.Exec(`SELECT vec_version()`).Error; err != nil {
		return fmt.Errorf("sqlite vector extension is required: %w", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS ` + recordsTable + ` (
	vector_rowid INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	source_kind TEXT NOT NULL,
	source_id TEXT NOT NULL,
	session_id TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	tool_name TEXT NOT NULL DEFAULT '',
	embedding_model TEXT NOT NULL,
	dimensions INTEGER NOT NULL,
	content_hash TEXT NOT NULL,
	vector BLOB NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
)`).Error; err != nil {
			return fmt.Errorf("failed to create vector records table: %w", err)
		}
		indexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_vectors_source ON ` + recordsTable + ` (source_kind, source_id)`,
			`CREATE INDEX IF NOT EXISTS idx_vectors_session ON ` + recordsTable + ` (source_kind, session_id)`,
			`CREATE INDEX IF NOT EXISTS idx_vectors_session_role_tool ON ` + recordsTable + ` (source_kind, session_id, role, tool_name)`,
			`CREATE INDEX IF NOT EXISTS idx_vectors_model_dimensions ON ` + recordsTable + ` (embedding_model, dimensions)`,
			`CREATE INDEX IF NOT EXISTS idx_vectors_content_hash ON ` + recordsTable + ` (content_hash)`,
		}
		for _, statement := range indexes {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("failed to create vector records index: %w", err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func upsertRecord(tx *gorm.DB, record Record) error {
	blob, err := serialize(record.Vector)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	createdAt := record.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := record.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = now
	}

	existing, ok, err := recordRef(tx, record.ID)
	if err != nil {
		return err
	}
	var rowID int64
	if ok {
		rowID = existing.RowID
		if err := deleteIndexRow(tx, existing.Dimensions, existing.RowID); err != nil {
			return err
		}

		if err := tx.Exec(`UPDATE `+recordsTable+` SET
	source_kind = ?,
	source_id = ?,
	session_id = ?,
	role = ?,
	tool_name = ?,
	embedding_model = ?,
	dimensions = ?,
	content_hash = ?,
	vector = ?,
	updated_at = ?
WHERE id = ?`,
			string(record.SourceKind),
			record.SourceID,
			strings.TrimSpace(record.SessionID),
			strings.TrimSpace(record.Role),
			strings.TrimSpace(record.ToolName),
			record.EmbeddingModel,
			record.Dimensions,
			record.ContentHash,
			blob,
			updatedAt,
			record.ID,
		).Error; err != nil {
			return fmt.Errorf("failed to update vector record: %w", err)
		}
	} else {
		if err := tx.Raw(`INSERT INTO `+recordsTable+` (
	id,
	source_kind,
	source_id,
	session_id,
	role,
	tool_name,
	embedding_model,
	dimensions,
	content_hash,
	vector,
	created_at,
	updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING vector_rowid`,
			record.ID,
			string(record.SourceKind),
			record.SourceID,
			strings.TrimSpace(record.SessionID),
			strings.TrimSpace(record.Role),
			strings.TrimSpace(record.ToolName),
			record.EmbeddingModel,
			record.Dimensions,
			record.ContentHash,
			blob,
			createdAt,
			updatedAt,
		).Scan(&rowID).Error; err != nil {
			return fmt.Errorf("failed to insert vector record: %w", err)
		}
	}

	if err := tx.Exec(
		`INSERT INTO `+
			indexTableName(record.Dimensions)+
			` (
				rowid,
				vector,
				source_kind,
				source_id,
				session_id,
				role,
				tool_name,
				embedding_model
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rowID,
		blob,
		string(record.SourceKind),
		record.SourceID,
		strings.TrimSpace(record.SessionID),
		strings.TrimSpace(record.Role),
		strings.TrimSpace(record.ToolName),
		record.EmbeddingModel,
	).Error; err != nil {
		return fmt.Errorf("failed to insert vector index row: %w", err)
	}

	return nil
}

func ensureIndexTable(db *gorm.DB, dimensions int) error {
	if dimensions <= 0 {
		return errors.New("vector dimensions must be greater than zero")
	}
	exists, err := indexTableExists(db, dimensions)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := db.Exec(`CREATE VIRTUAL TABLE ` + indexTableName(dimensions) + ` USING vec0(
	vector float[` + fmt.Sprintf("%d", dimensions) + `] distance_metric=cosine,
	source_kind TEXT,
	source_id TEXT,
	session_id TEXT,
	role TEXT,
	tool_name TEXT,
	embedding_model TEXT
)`).Error; err != nil {
		return fmt.Errorf("failed to create vector index table: %w", err)
	}

	return nil
}

func deleteIndexRow(tx *gorm.DB, dimensions int, rowID int64) error {
	if dimensions <= 0 || rowID <= 0 {
		return nil
	}

	return deleteIndexRows(tx, dimensions, []int64{rowID})
}

func deleteIndexRows(tx *gorm.DB, dimensions int, rowIDs []int64) error {
	if dimensions <= 0 || len(rowIDs) == 0 {
		return nil
	}
	if err := tx.Exec(`DELETE FROM `+indexTableName(dimensions)+` WHERE rowid IN ?`, rowIDs).Error; err != nil {
		return fmt.Errorf("failed to delete vector index row: %w", err)
	}

	return nil
}

func recordRef(tx *gorm.DB, id string) (recordRefRow, bool, error) {
	var row recordRefRow
	err := tx.Raw(
		`SELECT vector_rowid, dimensions FROM `+recordsTable+` WHERE id = ?`,
		id,
	).Scan(&row).Error
	if err != nil {
		return recordRefRow{}, false, fmt.Errorf("failed to load vector record ref: %w", err)
	}
	if row.RowID == 0 {
		return recordRefRow{}, false, nil
	}

	return row, true, nil
}

func vectorRowsForSources(tx *gorm.DB, sourceKind SourceKind, sourceIDs []string) ([]recordRefRow, error) {
	var rows []recordRefRow
	if err := tx.Raw(
		`SELECT vector_rowid, dimensions FROM `+recordsTable+` WHERE source_kind = ? AND source_id IN ?`,
		string(sourceKind),
		sourceIDs,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load vector record refs: %w", err)
	}

	return rows, nil
}

func normalizeDeleteSourceIDs(req DeleteRequest) []string {
	sourceIDs := make([]string, 0, len(req.SourceIDs))
	seen := make(map[string]struct{}, len(req.SourceIDs))
	for _, sourceID := range req.SourceIDs {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID != "" {
			if _, ok := seen[sourceID]; ok {
				continue
			}
			seen[sourceID] = struct{}{}
			sourceIDs = append(sourceIDs, sourceID)
		}
	}

	return sourceIDs
}

func serialize(values []float64) ([]byte, error) {
	blob := make([]byte, len(values)*4)
	for idx, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("vector value must be finite")
		}
		converted := float32(value)
		if math.IsInf(float64(converted), 0) {
			return nil, errors.New("vector value exceeds float32 range")
		}
		binary.LittleEndian.PutUint32(blob[idx*4:idx*4+4], math.Float32bits(converted))
	}

	return blob, nil
}

func deserialize(blob []byte, dimensions int) ([]float64, error) {
	if dimensions <= 0 {
		return nil, errors.New("vector dimensions must be greater than zero")
	}
	if len(blob) != dimensions*4 {
		return nil, errors.New("vector blob length must match dimensions")
	}

	vector := make([]float64, dimensions)
	for idx := range vector {
		bits := binary.LittleEndian.Uint32(blob[idx*4 : idx*4+4])
		value := math.Float32frombits(bits)
		vector[idx] = float64(value)
	}

	return vector, nil
}

func indexTableName(dimensions int) string {
	return fmt.Sprintf("vector_index_%d", dimensions)
}

func indexTableExists(db *gorm.DB, dimensions int) (bool, error) {
	if dimensions <= 0 {
		return false, errors.New("vector dimensions must be greater than zero")
	}

	var count int
	if err := db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		indexTableName(dimensions),
	).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check vector index table: %w", err)
	}

	return count > 0, nil
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

type recordRefRow struct {
	RowID      int64 `gorm:"column:vector_rowid"`
	Dimensions int   `gorm:"column:dimensions"`
}

type searchRow struct {
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	Vector         []byte    `gorm:"column:vector"`
	ID             string    `gorm:"column:id"`
	SourceKind     string    `gorm:"column:source_kind"`
	SourceID       string    `gorm:"column:source_id"`
	SessionID      string    `gorm:"column:session_id"`
	Role           string    `gorm:"column:role"`
	ToolName       string    `gorm:"column:tool_name"`
	EmbeddingModel string    `gorm:"column:embedding_model"`
	ContentHash    string    `gorm:"column:content_hash"`
	RowID          int64     `gorm:"column:vector_rowid"`
	Dimensions     int       `gorm:"column:dimensions"`
	Distance       float64   `gorm:"column:distance"`
}

func (r searchRow) record() (Record, error) {
	vector, err := deserialize(r.Vector, r.Dimensions)
	if err != nil {
		return Record{}, err
	}

	return Record{
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
		ID:             r.ID,
		SourceKind:     SourceKind(r.SourceKind),
		SourceID:       r.SourceID,
		SessionID:      r.SessionID,
		Role:           r.Role,
		ToolName:       r.ToolName,
		EmbeddingModel: r.EmbeddingModel,
		ContentHash:    r.ContentHash,
		Vector:         vector,
		Dimensions:     r.Dimensions,
	}, nil
}

var _ retrieval.VectorStore = (*Store)(nil)

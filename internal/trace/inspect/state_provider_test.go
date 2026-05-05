package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	statemock "github.com/wandxy/hand/internal/state/mock"
	handtrace "github.com/wandxy/hand/internal/trace"
)

func TestConfigureStateProvider_NoopsWhenMissingInputs(t *testing.T) {
	require.NoError(t, ConfigureStateProvider(nil, nil))

	cfg := &config.Config{}
	require.NoError(t, ConfigureStateProvider(cfg, nil))
}

func TestConfigureStateProvider_ReturnsStoreOpenError(t *testing.T) {
	dir := t.TempDir()
	writeTraceSession(t, dir, storage.DefaultSessionID)
	app := NewApp(dir)
	cfg := &config.Config{Storage: config.StorageConfig{Backend: "bad"}}

	err := ConfigureStateProvider(cfg, app)

	require.Error(t, err)
	require.Contains(t, err.Error(), "storage backend")
}

func TestConfigureStateProvider_ReturnsManagerSetupError(t *testing.T) {
	originalNewStateManager := newStateManager
	t.Cleanup(func() { newStateManager = originalNewStateManager })
	expected := errors.New("manager setup failed")
	newStateManager = func(
		storage.Store,
		time.Duration,
		time.Duration,
	) (*statemanager.Manager, error) {
		return nil, expected
	}

	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	dir := t.TempDir()
	writeTraceSession(t, dir, storage.DefaultSessionID)
	app := NewApp(dir)
	cfg := &config.Config{Storage: config.StorageConfig{Backend: "sqlite"}}

	err := ConfigureStateProvider(cfg, app)

	require.ErrorIs(t, err, expected)
}

func TestConfigureStateProvider_AttachesStateProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	dir := t.TempDir()
	writeTraceSession(t, dir, storage.DefaultSessionID)
	app := NewApp(dir)
	cfg := &config.Config{Storage: config.StorageConfig{Backend: "sqlite"}}

	require.NoError(t, ConfigureStateProvider(cfg, app))

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+storage.DefaultSessionID, nil)
	resp := httptest.NewRecorder()
	app.Handler().ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload struct {
		Memories struct {
			Source string `json:"source"`
		} `json:"memories"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Equal(t, "state", payload.Memories.Source)
}

func TestConfigureStateProvider_AttachesStateProviderWhenMemoryDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	dir := t.TempDir()
	writeTraceSession(t, dir, storage.DefaultSessionID)
	app := NewApp(dir)
	disabled := false
	cfg := &config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Memory:  config.MemoryConfig{Enabled: &disabled},
	}

	require.NoError(t, ConfigureStateProvider(cfg, app))

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+storage.DefaultSessionID, nil)
	resp := httptest.NewRecorder()
	app.Handler().ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload struct {
		Memories struct {
			Source string `json:"source"`
		} `json:"memories"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Equal(t, "state", payload.Memories.Source)
}

func TestTraceViewerStateConfig_DisablesSearchDependencies(t *testing.T) {
	rerankerEnabled := true
	cfg := &config.Config{
		Search: config.SearchConfig{
			Vector: config.SearchVectorConfig{Enabled: true},
		},
		Reranker: config.RerankerConfig{
			Enabled: &rerankerEnabled,
			Type:    "llm",
		},
	}

	stateCfg := traceViewerStateConfig(cfg)

	require.False(t, stateCfg.Search.Vector.Enabled)
	require.NotNil(t, stateCfg.Reranker.Enabled)
	require.False(t, *stateCfg.Reranker.Enabled)
	require.True(t, cfg.Search.Vector.Enabled)
	require.True(t, *cfg.Reranker.Enabled)
}

func TestStateSessionMemoryProvider_ListSessionMemoriesFiltersAndClones(t *testing.T) {
	searchResult := storage.MemorySearchResult{
		Hits: []storage.MemorySearchHit{
			{Item: storage.MemoryItem{
				ID:     "source-link",
				Kind:   storage.MemoryKindEpisodic,
				Status: storage.MemoryStatusCandidate,
				SourceLinks: []storage.MemorySourceLink{{
					SessionID: " " + storage.DefaultSessionID + " ",
					Offsets:   []int{1},
				}},
			}},
			{Item: storage.MemoryItem{
				ID:       "metadata",
				Kind:     storage.MemoryKindEpisodic,
				Status:   storage.MemoryStatusActive,
				Metadata: map[string]string{"source_session_id": storage.DefaultSessionID},
			}},
			{Item: storage.MemoryItem{
				ID:       "other",
				Kind:     storage.MemoryKindEpisodic,
				Status:   storage.MemoryStatusActive,
				Metadata: map[string]string{"source_session_id": "other"},
			}},
		},
	}

	manager, err := statemanager.NewManager(
		&memorySearchStore{result: searchResult},
		time.Hour,
		time.Hour,
	)
	require.NoError(t, err)

	items, err := stateProvider{manager: manager}.ListSessionMemories(
		context.Background(),
		storage.DefaultSessionID,
	)

	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "source-link", items[0].ID)
	require.Equal(t, "metadata", items[1].ID)
	items[0].SourceLinks[0].Offsets[0] = 99
	require.Equal(t, []int{1}, searchResult.Hits[0].Item.SourceLinks[0].Offsets)
}

func TestStateSessionMemoryProvider_ReturnsSearchErrors(t *testing.T) {
	expected := errors.New("memory search failed")
	manager, err := statemanager.NewManager(
		&memorySearchErrorStore{searchErr: expected},
		time.Hour,
		time.Hour,
	)
	require.NoError(t, err)

	_, err = stateProvider{manager: manager}.ListSessionMemories(
		context.Background(),
		storage.DefaultSessionID,
	)

	require.ErrorIs(t, err, expected)
}

func writeTraceSession(t *testing.T, dir, id string) {
	t.Helper()

	path := filepath.Join(dir, id+".jsonl")
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	require.NoError(t, json.NewEncoder(file).Encode(handtrace.Event{
		SessionID: id,
		Type:      handtrace.EvtChatStarted,
		Timestamp: time.Now().UTC(),
	}))
}

type memorySearchErrorStore struct {
	statemock.Store
	searchErr error
}

type memorySearchStore struct {
	statemock.Store
	result storage.MemorySearchResult
}

func (s memorySearchStore) SearchMemory(
	context.Context,
	storage.MemorySearchQuery,
) (storage.MemorySearchResult, error) {
	return s.result, nil
}

func (s memorySearchStore) UpsertMemory(
	context.Context,
	storage.MemoryItem,
) (storage.MemoryItem, error) {
	return storage.MemoryItem{}, nil
}

func (s memorySearchStore) DeleteMemory(
	context.Context,
	storage.MemoryDeleteRequest,
) error {
	return nil
}

func (s memorySearchErrorStore) SearchMemory(
	context.Context,
	storage.MemorySearchQuery,
) (storage.MemorySearchResult, error) {
	return storage.MemorySearchResult{}, s.searchErr
}

func (s memorySearchErrorStore) UpsertMemory(
	context.Context,
	storage.MemoryItem,
) (storage.MemoryItem, error) {
	return storage.MemoryItem{}, nil
}

func (s memorySearchErrorStore) DeleteMemory(
	context.Context,
	storage.MemoryDeleteRequest,
) error {
	return nil
}

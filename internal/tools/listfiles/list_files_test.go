package listfiles

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
	nativemocks "github.com/wandxy/morph/internal/tools/mocks"
)

func TestListFiles_ToolListsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden", "secret.txt"), []byte("secret"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":true}`})

	require.NoError(t, err)
	var payload struct {
		Root    string `json:"root"`
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, root, payload.Root)
	require.Equal(t, ".", payload.Path)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, int64(5), payload.Entries[0].Size)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
	require.Equal(t, int64(0), payload.Entries[1].Size)
}

func TestListFiles_FullAccessListsAbsolutePathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "alpha.txt"), []byte("alpha"), 0o644))
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Preset: permissions.PresetFullAccess},
		Definition,
	)

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "list_files",
		Input: `{"path":` + nativemocks.QuoteJSON(outside) + `,"recursive":false}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	var payload struct {
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, filepath.ToSlash(outside), payload.Path)
	require.Equal(t, []struct {
		Path string `json:"path"`
	}{{Path: "alpha.txt"}}, payload.Entries)
}

func TestListFiles_AskPresetListsAbsolutePathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "alpha.txt"), []byte("alpha"), 0o644))
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Preset: permissions.PresetAskForApproval},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceTUI,
	})

	result, err := registry.Invoke(ctx, tools.Call{
		Name:  "list_files",
		Input: `{"path":` + nativemocks.QuoteJSON(outside) + `,"recursive":false}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, []struct {
		Path string `json:"path"`
	}{{Path: "alpha.txt"}}, payload.Entries)
}

func TestListFiles_ToolListsDirectoryNonRecursivelyAndSkipsHiddenEntries(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".secret"), []byte("hidden"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "child", "beta.txt"), []byte("beta"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":false}`})

	require.NoError(t, err)
	var payload struct {
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, ".", payload.Path)
	require.Len(t, payload.Entries, 2)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
}

func TestListFiles_ToolDefaultsToNonRecursiveListing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "child", "beta.txt"), []byte("beta"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":"."}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 2)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
}

func TestListFiles_ToolIncludesHiddenEntriesWhenRequested(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden", "secret.txt"), []byte("secret"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":true,"include_hidden":true}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 2)
	require.Equal(t, ".hidden", payload.Entries[0].Path)
	require.Equal(t, "dir", payload.Entries[0].Type)
	require.Equal(t, ".hidden/secret.txt", payload.Entries[1].Path)
	require.Equal(t, "file", payload.Entries[1].Type)
}

func TestListFiles_ToolAppliesEntryLimit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "beta.txt"), []byte("beta"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":false,"max_entries":1}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 1)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
}

func TestListFiles_DefinitionDeclaresStrictRequiredSchema(t *testing.T) {
	root := t.TempDir()
	definition := Definition(nativemocks.NewRuntime(root, guardrails.CommandPolicy{}))

	required, ok := definition.InputSchema["required"].([]string)
	require.True(t, ok)
	require.Equal(t, []string{"path", "recursive", "include_hidden", "max_entries"}, required)
}

func TestListFiles_ResolvePermissionRejectsInvalidJSON(t *testing.T) {
	inputs, err := Definition(nil).ResolvePermission(context.Background(), tools.Call{Input: `{"path":`})

	require.Nil(t, inputs)
	require.EqualError(t, err, "invalid tool input")
}

func TestListFiles_HandlerReturnsInputAndPathErrors(t *testing.T) {
	root := t.TempDir()
	handler := Definition(nativemocks.NewRuntime(root, guardrails.CommandPolicy{})).Handler

	result, err := handler.Invoke(context.Background(), tools.Call{Input: `{"path":`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"invalid_input"`)

	outside := t.TempDir()
	result, err = handler.Invoke(context.Background(), tools.Call{
		Input: `{"path":` + nativemocks.QuoteJSON(outside) + `}`,
	})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"path_outside_roots"`)
}

func TestListFiles_HandlerReturnsDirectoryDependencyErrors(t *testing.T) {
	root := t.TempDir()
	handler := Definition(nativemocks.NewRuntime(root, guardrails.CommandPolicy{})).Handler
	originalReadDir := common.ReadDir
	originalWalkDir := common.WalkDir
	t.Cleanup(func() {
		common.ReadDir = originalReadDir
		common.WalkDir = originalWalkDir
	})

	common.ReadDir = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	result, err := handler.Invoke(context.Background(), tools.Call{Input: `{"path":"."}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"access_denied"`)

	common.ReadDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{dirEntryStub{name: "broken", infoErr: os.ErrPermission}}, nil
	}
	result, err = handler.Invoke(context.Background(), tools.Call{Input: `{"path":"."}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"access_denied"`)

	common.WalkDir = func(path string, walk fs.WalkDirFunc) error {
		return walk(filepath.Join(path, "broken"), nil, os.ErrPermission)
	}
	result, err = handler.Invoke(context.Background(), tools.Call{Input: `{"path":".","recursive":true}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"access_denied"`)

	common.WalkDir = func(path string, walk fs.WalkDirFunc) error {
		return walk(filepath.Join(path, "broken"), dirEntryStub{name: "broken", infoErr: os.ErrPermission}, nil)
	}
	result, err = handler.Invoke(context.Background(), tools.Call{Input: `{"path":".","recursive":true}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"access_denied"`)
}

func TestListFiles_HandlerHandlesRelativePathEdgeCases(t *testing.T) {
	root := t.TempDir()
	info, err := os.Stat(root)
	require.NoError(t, err)
	handler := Definition(nativemocks.NewRuntime(root, guardrails.CommandPolicy{})).Handler
	originalReadDir := common.ReadDir
	originalRelativePath := getRelativePath
	t.Cleanup(func() {
		common.ReadDir = originalReadDir
		getRelativePath = originalRelativePath
	})

	common.ReadDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{dirEntryStub{name: ".", directory: true, info: info}}, nil
	}
	result, err := handler.Invoke(context.Background(), tools.Call{Input: `{"path":"."}`})
	require.NoError(t, err)
	require.Empty(t, result.Error)
	var payload struct {
		Entries []any `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Empty(t, payload.Entries)

	common.ReadDir = originalReadDir
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0o644))
	getRelativePath = func(string, string) (string, error) { return "", errors.New("relative path failed") }
	result, err = handler.Invoke(context.Background(), tools.Call{Input: `{"path":"."}`})
	require.NoError(t, err)
	require.Empty(t, result.Error)
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Empty(t, payload.Entries)
}

func TestListFiles_RecursiveListingStopsAtEntryLimit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "beta.txt"), []byte("beta"), 0o644))
	handler := Definition(nativemocks.NewRuntime(root, guardrails.CommandPolicy{})).Handler

	result, err := handler.Invoke(context.Background(), tools.Call{
		Input: `{"path":".","recursive":true,"max_entries":1}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	var payload struct {
		Entries []any `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 1)
}

type dirEntryStub struct {
	name      string
	directory bool
	info      os.FileInfo
	infoErr   error
}

func (s dirEntryStub) Name() string               { return s.name }
func (s dirEntryStub) IsDir() bool                { return s.directory }
func (s dirEntryStub) Type() fs.FileMode          { return 0 }
func (s dirEntryStub) Info() (os.FileInfo, error) { return s.info, s.infoErr }

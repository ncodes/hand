package writefile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	nativemocks "github.com/wandxy/morph/internal/tools/mocks"
)

func TestWriteFile_ToolCreatesFile(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "write_file", Input: `{"path":"nested/file.txt","content":"hello"}`})

	require.NoError(t, err)
	var payload struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
		Created      bool   `json:"created"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "nested/file.txt", payload.Path)
	require.Equal(t, 5, payload.BytesWritten)
	require.True(t, payload.Created)
	content, readErr := os.ReadFile(filepath.Join(root, "nested", "file.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "hello", string(content))
}

func TestWriteFile_ToolOverwritesExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "write_file", Input: `{"path":"file.txt","content":"after"}`})

	require.NoError(t, err)
	var payload struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
		Created      bool   `json:"created"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "file.txt", payload.Path)
	require.Equal(t, 5, payload.BytesWritten)
	require.False(t, payload.Created)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "after", string(content))
}

func TestWriteFile_EnforcementDeniesBeforeFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Mode: permissions.ModeEnforce, Rules: []permissions.Rule{{
			Name: "deny writes", Tools: []string{"write_file"}, Decision: permissions.DecisionDeny,
		}}},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "write_file", Input: `{"path":"blocked.txt","content":"should not exist"}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, permissions.ErrorCodeDenied, toolErr.Code)
	_, statErr := os.Stat(filepath.Join(root, "blocked.txt"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestWriteFile_ResolvePermissionValidatesAndNormalizesTarget(t *testing.T) {
	resolver := Definition(nil).ResolvePermission

	path := filepath.Join("nested", "file.txt")
	inputs, err := resolver(context.Background(), tools.Call{Input: `{"path":` + nativemocks.QuoteJSON(path) + `}`})
	require.NoError(t, err)
	require.Equal(t, "nested/file.txt", inputs[0].Operation.Target)

	inputs, err = resolver(context.Background(), tools.Call{Input: `{"path":`})
	require.EqualError(t, err, "invalid tool input")
	require.Nil(t, inputs)

	inputs, err = resolver(context.Background(), tools.Call{Input: `{}`})
	require.EqualError(t, err, "path is required")
	require.Nil(t, inputs)
}

func TestWriteFile_ExplicitGrantDoesNotOverrideFilesystemRoots(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Mode: permissions.ModeEnforce, Rules: []permissions.Rule{{
			Name: "allow writes", Tools: []string{"write_file"}, Decision: permissions.DecisionAllow,
		}}},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "write_file", Input: `{"path":` + nativemocks.QuoteJSON(outside) + `,"content":"blocked"}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
	_, statErr := os.Stat(outside)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestWriteFile_EnforcementNormalizesTargetBeforeRuleMatching(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Mode: permissions.ModeEnforce, Rules: []permissions.Rule{
			{Name: "allow writes", Tools: []string{"write_file"}, Decision: permissions.DecisionAllow},
			{Name: "deny blocked file", TargetPrefixes: []string{"blocked.txt"}, Decision: permissions.DecisionDeny},
		}},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "write_file", Input: `{"path":"nested/../blocked.txt","content":"blocked"}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, permissions.ErrorCodeDenied, toolErr.Code)
	_, statErr := os.Stat(filepath.Join(root, "blocked.txt"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

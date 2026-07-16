package patch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	nativemocks "github.com/wandxy/morph/internal/tools/mocks"
)

func TestPatch_ToolAppliesUnifiedDiff(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello\nworld\n"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n hello\n-world\n+there\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var payload struct {
		AppliedFiles []string `json:"applied_files"`
		CreatedFiles []string `json:"created_files"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, []string{"file.txt"}, payload.AppliedFiles)
	require.Empty(t, payload.CreatedFiles)
	content, readErr := os.ReadFile(filepath.Join(root, "file.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "hello\nthere\n", string(content))
}

func TestPatch_EnforcementChecksEveryTargetBeforeMutation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "allowed.txt"), []byte("before\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "blocked.txt"), []byte("before\n"), 0o644))
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Mode: permissions.ModeEnforce, Rules: []permissions.Rule{
			{Name: "allow patches", Tools: []string{"patch"}, Decision: permissions.DecisionAllow},
			{Name: "deny blocked path", TargetPrefixes: []string{"blocked.txt"}, Decision: permissions.DecisionDeny},
		}},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})
	patch := "--- a/allowed.txt\n+++ b/allowed.txt\n@@ -1 +1 @@\n-before\n+after\n" +
		"--- a/blocked.txt\n+++ b/blocked.txt\n@@ -1 +1 @@\n-before\n+after\n"

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, permissions.ErrorCodeDenied, toolErr.Code)
	allowed, err := os.ReadFile(filepath.Join(root, "allowed.txt"))
	require.NoError(t, err)
	require.Equal(t, "before\n", string(allowed))
	blocked, err := os.ReadFile(filepath.Join(root, "blocked.txt"))
	require.NoError(t, err)
	require.Equal(t, "before\n", string(blocked))
}

func TestPatch_ResolvePermissionRejectsInputWithoutDiff(t *testing.T) {
	inputs, err := resolvePermission(context.Background(), tools.Call{Input: `{"patch":"not a diff"}`})

	require.EqualError(t, err, "invalid patch")
	require.Nil(t, inputs)
}

func TestPatch_EnforcementNormalizesTargetBeforeRuleMatching(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "blocked.txt"), []byte("before\n"), 0o644))
	registry := nativemocks.RegisterRuntimeWithPermissionPolicy(
		t,
		root,
		guardrails.CommandPolicy{},
		permissions.Policy{Mode: permissions.ModeEnforce, Rules: []permissions.Rule{
			{Name: "allow patches", Tools: []string{"patch"}, Decision: permissions.DecisionAllow},
			{Name: "deny blocked file", TargetPrefixes: []string{"blocked.txt"}, Decision: permissions.DecisionDeny},
		}},
		Definition,
	)
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})
	patch := "--- a/nested/../blocked.txt\n+++ b/nested/../blocked.txt\n@@ -1 +1 @@\n-before\n+after\n"

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, permissions.ErrorCodeDenied, toolErr.Code)
	content, readErr := os.ReadFile(filepath.Join(root, "blocked.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "before\n", string(content))
}

func TestPatch_ToolRejectsDeletePatch(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello\n"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- a/file.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-hello\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Contains(t, toolErr.Message, "delete patches are not supported")
}

func TestPatch_ToolCreatesFileFromDevNull(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,2 @@\n+alpha\n+beta\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var payload struct {
		AppliedFiles []string `json:"applied_files"`
		CreatedFiles []string `json:"created_files"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, []string{"new.txt"}, payload.AppliedFiles)
	require.Equal(t, []string{"new.txt"}, payload.CreatedFiles)
	content, readErr := os.ReadFile(filepath.Join(root, "new.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "alpha\nbeta\n", string(content))
}

func TestPatch_ToolUsesHunkPositionToChooseOccurrence(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("dup\nkeep\n\ndup\nchange\n"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- a/file.txt\n+++ b/file.txt\n@@ -4,2 +4,2 @@\n dup\n-change\n+updated\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	content, readErr := os.ReadFile(filepath.Join(root, "file.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "dup\nkeep\n\ndup\nupdated\n", string(content))
}

func TestPatch_ToolRejectsBlankPatch(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":"   "}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "patch is required", toolErr.Message)
}

func TestPatch_ToolRejectsInvalidJSONInput(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "invalid tool input", toolErr.Message)
}

func TestPatch_ToolReturnsConflictForNonApplyingHunk(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello\nworld\n"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n hello\n-mars\n+there\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "conflict", toolErr.Code)
	require.Contains(t, toolErr.Message, "patch conflict")
}

func TestPatch_ToolRejectsRenamePatch(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.txt"), []byte("hello\n"), 0o644))
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "diff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Contains(t, toolErr.Message, "rename")
}

func TestPatch_ToolRejectsOutsideAllowedRoots(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "--- /dev/null\n+++ ../../outside.txt\n@@ -0,0 +1 @@\n+hello\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
}

func TestPatch_ToolMapsMalformedPatchToInternalError(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	patch := "@@ -1 +1 @@\n-old\n+new\n"

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "patch", Input: `{"patch":` + nativemocks.QuoteJSON(patch) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "internal_error", toolErr.Code)
	require.NotEmpty(t, toolErr.Message)
}

func TestApplyUnifiedDiff_RejectsInvalidAndBinaryPatchKinds(t *testing.T) {
	root := t.TempDir()
	policy := guardrails.FilesystemPolicy{Roots: []string{root}}

	_, _, err := applyUnifiedDiff(context.Background(), policy, "not a patch", 0)
	require.EqualError(t, err, "invalid patch")

	binaryPatch := "diff --git a/file.bin b/file.bin\nindex 0000000..1111111\nBinary files a/file.bin and b/file.bin differ\n"
	_, _, err = applyUnifiedDiff(context.Background(), policy, binaryPatch, 0)
	require.EqualError(t, err, "binary patches are not supported")
}

func TestApplyUnifiedDiff_ReturnsSourceReadErrorForMissingExistingFile(t *testing.T) {
	root := t.TempDir()
	policy := guardrails.FilesystemPolicy{Roots: []string{root}}
	patch := "--- a/missing.txt\n+++ b/missing.txt\n@@ -1 +1 @@\n-old\n+new\n"

	_, _, err := applyUnifiedDiff(context.Background(), policy, patch, 0)

	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestApplyUnifiedDiff_ReturnsNonConflictApplyErrorForShortSource(t *testing.T) {
	root := t.TempDir()
	policy := guardrails.FilesystemPolicy{Roots: []string{root}}
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("line 1\n"), 0o644))
	patch := "--- a/file.txt\n+++ b/file.txt\n@@ -3 +3 @@\n-line 3\n+new line\n"

	_, _, err := applyUnifiedDiff(context.Background(), policy, patch, 0)

	require.Error(t, err)
	require.False(t, errors.Is(err, &gitdiff.Conflict{}))
}

func TestApplyUnifiedDiff_ReturnsMkdirErrorForNewFile(t *testing.T) {
	root := t.TempDir()
	policy := guardrails.FilesystemPolicy{Roots: []string{root}}
	originalMkdirAll := mkdirAll
	t.Cleanup(func() {
		mkdirAll = originalMkdirAll
	})
	mkdirAll = func(string, os.FileMode) error {
		return errors.New("mkdir failed")
	}

	_, _, err := applyUnifiedDiff(
		context.Background(),
		policy,
		"--- /dev/null\n+++ b/dir/new.txt\n@@ -0,0 +1 @@\n+hello\n",
		0,
	)

	require.EqualError(t, err, "mkdir failed")
}

func TestApplyUnifiedDiff_ReturnsWriteError(t *testing.T) {
	root := t.TempDir()
	policy := guardrails.FilesystemPolicy{Roots: []string{root}}
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello\n"), 0o644))
	originalWriteFile := writeFile
	t.Cleanup(func() {
		writeFile = originalWriteFile
	})
	writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("write failed")
	}

	_, _, err := applyUnifiedDiff(
		context.Background(),
		policy,
		"--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-hello\n+world\n",
		0,
	)

	require.EqualError(t, err, "write failed")
}

func TestStripPath_HandlesSpecialCases(t *testing.T) {
	require.Equal(t, "/dev/null", stripPath("/dev/null", 0))
	require.Equal(t, "file.txt", stripPath("a/dir/file.txt", 5))
	require.Equal(t, filepath.FromSlash("sub/file.txt"), stripPath("b/root/sub/file.txt", 1))
}

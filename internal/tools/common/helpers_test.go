package common_test

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
	common "github.com/wandxy/morph/internal/tools/common"
	listfiles "github.com/wandxy/morph/internal/tools/listfiles"
	nativemocks "github.com/wandxy/morph/internal/tools/mocks"
	patchtool "github.com/wandxy/morph/internal/tools/patch"
	plantool "github.com/wandxy/morph/internal/tools/plan"
	readfile "github.com/wandxy/morph/internal/tools/readfile"
	runcommand "github.com/wandxy/morph/internal/tools/runcommand"
	searchfiles "github.com/wandxy/morph/internal/tools/searchfiles"
	timetool "github.com/wandxy/morph/internal/tools/time"
	writefile "github.com/wandxy/morph/internal/tools/writefile"
)

func TestResolveFilesystemPath_EnforcesRootsWithoutFullAccess(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")

	_, err := common.ResolveFilesystemPath(
		context.Background(),
		guardrails.FilesystemPolicy{Roots: []string{root}},
		outside,
	)

	require.EqualError(t, err, "path is outside allowed roots")
}

func TestResolveFilesystemPath_AllowsOutsideRootsWithFullAccess(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	ctx := permissions.WithFullAccess(context.Background())

	resolved, err := common.ResolveFilesystemPath(
		ctx,
		guardrails.FilesystemPolicy{Roots: []string{root}},
		outside,
	)

	require.NoError(t, err)
	require.Equal(t, outside, resolved.Absolute)
	require.Equal(t, filepath.ToSlash(outside), resolved.Relative)
	require.Empty(t, resolved.Root)
}

func TestFileError_MapsKnownErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{name: "not found", err: os.ErrNotExist, code: "not_found", message: "path not found"},
		{name: "permission", err: os.ErrPermission, code: "access_denied", message: "access denied"},
		{name: "invalid", err: fs.ErrInvalid, code: "invalid_input", message: "path must be a file"},
		{name: "outside roots", err: errors.New("path is outside allowed roots"), code: "path_outside_roots", message: "path is outside allowed roots"},
		{name: "size limit", err: errors.New("file exceeds size limit"), code: "too_large", message: "file exceeds size limit"},
		{name: "not text", err: errors.New("file is not text"), code: "not_text", message: "file is not text"},
		{name: "directory", err: errors.New("is a directory"), code: "invalid_input", message: "path must be a file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := common.FileError(tt.err)

			require.Contains(t, result.Error, `"code":"`+tt.code+`"`)
			require.Contains(t, result.Error, `"message":"`+tt.message+`"`)
		})
	}
}

func TestFileError_HandlesNilError(t *testing.T) {
	require.Equal(t, tools.Result{}, common.FileError(nil))
}

func TestFileError_UsesInternalErrorFallback(t *testing.T) {
	result := common.FileError(errors.New("boom"))

	require.Contains(t, result.Error, `"code":"internal_error"`)
	require.Contains(t, result.Error, `"message":"boom"`)
}

func TestHiddenPath_DetectsHiddenSegments(t *testing.T) {
	require.True(t, common.HiddenPath(".git/config"))
	require.True(t, common.HiddenPath("dir/.env"))
	require.False(t, common.HiddenPath("dir/file.txt"))
	require.False(t, common.HiddenPath("dir/./file.txt"))
	require.False(t, common.HiddenPath("dir/../file.txt"))
}

func TestTrimOutput_ClampsToLimit(t *testing.T) {
	require.Equal(t, "abc", common.TrimOutput("abcdef", 3))
	require.Equal(t, "abc", common.TrimOutput("abc", 3))
}

func TestWithTimeoutSeconds_ClampsToSupportedRange(t *testing.T) {
	require.Equal(t, common.DefaultTimeout, common.WithTimeoutSeconds(0))
	require.Equal(t, common.MaxTimeout, common.WithTimeoutSeconds(common.MaxTimeout+1))
	require.Equal(t, 12, common.WithTimeoutSeconds(12))
}

func TestJoinStrings_JoinsNonEmptyParts(t *testing.T) {
	require.Equal(t, "first second third", common.JoinStrings("first", " ", "second", "", "third"))
}

func TestDecodeInput_UsesEmptyObjectWhenInputIsBlank(t *testing.T) {
	var target struct {
		Name string `json:"name"`
	}

	result := common.DecodeInput(tools.Call{Input: "   "}, &target)

	require.Equal(t, tools.Result{}, result)
	require.Equal(t, "", target.Name)
}

func TestDecodeInput_ReturnsStructuredErrorForInvalidJSON(t *testing.T) {
	var target map[string]any

	result := common.DecodeInput(tools.Call{Input: "{"}, &target)

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
}

func TestEncodeOutput_EncodesJSON(t *testing.T) {
	result, err := common.EncodeOutput(map[string]string{"message": "hello"})

	require.NoError(t, err)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, map[string]string{"message": "hello"}, payload)
}

func TestEncodeOutput_ReturnsMarshalError(t *testing.T) {
	_, err := common.EncodeOutput(map[string]any{"bad": make(chan int)})

	require.Error(t, err)
}

func TestNativeToolDefinitions_AdvertiseArgumentDescriptions(t *testing.T) {
	root := t.TempDir()
	runtime := nativemocks.NewRuntime(root, guardrails.CommandPolicy{})

	definitions := tools.Definitions{
		timetool.Definition(),
		listfiles.Definition(runtime),
		readfile.Definition(runtime),
		searchfiles.Definition(runtime),
		writefile.Definition(runtime),
		patchtool.Definition(runtime),
		plantool.Definition(runtime),
		runcommand.Definition(runtime),
	}

	for _, definition := range definitions {
		t.Run(definition.Name, func(t *testing.T) {
			require.Equal(t, "object", definition.InputSchema["type"])
			require.Equal(t, false, definition.InputSchema["additionalProperties"])

			properties, _ := definition.InputSchema["properties"].(map[string]any)
			if definition.Name == "time" {
				require.Empty(t, properties)
				return
			}

			require.NotEmpty(t, properties)
			for name, property := range properties {
				field, ok := property.(map[string]any)
				require.True(t, ok, name)
				require.NotEmpty(t, field["description"], name)
			}
		})
	}
}

func TestNativeFilesystemToolDefinitions_DoNotClaimWorkspaceOnlyAccess(t *testing.T) {
	root := t.TempDir()
	runtime := nativemocks.NewRuntime(root, guardrails.CommandPolicy{})
	definitions := tools.Definitions{
		listfiles.Definition(runtime),
		readfile.Definition(runtime),
		searchfiles.Definition(runtime),
		writefile.Definition(runtime),
		patchtool.Definition(runtime),
		runcommand.Definition(runtime),
	}

	for _, definition := range definitions {
		t.Run(definition.Name, func(t *testing.T) {
			require.NotContains(t, definition.Description, "allowed workspace root")

			properties, _ := definition.InputSchema["properties"].(map[string]any)
			for _, name := range []string{"path", "cwd"} {
				property, ok := properties[name].(map[string]any)
				if !ok {
					continue
				}
				require.NotContains(t, property["description"], "allowed workspace root")
			}
		})
	}
}

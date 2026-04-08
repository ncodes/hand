package native

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

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
			result := fileError(tt.err)

			require.Contains(t, result.Error, `"code":"`+tt.code+`"`)
			require.Contains(t, result.Error, `"message":"`+tt.message+`"`)
		})
	}
}

func TestFileError_HandlesNilError(t *testing.T) {
	require.Equal(t, tools.Result{}, fileError(nil))
}

func TestFileError_UsesInternalErrorFallback(t *testing.T) {
	result := fileError(errors.New("boom"))

	require.Contains(t, result.Error, `"code":"internal_error"`)
	require.Contains(t, result.Error, `"message":"boom"`)
}

func TestHiddenPath_DetectsHiddenSegments(t *testing.T) {
	require.True(t, hiddenPath(".git/config"))
	require.True(t, hiddenPath("dir/.env"))
	require.False(t, hiddenPath("dir/file.txt"))
	require.False(t, hiddenPath("dir/./file.txt"))
	require.False(t, hiddenPath("dir/../file.txt"))
}

func TestTrimOutput_ClampsToLimit(t *testing.T) {
	require.Equal(t, "abc", trimOutput("abcdef", 3))
	require.Equal(t, "abc", trimOutput("abc", 3))
}

func TestWithTimeoutSeconds_ClampsToSupportedRange(t *testing.T) {
	require.Equal(t, defaultTimeout, withTimeoutSeconds(0))
	require.Equal(t, maxTimeout, withTimeoutSeconds(maxTimeout+1))
	require.Equal(t, 12, withTimeoutSeconds(12))
}

func TestJoinStrings_JoinsNonEmptyParts(t *testing.T) {
	require.Equal(t, "first second third", joinStrings("first", " ", "second", "", "third"))
}

func TestDecodeInput_UsesEmptyObjectWhenInputIsBlank(t *testing.T) {
	var target struct {
		Name string `json:"name"`
	}

	result := decodeInput(tools.Call{Input: "   "}, &target)

	require.Equal(t, tools.Result{}, result)
	require.Equal(t, "", target.Name)
}

func TestDecodeInput_ReturnsStructuredErrorForInvalidJSON(t *testing.T) {
	var target map[string]any

	result := decodeInput(tools.Call{Input: "{"}, &target)

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
}

func TestEncodeOutput_EncodesJSON(t *testing.T) {
	result, err := encodeOutput(map[string]string{"message": "hello"})

	require.NoError(t, err)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, map[string]string{"message": "hello"}, payload)
}

func TestEncodeOutput_ReturnsMarshalError(t *testing.T) {
	_, err := encodeOutput(map[string]any{"bad": make(chan int)})

	require.Error(t, err)
}

func TestNativeToolDefinitions_AdvertiseArgumentDescriptions(t *testing.T) {
	root := t.TempDir()
	runtime := &testRuntime{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{root})},
		commandPolicy: guardrails.CommandPolicy{}.Normalize(),
	}

	definitions := []tools.Definition{
		TimeDefinition(),
		ListFilesDefinition(runtime),
		ReadFileDefinition(runtime),
		SearchFilesDefinition(runtime),
		WriteFileDefinition(runtime),
		PatchDefinition(runtime),
		PlanDefinition(runtime),
		RunCommandDefinition(runtime),
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

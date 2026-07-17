package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatAndCheckCommands(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	require.NoError(t, os.WriteFile(path, []byte("package sample\nfunc value(){return}\n"), 0o600))

	var output bytes.Buffer
	command := newCommand(&output)
	err := command.Run(context.Background(), []string{"goreadability", "check", root})
	require.Error(t, err)
	require.Contains(t, output.String(), path)

	output.Reset()
	command = newCommand(&output)
	require.NoError(t, command.Run(context.Background(), []string{"goreadability", "format", root}))
	require.Contains(t, output.String(), "1 Go file(s) checked; 1 changed")

	output.Reset()
	command = newCommand(&output)
	require.NoError(t, command.Run(context.Background(), []string{"goreadability", "check", root}))
	require.Contains(t, output.String(), "1 Go file(s) checked; 0 changed")
}

func TestGetSinglePath_DefaultsToWorkingDirectory(t *testing.T) {
	command := newLintCommand(nil)
	require.Equal(t, ".", getSinglePath(command))
}

package goreadability

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetLintCommands_BuildsBaselineAndOptionalLinters(t *testing.T) {
	commands := getLintCommands(LintOptions{
		GoBinary:     "/usr/local/bin/go",
		Tags:         "sqlite_fts5,integration",
		Staticcheck:  true,
		GolangCILint: true,
	})

	require.Equal(t, []lintCommand{
		{name: "/usr/local/bin/go", args: []string{"vet", "-tags", "sqlite_fts5,integration", "./..."}},
		{name: "staticcheck", args: []string{"-tags", "sqlite_fts5,integration", "./..."}},
		{name: "golangci-lint", args: []string{"run", "--build-tags", "sqlite_fts5,integration", "./..."}},
	}, commands)
}

func TestGetLintCommands_DefaultsToGoVet(t *testing.T) {
	require.Equal(t, []lintCommand{{name: "go", args: []string{"vet", "./..."}}}, getLintCommands(LintOptions{}))
}

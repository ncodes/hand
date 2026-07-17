package goreadability

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LintOptions struct {
	GoBinary     string
	Tags         string
	Staticcheck  bool
	GolangCILint bool
	Output       io.Writer
}

type lintCommand struct {
	name string
	args []string
}

func Lint(ctx context.Context, root string, options LintOptions) error {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve codebase path: %w", err)
	}

	if options.Output == nil {
		options.Output = os.Stdout
	}

	for _, command := range getLintCommands(options) {
		if _, err := exec.LookPath(command.name); err != nil {
			return fmt.Errorf("required linter %q is not installed: %w", command.name, err)
		}

		cmd := exec.CommandContext(ctx, command.name, command.args...)
		cmd.Dir = absoluteRoot
		cmd.Stdout = options.Output
		cmd.Stderr = options.Output
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", command.name, err)
		}
	}

	return nil
}

func getLintCommands(options LintOptions) []lintCommand {
	goBinary := strings.TrimSpace(options.GoBinary)
	if goBinary == "" {
		goBinary = "go"
	}

	vetArgs := []string{"vet"}
	if tags := strings.TrimSpace(options.Tags); tags != "" {
		vetArgs = append(vetArgs, "-tags", tags)
	}
	vetArgs = append(vetArgs, "./...")
	commands := []lintCommand{{name: goBinary, args: vetArgs}}
	if options.Staticcheck {
		args := make([]string, 0, 3)
		if tags := strings.TrimSpace(options.Tags); tags != "" {
			args = append(args, "-tags", tags)
		}
		args = append(args, "./...")
		commands = append(commands, lintCommand{name: "staticcheck", args: args})
	}
	if options.GolangCILint {
		args := []string{"run"}
		if tags := strings.TrimSpace(options.Tags); tags != "" {
			args = append(args, "--build-tags", tags)
		}
		args = append(args, "./...")
		commands = append(commands, lintCommand{name: "golangci-lint", args: args})
	}

	return commands
}

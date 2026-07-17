package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/morph/pkg/goreadability"
)

func main() {
	if err := newCommand(os.Stdout).Run(context.Background(), os.Args); err != nil {
		var exitCoder cli.ExitCoder
		if errors.As(err, &exitCoder) {
			fmt.Fprintln(os.Stderr, exitCoder.Error())
			os.Exit(exitCoder.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand(output io.Writer) *cli.Command {
	if output == nil {
		output = io.Discard
	}

	return &cli.Command{
		Name:  "goreadability",
		Usage: "Format, space, and lint a Go codebase",
		Commands: []*cli.Command{
			newFormatCommand(output, false),
			newFormatCommand(output, true),
			newLintCommand(output),
			newRunCommand(output),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newFormatCommand(output io.Writer, check bool) *cli.Command {
	name := "format"
	usage := "Apply gofmt and conservative semantic spacing"
	if check {
		name = "check"
		usage = "Report files that need gofmt or semantic spacing"
	}

	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "[path ...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "include-generated", Usage: "Include generated Go files"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			result, err := goreadability.FormatPaths(cmd.Args().Slice(), goreadability.FormatOptions{
				Write:            !check,
				IncludeGenerated: cmd.Bool("include-generated"),
			})
			if err != nil {
				return err
			}

			for _, path := range result.Changed {
				if _, err := fmt.Fprintln(output, path); err != nil {
					return err
				}
			}

			if check && len(result.Changed) > 0 {
				return fmt.Errorf("%d Go file(s) need readability formatting", len(result.Changed))
			}

			_, err = fmt.Fprintf(output, "%d Go file(s) checked; %d changed\n", result.Files, len(result.Changed))
			return err
		},
	}
}

func newLintCommand(output io.Writer) *cli.Command {
	return &cli.Command{
		Name:      "lint",
		Usage:     "Run go vet and optional external linters",
		ArgsUsage: "[path]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "go", Value: "go", Usage: "Go command to execute"},
			&cli.StringFlag{Name: "tags", Usage: "Comma-separated Go build tags"},
			&cli.BoolFlag{Name: "staticcheck", Usage: "Also run staticcheck"},
			&cli.BoolFlag{Name: "golangci-lint", Usage: "Also run golangci-lint"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLint(ctx, cmd, output, getSinglePath(cmd))
		},
	}
}

func newRunCommand(output io.Writer) *cli.Command {
	command := newLintCommand(output)
	command.Name = "run"
	command.Usage = "Apply readability formatting, then run linters"
	command.Flags = append(command.Flags,
		&cli.BoolFlag{Name: "include-generated", Usage: "Include generated Go files"},
	)
	command.Action = func(ctx context.Context, cmd *cli.Command) error {
		path := getSinglePath(cmd)
		result, err := goreadability.FormatPaths([]string{path}, goreadability.FormatOptions{
			Write:            true,
			IncludeGenerated: cmd.Bool("include-generated"),
		})
		if err != nil {
			return err
		}

		for _, changed := range result.Changed {
			if _, err := fmt.Fprintln(output, changed); err != nil {
				return err
			}
		}

		return runLint(ctx, cmd, output, path)
	}

	return command
}

func runLint(ctx context.Context, cmd *cli.Command, output io.Writer, path string) error {
	return goreadability.Lint(ctx, path, goreadability.LintOptions{
		GoBinary:     cmd.String("go"),
		Tags:         cmd.String("tags"),
		Staticcheck:  cmd.Bool("staticcheck"),
		GolangCILint: cmd.Bool("golangci-lint"),
		Output:       output,
	})
}

func getSinglePath(cmd *cli.Command) string {
	args := cmd.Args()
	if args == nil {
		return "."
	}

	path := strings.TrimSpace(args.First())
	if path == "" {
		return "."
	}

	return path
}

package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/diagnostics"
)

var doctorOutput io.Writer = os.Stdout

const (
	colorReset  = "\x1b[0m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorRed    = "\x1b[31m"
)

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Run startup diagnostics",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("env-file"), cmd.String("config"))
			if err == nil {
				handcli.ApplyConfigOverrides(cmd, cfg)
			}

			report := diagnostics.Build(cmd.String("env-file"), cmd.String("config"), cfg, err)
			for _, check := range report.Checks {
				if _, writeErr := fmt.Fprintf(doctorOutput, "[%s] %s: %s\n", formatStatus(check.Status, cfg), check.Name, check.Message); writeErr != nil {
					return writeErr
				}
			}

			if report.HasFailures() {
				return fmt.Errorf("doctor checks failed: %s", report.Summary())
			}

			_, err = fmt.Fprintln(doctorOutput, "doctor checks passed")
			return err
		},
	}
}

func formatStatus(status diagnostics.Status, cfg *config.Config) string {
	label := strings.ToUpper(string(status))
	if cfg != nil && cfg.LogNoColor {
		return label
	}

	switch status {
	case diagnostics.StatusPass:
		return colorGreen + label + colorReset
	case diagnostics.StatusWarn:
		return colorYellow + label + colorReset
	case diagnostics.StatusFail:
		return colorRed + label + colorReset
	default:
		return label
	}
}

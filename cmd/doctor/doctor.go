package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/diagnostics"
	"github.com/wandxy/hand/internal/diagnostics/readiness"
)

var doctorOutput io.Writer = os.Stdout

func SetOutput(w io.Writer) io.Writer {
	previous := doctorOutput
	if w == nil {
		doctorOutput = io.Discard
		return previous
	}
	doctorOutput = w
	return previous
}

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
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "Print diagnostics and readiness as JSON"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, inputs, err := handcli.LoadConfig(cmd)
			if err == nil {
				handcli.ApplyConfigOverrides(cmd, cfg)
				handcli.AddStartupFilesystemRoots(cfg, inputs)
			}

			report := diagnostics.Build(inputs.EnvPath, inputs.ConfigPath, cfg, err)
			safety := ""
			if cfg != nil {
				safety = handcli.SafetySummary(cfg)
			}
			var readinessReport readiness.Report
			if cfg != nil {
				readinessReport = readiness.Build(ctx, readiness.Options{
					Config:     cfg,
					Profile:    inputs.Profile,
					EnvPath:    inputs.EnvPath,
					ConfigPath: inputs.ConfigPath,
				})
			}
			if cmd.Bool("json") {
				if err := renderJSONReport(doctorOutput, report, readinessReport, safety); err != nil {
					return err
				}

				return doctorError(report, readinessReport)
			}

			for _, check := range report.Checks {
				if _, writeErr := fmt.Fprintf(doctorOutput, "[%s] %s: %s\n", formatStatus(check.Status, cfg), check.Name, check.Message); writeErr != nil {
					return writeErr
				}
			}
			if safety != "" {
				if _, writeErr := fmt.Fprintf(doctorOutput, "safety: %s\n", safety); writeErr != nil {
					return writeErr
				}
			}
			if err := renderReadinessReport(doctorOutput, readinessReport, cfg); err != nil {
				return err
			}
			if err := doctorError(report, readinessReport); err != nil {
				return err
			}

			_, err = fmt.Fprintln(doctorOutput, "doctor checks passed")
			return err
		},
	}
}

type jsonReport struct {
	OK          bool                 `json:"ok"`
	Summary     string               `json:"summary"`
	Diagnostics []jsonCheck          `json:"diagnostics"`
	Safety      string               `json:"safety,omitempty"`
	Readiness   []jsonReadinessGroup `json:"readiness,omitempty"`
}

type jsonReadinessGroup struct {
	Name   string      `json:"name"`
	Checks []jsonCheck `json:"checks"`
}

type jsonCheck struct {
	Name    string       `json:"name"`
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Actions []jsonAction `json:"actions,omitempty"`
}

type jsonAction struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
}

func renderJSONReport(w io.Writer, diagnosticsReport diagnostics.Report, readinessReport readiness.Report, safety string) error {
	payload := jsonReport{
		OK:          !diagnosticsReport.HasFailures() && !readinessReport.HasFailures(),
		Summary:     getDoctorSummary(diagnosticsReport, readinessReport),
		Diagnostics: diagnosticsChecksToJSON(diagnosticsReport.Checks),
		Safety:      strings.TrimSpace(safety),
		Readiness:   readinessGroupsToJSON(readinessReport.Groups),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func diagnosticsChecksToJSON(checks []diagnostics.Check) []jsonCheck {
	items := make([]jsonCheck, 0, len(checks))
	for _, check := range checks {
		items = append(items, jsonCheck{
			Name:    check.Name,
			Status:  string(check.Status),
			Message: check.Message,
		})
	}

	return items
}

func readinessGroupsToJSON(groups []readiness.Group) []jsonReadinessGroup {
	items := make([]jsonReadinessGroup, 0, len(groups))
	for _, group := range groups {
		item := jsonReadinessGroup{
			Name:   group.Name,
			Checks: make([]jsonCheck, 0, len(group.Checks)),
		}
		for _, check := range group.Checks {
			item.Checks = append(item.Checks, jsonCheck{
				Name:    check.Name,
				Status:  string(check.Status),
				Message: check.Message,
				Actions: readinessActionsToJSON(check.Actions),
			})
		}
		items = append(items, item)
	}

	return items
}

func readinessActionsToJSON(actions []readiness.Action) []jsonAction {
	items := make([]jsonAction, 0, len(actions))
	for _, action := range actions {
		items = append(items, jsonAction{
			Command:     action.Command,
			Description: action.Description,
		})
	}

	return items
}

func doctorError(diagnosticsReport diagnostics.Report, readinessReport readiness.Report) error {
	if diagnosticsReport.HasFailures() {
		return fmt.Errorf("doctor checks failed: %s", diagnosticsReport.Summary())
	}
	if readinessReport.HasFailures() {
		return fmt.Errorf("doctor checks failed: %s", readinessReport.Summary())
	}

	return nil
}

func getDoctorSummary(diagnosticsReport diagnostics.Report, readinessReport readiness.Report) string {
	if diagnosticsReport.HasFailures() {
		return diagnosticsReport.Summary()
	}
	if readinessReport.HasFailures() {
		return readinessReport.Summary()
	}

	return "doctor checks passed"
}

func renderReadinessReport(w io.Writer, report readiness.Report, cfg *config.Config) error {
	for _, group := range report.Groups {
		if _, err := fmt.Fprintf(w, "\n%s readiness:\n", group.Name); err != nil {
			return err
		}
		for _, check := range group.Checks {
			if _, err := fmt.Fprintf(w, "[%s] %s: %s\n", formatStatus(check.Status, cfg), check.Name, check.Message); err != nil {
				return err
			}
			for _, action := range check.Actions {
				line := action.Command
				if action.Description != "" {
					line += " - " + action.Description
				}
				if _, err := fmt.Fprintf(w, "  fix: %s\n", line); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func formatStatus(status diagnostics.Status, cfg *config.Config) string {
	label := strings.ToUpper(string(status))
	if cfg != nil && cfg.Log.NoColor {
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

package diagnostics

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/wandxy/hand/internal/config"
)

var osStat = os.Stat

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	Name    string
	Status  Status
	Message string
}

type Report struct {
	Checks []Check
}

func Build(envPath, configPath string, cfg *config.Config, loadErr error) Report {
	report := Report{
		Checks: []Check{
			fileCheck("env file", envPath, true),
			fileCheck("config file", configPath, false),
		},
	}

	if loadErr != nil {
		report.Checks = append(report.Checks, Check{
			Name:    "config load",
			Status:  StatusFail,
			Message: loadErr.Error(),
		})
		return report
	}

	if cfg == nil {
		report.Checks = append(report.Checks, Check{
			Name:    "config load",
			Status:  StatusFail,
			Message: "config is required",
		})
		return report
	}

	if err := cfg.Validate(); err != nil {
		report.Checks = append(report.Checks, Check{
			Name:    "config validation",
			Status:  StatusFail,
			Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, Check{
			Name:    "config validation",
			Status:  StatusPass,
			Message: "configuration is valid",
		})
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		report.Checks = append(report.Checks, Check{
			Name:    "model auth",
			Status:  StatusFail,
			Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, Check{
			Name:    "model auth",
			Status:  StatusPass,
			Message: fmt.Sprintf("resolved auth for provider %q", auth.Provider),
		})
		report.Checks = append(report.Checks, baseURLCheck(auth.BaseURL))
	}

	return report
}

func (r Report) HasFailures() bool {
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r Report) Summary() string {
	parts := make([]string, 0, len(r.Checks))
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			parts = append(parts, fmt.Sprintf("%s: %s", check.Name, check.Message))
		}
	}

	if len(parts) == 0 {
		return "startup diagnostics passed"
	}

	return strings.Join(parts, "; ")
}

func (r Report) FirstFailure() string {
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			return check.Message
		}
	}
	return ""
}

func fileCheck(name, path string, optional bool) Check {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: "not set",
		}
	}

	info, err := osStat(trimmed)
	if err == nil {
		if info.IsDir() {
			return Check{
				Name:    name,
				Status:  StatusFail,
				Message: fmt.Sprintf("%q is a directory", trimmed),
			}
		}

		return Check{
			Name:    name,
			Status:  StatusPass,
			Message: fmt.Sprintf("found %q", trimmed),
		}
	}

	if os.IsNotExist(err) && optional {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: fmt.Sprintf("%q not found; continuing without it", trimmed),
		}
	}

	if os.IsNotExist(err) {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: fmt.Sprintf("%q not found; continuing without file values", trimmed),
		}
	}

	return Check{
		Name:    name,
		Status:  StatusFail,
		Message: err.Error(),
	}
}

func baseURLCheck(raw string) Check {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Check{
			Name:    "model base URL",
			Status:  StatusPass,
			Message: "using provider default base URL",
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Check{
			Name:    "model base URL",
			Status:  StatusFail,
			Message: fmt.Sprintf("%q is not a valid absolute URL", trimmed),
		}
	}

	return Check{
		Name:    "model base URL",
		Status:  StatusPass,
		Message: fmt.Sprintf("using %q", trimmed),
	}
}

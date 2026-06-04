package readiness

import (
	"fmt"
	"os"
	"strings"

	"github.com/wandxy/hand/internal/profile"
)

var statPath = os.Stat

func buildProfileGroup(active profile.Profile, envPath string, configPath string) Group {
	active = profile.WithMetadataPaths(active)
	if strings.TrimSpace(envPath) != "" {
		active.EnvPath = strings.TrimSpace(envPath)
	}
	if strings.TrimSpace(configPath) != "" {
		active.ConfigPath = strings.TrimSpace(configPath)
	}

	return Group{
		Name: "profile",
		Checks: []Check{
			check("name", StatusPass, fmt.Sprintf("using profile %q", defaultString(active.Name, profile.DefaultName))),
			buildPathCheck("home", active.HomeDir, true, true),
			buildPathCheck("config", active.ConfigPath, false, true),
			buildPathCheck("env", active.EnvPath, false, true),
			buildPathCheck("runtime", active.RuntimePath, false, true),
		},
	}
}

func buildPathCheck(name string, path string, wantDir bool, optional bool) Check {
	path = strings.TrimSpace(path)
	if path == "" {
		status := StatusFail
		if optional {
			status = StatusWarn
		}
		return check(name, status, "path is not set")
	}

	info, err := statPath(path)
	if err != nil {
		if os.IsNotExist(err) {
			status := StatusFail
			message := fmt.Sprintf("%q does not exist", path)
			if optional {
				status = StatusWarn
				message = fmt.Sprintf("%q is not present", path)
			}
			return check(name, status, message)
		}

		return check(name, StatusFail, err.Error())
	}
	if wantDir && !info.IsDir() {
		return check(name, StatusFail, fmt.Sprintf("%q is not a directory", path))
	}
	if !wantDir && info.IsDir() {
		return check(name, StatusFail, fmt.Sprintf("%q is a directory", path))
	}

	return check(name, StatusPass, fmt.Sprintf("found %q", path))
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

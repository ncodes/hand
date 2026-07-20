package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func discoverChromiumExecutable(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return resolveExecutable(configured)
	}
	for _, candidate := range getChromiumExecutableCandidates() {
		resolved, err := resolveExecutable(candidate)
		if err == nil {
			return resolved, nil
		}
	}

	return "", errors.New("chromium executable not found; configure browser.executable")
}

func DiscoverChromiumExecutable(configured string) (string, error) {
	return discoverChromiumExecutable(configured)
}

func resolveExecutable(candidate string) (string, error) {
	if filepath.IsAbs(candidate) {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			return "", errors.New("browser executable is unavailable")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", errors.New("browser executable is not executable")
		}
		return filepath.Clean(candidate), nil
	}
	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return "", err
	}

	return resolved, nil
}

func getChromiumExecutableCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"google-chrome", "chromium", "chromium-browser",
		}
	case "windows":
		candidates := []string{"chrome.exe", "msedge.exe", "chromium.exe"}
		for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if root == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return candidates
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"}
	}
}

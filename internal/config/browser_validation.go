package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/net/idna"
)

func (c *Config) validateBrowserSettings() error {
	if c.Browser.StartTimeout <= 0 {
		return errors.New("browser start timeout must be greater than zero")
	}
	if c.Browser.InactivityTimeout <= 0 {
		return errors.New("browser inactivity timeout must be greater than zero")
	}
	if c.Browser.CleanupInterval <= 0 {
		return errors.New("browser cleanup interval must be greater than zero")
	}
	if c.Browser.TerminalRetention <= 0 {
		return errors.New("browser terminal retention must be greater than zero")
	}
	if c.Browser.Artifacts.MaxBytes <= 0 {
		return errors.New("browser artifact max bytes must be greater than zero")
	}
	if c.Browser.Artifacts.MaxTotalBytes < c.Browser.Artifacts.MaxBytes {
		return errors.New("browser artifact total bytes must be greater than or equal to max bytes")
	}
	if c.Browser.Artifacts.Retention <= 0 {
		return errors.New("browser artifact retention must be greater than zero")
	}
	for _, root := range []string{c.Browser.ProfileRoot, c.Browser.TemporaryRoot, c.Browser.Artifacts.Root} {
		if !filepath.IsAbs(root) {
			return errors.New("browser managed roots must be absolute")
		}
	}
	managedRoots := []string{c.Browser.ProfileRoot, c.Browser.TemporaryRoot, c.Browser.Artifacts.Root}
	canonicalRoots := make([]string, len(managedRoots))
	for index, root := range managedRoots {
		resolved, err := getCanonicalPath(root)
		if err != nil {
			return errors.New("browser managed root could not be resolved")
		}
		if isKnownPersonalBrowserPath(resolved) {
			return errors.New("browser managed roots must not use a personal browser data root")
		}
		canonicalRoots[index] = resolved
	}
	for left := range managedRoots {
		for right := left + 1; right < len(managedRoots); right++ {
			if isPathOverlapping(canonicalRoots[left], canonicalRoots[right]) {
				return errors.New("browser managed roots must not overlap")
			}
		}
	}

	if err := validateBrowserNetwork(c.Browser.Network); err != nil {
		return err
	}

	names := make(map[string]struct{}, len(c.Browser.Profiles))
	managedPaths := make([]string, 0, len(c.Browser.Profiles))
	for _, profile := range c.Browser.Profiles {
		if profile.Name == "" {
			return errors.New("browser profile name is required")
		}
		if _, exists := names[profile.Name]; exists {
			return errors.New("browser profile names must be unique")
		}
		names[profile.Name] = struct{}{}

		path, err := validateBrowserProfile(c.Browser.ProfileRoot, profile)
		if err != nil {
			return fmt.Errorf("browser profile %q: %w", profile.Name, err)
		}
		if path != "" {
			for _, existing := range managedPaths {
				if isPathOverlapping(existing, path) {
					return errors.New("browser managed profile directories must not overlap")
				}
			}
			managedPaths = append(managedPaths, path)
		}
	}
	if _, ok := names[c.Browser.DefaultProfile]; !ok {
		return errors.New("browser default profile must reference a configured profile")
	}
	defaultProfile, _ := c.Browser.Profile(c.Browser.DefaultProfile)
	if defaultProfile.Mode == BrowserProfileExistingSession {
		return errors.New("browser default profile must not attach to an existing session")
	}

	return nil
}

func validateBrowserNetwork(cfg BrowserNetworkConfig) error {
	for _, host := range cfg.DevelopmentAllowedHosts {
		ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(host), "."))
		if err != nil || ascii == "" || strings.ContainsAny(ascii, "/:@") {
			return errors.New("browser development allowed host is invalid")
		}
	}
	for _, raw := range cfg.DevelopmentAllowedCIDRs {
		if _, err := netip.ParsePrefix(raw); err != nil {
			return errors.New("browser development allowed CIDR is invalid")
		}
	}

	return nil
}

func validateBrowserProfile(root string, profile BrowserProfileConfig) (string, error) {
	switch profile.Mode {
	case BrowserProfileManagedEphemeral:
		if hasBrowserAttachmentConfig(profile) || profile.Directory != "" {
			return "", errors.New("managed ephemeral profile cannot set attachment configuration")
		}
		return "", nil
	case BrowserProfileManagedPersistent:
		if profile.Directory == "" {
			return "", errors.New("managed persistent profile directory is required")
		}
		if hasBrowserAttachmentConfig(profile) {
			return "", errors.New("managed persistent profile cannot set attachment configuration")
		}
		return validateManagedProfilePath(root, profile.Directory)
	case BrowserProfileRemoteCDP, BrowserProfileExistingSession:
		if profile.Directory != "" {
			return "", errors.New("attached browser profile cannot set a directory")
		}
		if profile.CDPEndpoint == "" {
			return "", errors.New("CDP endpoint is required")
		}
		if profile.Mode == BrowserProfileExistingSession && profile.DataIdentity == "" {
			return "", errors.New("existing session data identity is required")
		}
		if err := validateBrowserCredentialRef(profile.CredentialRef); err != nil {
			return "", err
		}
		if err := validateCDPEndpoint(profile.CDPEndpoint); err != nil {
			return "", err
		}
		if err := validateBrowserAttachmentScope(profile); err != nil {
			return "", err
		}
		if !profile.AcknowledgeUnmanagedEgress {
			return "", errors.New("attached browser profile must acknowledge unmanaged egress")
		}
		return "", nil
	default:
		return "", errors.New("mode must be one of: managed_ephemeral, managed_persistent, remote_cdp, existing_session")
	}
}

func hasBrowserAttachmentConfig(profile BrowserProfileConfig) bool {
	return profile.CDPEndpoint != "" || profile.CredentialRef != "" || profile.DataIdentity != "" ||
		profile.AttachmentScope != "" || profile.BrowserContextID != "" || len(profile.TargetIDs) > 0 ||
		profile.AcknowledgeUnmanagedEgress
}

func validateBrowserCredentialRef(value string) error {
	if value == "" {
		return nil
	}
	name, ok := strings.CutPrefix(value, "env:")
	if !ok || name == "" || strings.ContainsAny(name, "= \t\r\n") {
		return errors.New("CDP credential reference must use env:VARIABLE")
	}
	return nil
}

func validateBrowserAttachmentScope(profile BrowserProfileConfig) error {
	switch profile.AttachmentScope {
	case BrowserAttachmentTargets:
		if len(profile.TargetIDs) == 0 || profile.BrowserContextID != "" {
			return errors.New("target attachment scope requires target IDs and no browser context ID")
		}
	case BrowserAttachmentContext:
		if profile.BrowserContextID == "" || len(profile.TargetIDs) > 0 {
			return errors.New("context attachment scope requires a browser context ID and no target IDs")
		}
	case BrowserAttachmentBrowser:
		if profile.BrowserContextID != "" || len(profile.TargetIDs) > 0 {
			return errors.New("browser attachment scope cannot set a browser context ID or target IDs")
		}
	default:
		return errors.New("attachment scope must be one of: targets, context, browser")
	}
	return nil
}

func validateCDPEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("CDP endpoint is invalid")
	}
	if !slices.Contains([]string{"http", "https", "ws", "wss"}, strings.ToLower(parsed.Scheme)) {
		return errors.New("CDP endpoint scheme must be one of: http, https, ws, wss")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("CDP endpoint must not contain inline credentials, query parameters, or fragments")
	}

	return nil
}

func validateManagedProfilePath(root, configured string) (string, error) {
	return validateManagedProfilePathWith(root, configured, getCanonicalPath)
}

func validateManagedProfilePathWith(
	root string,
	configured string,
	canonicalize func(string) (string, error),
) (string, error) {
	if !filepath.IsAbs(root) || !filepath.IsAbs(configured) {
		return "", errors.New("managed persistent profile paths must be absolute")
	}
	absRoot := filepath.Clean(root)
	absConfigured := filepath.Clean(configured)
	if !isPathWithin(filepath.Clean(absRoot), filepath.Clean(absConfigured)) {
		return "", errors.New("managed persistent profile directory must be inside the browser profile root")
	}
	if hasPathSymlink(absRoot, absConfigured) {
		return "", errors.New("managed persistent profile directory must not traverse a symbolic link or junction")
	}
	rootPath, err := canonicalize(root)
	if err != nil {
		return "", err
	}
	profilePath, err := canonicalize(configured)
	if err != nil {
		return "", err
	}
	if profilePath == rootPath || !isPathWithin(rootPath, profilePath) {
		return "", errors.New("managed persistent profile directory must be inside the browser profile root")
	}
	if isKnownPersonalBrowserPath(profilePath) {
		return "", errors.New("managed persistent profile directory must not use a personal browser data root")
	}

	return profilePath, nil
}

func hasPathSymlink(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	current := filepath.Clean(root)
	for segment := range strings.SplitSeq(rel, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return !os.IsNotExist(statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}

	return false
}

func getCanonicalPath(path string) (string, error) {
	return getCanonicalPathWith(path, filepath.EvalSymlinks)
}

func getCanonicalPathWith(path string, evaluate func(string) (string, error)) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute")
	}
	absPath := filepath.Clean(path)

	existing := absPath
	remainder := make([]string, 0)
	for {
		resolved, resolveErr := evaluate(existing)
		if resolveErr == nil {
			for _, r := range slices.Backward(remainder) {
				resolved = filepath.Join(resolved, r)
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", resolveErr
		}
		remainder = append(remainder, filepath.Base(existing))
		existing = parent
	}
}

func isPathOverlapping(left, right string) bool {
	return left == right || isPathWithin(left, right) || isPathWithin(right, left)
}

func isPathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isKnownPersonalBrowserPath(candidate string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	paths := []string{
		filepath.Join(home, ".config", "google-chrome"),
		filepath.Join(home, ".config", "google-chrome-beta"),
		filepath.Join(home, ".config", "google-chrome-unstable"),
		filepath.Join(home, ".config", "chromium"),
		filepath.Join(home, ".config", "microsoft-edge"),
		filepath.Join(home, ".config", "microsoft-edge-beta"),
		filepath.Join(home, ".config", "microsoft-edge-dev"),
		filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser"),
		filepath.Join(home, "snap", "chromium", "common", "chromium"),
		filepath.Join(home, ".var", "app", "org.chromium.Chromium", "config", "chromium"),
		filepath.Join(home, ".var", "app", "com.brave.Browser", "config", "BraveSoftware", "Brave-Browser"),
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome"),
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome Beta"),
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome Canary"),
		filepath.Join(home, "Library", "Application Support", "Chromium"),
		filepath.Join(home, "Library", "Application Support", "Microsoft Edge"),
		filepath.Join(home, "Library", "Application Support", "Microsoft Edge Beta"),
		filepath.Join(home, "Library", "Application Support", "Microsoft Edge Dev"),
		filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser"),
		filepath.Join(home, "AppData", "Local", "Google", "Chrome", "User Data"),
		filepath.Join(home, "AppData", "Local", "Google", "Chrome Beta", "User Data"),
		filepath.Join(home, "AppData", "Local", "Google", "Chrome SxS", "User Data"),
		filepath.Join(home, "AppData", "Local", "Chromium", "User Data"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Edge", "User Data"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Edge Beta", "User Data"),
		filepath.Join(home, "AppData", "Local", "Microsoft", "Edge Dev", "User Data"),
		filepath.Join(home, "AppData", "Local", "BraveSoftware", "Brave-Browser", "User Data"),
	}
	for _, known := range paths {
		knownPath, resolveErr := getCanonicalPath(known)
		if resolveErr == nil && isPathOverlapping(knownPath, candidate) {
			return true
		}
	}

	return false
}

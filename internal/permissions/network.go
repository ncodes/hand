package permissions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/net/idna"
)

type NetworkRequestClass string

const (
	NetworkRequestNavigation  NetworkRequestClass = "navigation"
	NetworkRequestRedirect    NetworkRequestClass = "redirect"
	NetworkRequestSubresource NetworkRequestClass = "subresource"
	NetworkRequestWebSocket   NetworkRequestClass = "websocket"
	NetworkRequestDownload    NetworkRequestClass = "download"
	NetworkRequestCDP         NetworkRequestClass = "cdp"
	NetworkRequestBackground  NetworkRequestClass = "background"
)

type NetworkTarget struct {
	Scheme       string
	Host         string
	Port         uint16
	Path         string
	QueryHash    string
	Method       string
	RequestClass NetworkRequestClass
}

type NetworkSelector struct {
	Scheme       string              `yaml:"scheme"`
	Host         string              `yaml:"host"`
	Port         uint16              `yaml:"port"`
	PathPrefix   string              `yaml:"pathPrefix"`
	Method       string              `yaml:"method"`
	RequestClass NetworkRequestClass `yaml:"requestClass"`
}

func NetworkTargetFromURL(raw, method string, requestClass NetworkRequestClass) (NetworkTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return NetworkTarget{}, errors.New("permission network URL is invalid")
	}
	if parsed.User != nil {
		return NetworkTarget{}, errors.New("permission network URL must not contain inline credentials")
	}
	port, err := getNetworkPort(parsed.Scheme, parsed.Port())
	if err != nil {
		return NetworkTarget{}, err
	}

	queryHash := ""
	if query := parsed.Query().Encode(); query != "" {
		sum := sha256.Sum256([]byte(query))
		queryHash = hex.EncodeToString(sum[:])
	}
	return (NetworkTarget{
		Scheme:       parsed.Scheme,
		Host:         parsed.Hostname(),
		Port:         port,
		Path:         parsed.EscapedPath(),
		QueryHash:    queryHash,
		Method:       method,
		RequestClass: requestClass,
	}).Normalize()
}

func (t NetworkTarget) Normalize() (NetworkTarget, error) {
	t.Scheme = strings.ToLower(strings.TrimSpace(t.Scheme))
	if !isValidNetworkScheme(t.Scheme) {
		return NetworkTarget{}, errors.New("permission network scheme must be one of: http, https, ws, wss")
	}
	host, err := normalizeNetworkHost(t.Host)
	if err != nil {
		return NetworkTarget{}, err
	}
	t.Host = host
	if t.Port == 0 {
		t.Port, err = getNetworkPort(t.Scheme, "")
		if err != nil {
			return NetworkTarget{}, err
		}
	}
	t.Path, err = normalizeNetworkPath(t.Path, false)
	if err != nil {
		return NetworkTarget{}, err
	}
	t.QueryHash = strings.ToLower(strings.TrimSpace(t.QueryHash))
	if t.QueryHash != "" {
		decoded, decodeErr := hex.DecodeString(t.QueryHash)
		if decodeErr != nil || len(decoded) != sha256.Size {
			return NetworkTarget{}, errors.New("permission network query hash is invalid")
		}
	}
	t.Method = strings.ToUpper(strings.TrimSpace(t.Method))
	if t.Method == "" || !httpguts.ValidHeaderFieldName(t.Method) {
		return NetworkTarget{}, errors.New("permission network method is invalid")
	}
	t.RequestClass = NetworkRequestClass(strings.ToLower(strings.TrimSpace(string(t.RequestClass))))
	if !isValidNetworkRequestClass(t.RequestClass) {
		return NetworkTarget{}, errors.New("permission network request class is invalid")
	}

	return t, nil
}

func (s NetworkSelector) Normalize() (NetworkSelector, error) {
	var err error
	s.Scheme = strings.ToLower(strings.TrimSpace(s.Scheme))
	if s.Scheme != "" && !isValidNetworkScheme(s.Scheme) {
		return NetworkSelector{}, errors.New("permission network selector scheme is invalid")
	}
	if strings.TrimSpace(s.Host) != "" {
		s.Host, err = normalizeNetworkHost(s.Host)
		if err != nil {
			return NetworkSelector{}, err
		}
	}
	s.PathPrefix, err = normalizeNetworkPath(s.PathPrefix, true)
	if err != nil {
		return NetworkSelector{}, err
	}
	s.Method = strings.ToUpper(strings.TrimSpace(s.Method))
	if s.Method != "" && !httpguts.ValidHeaderFieldName(s.Method) {
		return NetworkSelector{}, errors.New("permission network selector method is invalid")
	}
	s.RequestClass = NetworkRequestClass(strings.ToLower(strings.TrimSpace(string(s.RequestClass))))
	if s.RequestClass != "" && !isValidNetworkRequestClass(s.RequestClass) {
		return NetworkSelector{}, errors.New("permission network selector request class is invalid")
	}
	if s == (NetworkSelector{}) {
		return NetworkSelector{}, errors.New("permission network selector must constrain at least one field")
	}

	return s, nil
}

func (s NetworkSelector) Matches(target NetworkTarget) bool {
	s, err := s.Normalize()
	if err != nil {
		return false
	}
	target, err = target.Normalize()
	if err != nil {
		return false
	}

	return (s.Scheme == "" || s.Scheme == target.Scheme) &&
		(s.Host == "" || s.Host == target.Host) &&
		(s.Port == 0 || s.Port == target.Port) &&
		(s.PathPrefix == "" || matchesNetworkPathPrefix(s.PathPrefix, target.Path)) &&
		(s.Method == "" || s.Method == target.Method) &&
		(s.RequestClass == "" || s.RequestClass == target.RequestClass)
}

func normalizeNetworkSelectors(values []NetworkSelector) ([]NetworkSelector, error) {
	result := make([]NetworkSelector, 0, len(values))
	for _, value := range values {
		normalized, err := value.Normalize()
		if err != nil {
			return nil, err
		}
		if !slices.Contains(result, normalized) {
			result = append(result, normalized)
		}
	}
	slices.SortFunc(result, func(left, right NetworkSelector) int {
		return strings.Compare(getNetworkSelectorFingerprint(left), getNetworkSelectorFingerprint(right))
	})

	return result, nil
}

func matchesNetworkSelectors(selectors []NetworkSelector, target *NetworkTarget) bool {
	if target == nil {
		return len(selectors) == 0
	}
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if selector.Matches(*target) {
			return true
		}
	}

	return false
}

func isSameNetworkTarget(left, right *NetworkTarget) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue, leftErr := left.Normalize()
	rightValue, rightErr := right.Normalize()
	return leftErr == nil && rightErr == nil && leftValue == rightValue
}

func getNetworkTargetFingerprint(target *NetworkTarget) string {
	if target == nil {
		return ""
	}
	value, err := target.Normalize()
	if err != nil {
		return "invalid"
	}
	return url.Values{
		"scheme": {value.Scheme},
		"host":   {value.Host},
		"port":   {strconv.Itoa(int(value.Port))},
		"path":   {value.Path},
		"query":  {value.QueryHash},
		"method": {value.Method},
		"class":  {string(value.RequestClass)},
	}.Encode()
}

func getNetworkSelectorFingerprint(selector NetworkSelector) string {
	return url.Values{
		"scheme": {selector.Scheme},
		"host":   {selector.Host},
		"port":   {strconv.Itoa(int(selector.Port))},
		"path":   {selector.PathPrefix},
		"method": {selector.Method},
		"class":  {string(selector.RequestClass)},
	}.Encode()
}

func getNetworkSelectorSpecificity(selectors []NetworkSelector) int {
	result := 0
	for _, selector := range selectors {
		if selector.Scheme != "" {
			result++
		}
		if selector.Host != "" {
			result++
		}
		if selector.Port != 0 {
			result++
		}
		if selector.PathPrefix != "" {
			result++
		}
		if selector.Method != "" {
			result++
		}
		if selector.RequestClass != "" {
			result++
		}
	}

	return result
}

func normalizeNetworkHost(raw string) (string, error) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" || strings.ContainsAny(ascii, "/:@") {
		return "", errors.New("permission network host is invalid")
	}

	return ascii, nil
}

func normalizeNetworkPath(raw string, optional bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if optional {
			return "", nil
		}
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") {
		return "", errors.New("permission network path must be absolute")
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return "", errors.New("permission network path must not contain encoded separators")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.Contains(decoded, "\\") {
		return "", errors.New("permission network path is invalid")
	}
	normalized := path.Clean(decoded)
	if normalized == "." {
		normalized = "/"
	}

	return normalized, nil
}

func matchesNetworkPathPrefix(prefix, target string) bool {
	return prefix == "/" || target == prefix || strings.HasPrefix(target, strings.TrimSuffix(prefix, "/")+"/")
}

func getNetworkPort(scheme, raw string) (uint16, error) {
	if raw != "" {
		value, err := strconv.ParseUint(raw, 10, 16)
		if err != nil || value == 0 {
			return 0, errors.New("permission network port is invalid")
		}
		return uint16(value), nil
	}
	if strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "ws") {
		return 80, nil
	}
	if strings.EqualFold(scheme, "https") || strings.EqualFold(scheme, "wss") {
		return 443, nil
	}

	return 0, errors.New("permission network port is required")
}

func isValidNetworkScheme(value string) bool {
	return value == "http" || value == "https" || value == "ws" || value == "wss"
}

func isValidNetworkRequestClass(value NetworkRequestClass) bool {
	return slices.Contains([]NetworkRequestClass{
		NetworkRequestNavigation,
		NetworkRequestRedirect,
		NetworkRequestSubresource,
		NetworkRequestWebSocket,
		NetworkRequestDownload,
		NetworkRequestCDP,
		NetworkRequestBackground,
	}, value)
}

package guardrails

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

var jsonUnmarshal = json.Unmarshal

type Redactor interface {
	Sanitize(any) any
}

type DefaultRedactor struct{}

var (
	prefixTokenPattern     = regexp.MustCompile(`(^|[^A-Za-z0-9_-])(sk-[A-Za-z0-9_-]{10,}|ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,}|xox[baprs]-[A-Za-z0-9-]{10,}|AIza[A-Za-z0-9_-]{30,}|pplx-[A-Za-z0-9]{10,}|fal_[A-Za-z0-9_-]{10,}|fc-[A-Za-z0-9]{10,}|bb_live_[A-Za-z0-9_-]{10,}|gAAAA[A-Za-z0-9_=-]{20,}|AKIA[A-Z0-9]{16}|sk_live_[A-Za-z0-9]{10,}|sk_test_[A-Za-z0-9]{10,}|rk_live_[A-Za-z0-9]{10,}|SG\.[A-Za-z0-9_-]{10,}|hf_[A-Za-z0-9]{10,}|r8_[A-Za-z0-9]{10,}|npm_[A-Za-z0-9]{10,}|pypi-[A-Za-z0-9_-]{10,}|dop_v1_[A-Za-z0-9]{10,}|doo_v1_[A-Za-z0-9]{10,}|am_[A-Za-z0-9_-]{10,})($|[^A-Za-z0-9_-])`)
	envAssignQuotedPattern = regexp.MustCompile(`(?i)([A-Z_]*(?:API_?KEY|ACCESS_TOKEN|REFRESH_TOKEN|AUTH_TOKEN|AUTH_SECRET|BEARER_TOKEN|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL)[A-Z_]*)\s*=\s*"([^"]*)"`)
	envAssignSinglePattern = regexp.MustCompile(`(?i)([A-Z_]*(?:API_?KEY|ACCESS_TOKEN|REFRESH_TOKEN|AUTH_TOKEN|AUTH_SECRET|BEARER_TOKEN|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL)[A-Z_]*)\s*=\s*'([^']*)'`)
	envAssignBarePattern   = regexp.MustCompile(`(?i)([A-Z_]*(?:API_?KEY|ACCESS_TOKEN|REFRESH_TOKEN|AUTH_TOKEN|AUTH_SECRET|BEARER_TOKEN|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL)[A-Z_]*)\s*=\s*([^\s'"][^\s]*)`)
	jsonFieldPattern       = regexp.MustCompile(`(?i)("(?:api_?[Kk]ey|token|secret|password|access_token|refresh_token|auth_token|bearer|secret_value|raw_secret|secret_input|key_material)")\s*:\s*"([^"]+)"`)
	authHeaderPattern      = regexp.MustCompile(`(?i)(Authorization:\s*(?:Bearer|Basic|Token|ApiKey)\s+)(\S+)`)
	bearerPattern          = regexp.MustCompile(`(?i)\b(Bearer\s+)(\S+)`)
	telegramPattern        = regexp.MustCompile(`(?i)(bot)?(\d{8,}):([-A-Za-z0-9_]{30,})`)
	privateKeyPattern      = regexp.MustCompile(`(?s)-----BEGIN[A-Z ]*PRIVATE KEY-----.*?-----END[A-Z ]*PRIVATE KEY-----`)
	dbConnPattern          = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp|https?|ftps?|sftp|ssh|mssql|sqlserver|oracle|cassandra|nats|kafka)://[^:]+:)([^@]+)(@)`)
	phonePattern           = regexp.MustCompile(`\+[1-9]\d{6,14}`)
	secretKeys             = []string{
		"authorization", "api_key", "apikey", "openaiapikey", "openrouterapikey", "modelkey",
		"token", "secret", "password", "credential", "accesstoken", "refreshtoken",
		"authtoken", "bearer", "secretvalue", "rawsecret", "secretinput", "keymaterial",
	}
)

func NewRedactor() Redactor {
	return DefaultRedactor{}
}

func (DefaultRedactor) Sanitize(value any) any {
	return sanitizeValue(value)
}

func Sanitize(value any) any {
	return sanitizeValue(value)
}

func sanitizeValue(value any) any {
	switch val := value.(type) {
	case nil:
		return nil
	case map[string]any:
		sanitized := make(map[string]any, len(val))
		for key, item := range val {
			if isSensitiveKey(key) {
				sanitized[key] = "[REDACTED]"
				continue
			}
			sanitized[key] = sanitizeValue(item)
		}

		return sanitized
	case []any:
		sanitized := make([]any, 0, len(val))
		for _, item := range val {
			sanitized = append(sanitized, sanitizeValue(item))
		}

		return sanitized
	case string:
		return sanitizeString(val)
	case bool, float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return val
	case []string:
		sanitized := make([]string, 0, len(val))
		for _, item := range val {
			sanitized = append(sanitized, sanitizeString(item))
		}

		return sanitized
	default:
		raw, err := json.Marshal(val)
		if err != nil {
			return val
		}
		var normalized any
		if err := jsonUnmarshal(raw, &normalized); err != nil {
			return sanitizeString(string(raw))
		}

		return sanitizeValue(normalized)
	}
}

func sanitizeString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	var decoded any
	if jsonUnmarshal([]byte(trimmed), &decoded) == nil {
		sanitized, err := json.Marshal(sanitizeValue(decoded))
		if err == nil {
			return string(sanitized)
		}
	}

	// Structured values are handled above and redact sensitive fields to
	// "[REDACTED]". Free-form text uses partial masking where possible so logs
	// and traces retain enough context to identify the credential family.
	sanitized := replaceAllStringSubmatchFunc(envAssignQuotedPattern, value, func(groups []string) string {
		return groups[1] + `="` + maskToken(groups[2]) + `"`
	})
	sanitized = replaceAllStringSubmatchFunc(envAssignSinglePattern, sanitized, func(groups []string) string {
		return groups[1] + `='` + maskToken(groups[2]) + `'`
	})
	sanitized = replaceAllStringSubmatchFunc(envAssignBarePattern, sanitized, func(groups []string) string {
		return groups[1] + "=" + maskToken(groups[2])
	})
	sanitized = replaceAllStringSubmatchFunc(jsonFieldPattern, sanitized, func(groups []string) string {
		return groups[1] + `: "` + maskToken(groups[2]) + `"`
	})
	sanitized = replaceAllStringSubmatchFunc(authHeaderPattern, sanitized, func(groups []string) string {
		return groups[1] + maskToken(groups[2])
	})
	sanitized = replaceAllStringSubmatchFunc(bearerPattern, sanitized, func(groups []string) string {
		return groups[1] + maskToken(groups[2])
	})
	sanitized = replaceAllStringSubmatchFunc(telegramPattern, sanitized, func(groups []string) string {
		return groups[1] + groups[2] + ":***"
	})
	sanitized = privateKeyPattern.ReplaceAllString(sanitized, "[REDACTED PRIVATE KEY]")
	sanitized = dbConnPattern.ReplaceAllString(sanitized, `${1}***${3}`)
	sanitized = phonePattern.ReplaceAllStringFunc(sanitized, func(match string) string {
		return redactPhone(match)
	})
	sanitized = replaceAllStringSubmatchFunc(prefixTokenPattern, sanitized, func(groups []string) string {
		return groups[1] + maskToken(groups[2]) + groups[3]
	})
	return sanitized
}

func replaceAllStringSubmatchFunc(re *regexp.Regexp, value string, replace func(groups []string) string) string {
	indexes := re.FindAllStringSubmatchIndex(value, -1)
	if len(indexes) == 0 {
		return value
	}

	groupCount := re.NumSubexp() + 1
	var builder strings.Builder
	last := 0

	for _, match := range indexes {
		start, end := match[0], match[1]
		builder.WriteString(value[last:start])

		groups := make([]string, groupCount)
		for i := 0; i < groupCount; i++ {
			groupStart, groupEnd := match[i*2], match[i*2+1]
			if groupStart >= 0 && groupEnd >= 0 {
				groups[i] = value[groupStart:groupEnd]
			}
		}
		builder.WriteString(replace(groups))
		last = end
	}

	builder.WriteString(value[last:])
	return builder.String()
}

func maskToken(token string) string {
	if len(token) < 18 {
		return "***"
	}

	return token[:6] + "..." + token[len(token)-4:]
}

func redactPhone(phone string) string {
	if len(phone) <= 8 {
		return phone[:2] + "****" + phone[len(phone)-2:]
	}

	return phone[:4] + "****" + phone[len(phone)-4:]
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", ""), "_", ""))
	return slices.Contains(secretKeys, normalized)
}

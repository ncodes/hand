package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	browserdomain "github.com/wandxy/morph/internal/browser"
)

const (
	maxBrowserInputBytes  = 1 << 20
	maxBrowserIDLength    = 256
	maxBrowserURLLength   = 8192
	maxBrowserTextLength  = 65536
	maxBrowserValueLength = 8192
	maxBrowserKeyLength   = 128
	maxBrowserRefLength   = 128
)

type request struct {
	Action    browserdomain.Action        `json:"action"`
	Profile   string                      `json:"profile,omitempty"`
	SessionID string                      `json:"session_id,omitempty"`
	TabID     string                      `json:"tab_id,omitempty"`
	URL       string                      `json:"url,omitempty"`
	Ref       string                      `json:"ref,omitempty"`
	Text      string                      `json:"text,omitempty"`
	Value     string                      `json:"value,omitempty"`
	Key       string                      `json:"key,omitempty"`
	X         int64                       `json:"x,omitempty"`
	Y         int64                       `json:"y,omitempty"`
	Condition browserdomain.WaitCondition `json:"condition,omitempty"`
	TimeoutMS int64                       `json:"timeout_ms,omitempty"`
	Replace   bool                        `json:"replace,omitempty"`
}

type requestSpec struct {
	allowed  []string
	required []string
}

var requestSpecs = map[browserdomain.Action]requestSpec{
	browserdomain.ActionStatus:   {allowed: []string{"action"}, required: []string{"action"}},
	browserdomain.ActionProfiles: {allowed: []string{"action"}, required: []string{"action"}},
	browserdomain.ActionStart:    {allowed: []string{"action", "profile"}, required: []string{"action"}},
	browserdomain.ActionStop:     sessionRequestSpec(),
	browserdomain.ActionTabs:     sessionRequestSpec(),
	browserdomain.ActionOpen: {
		allowed: []string{"action", "session_id", "url"}, required: []string{"action", "session_id", "url"},
	},
	browserdomain.ActionFocus:    tabRequestSpec(),
	browserdomain.ActionClose:    tabRequestSpec(),
	browserdomain.ActionNavigate: tabValueRequestSpec("url"),
	browserdomain.ActionReload:   tabRequestSpec(),
	browserdomain.ActionSnapshot: tabRequestSpec(),
	browserdomain.ActionClick:    tabValueRequestSpec("ref"),
	browserdomain.ActionType: {
		allowed:  []string{"action", "session_id", "tab_id", "ref", "text", "replace"},
		required: []string{"action", "session_id", "tab_id", "ref", "text"},
	},
	browserdomain.ActionPress: tabValueRequestSpec("key"),
	browserdomain.ActionScroll: {
		allowed:  []string{"action", "session_id", "tab_id", "x", "y"},
		required: []string{"action", "session_id", "tab_id", "y"},
	},
	browserdomain.ActionSelect: tabValueRequestSpec("ref", "value"),
	browserdomain.ActionWait: {
		allowed:  []string{"action", "session_id", "tab_id", "condition", "value", "ref", "timeout_ms"},
		required: []string{"action", "session_id", "tab_id", "condition"},
	},
	browserdomain.ActionBack:    tabRequestSpec(),
	browserdomain.ActionForward: tabRequestSpec(),
}

func sessionRequestSpec() requestSpec {
	return requestSpec{allowed: []string{"action", "session_id"}, required: []string{"action", "session_id"}}
}

func tabRequestSpec() requestSpec {
	return requestSpec{
		allowed: []string{"action", "session_id", "tab_id"}, required: []string{"action", "session_id", "tab_id"},
	}
}

func tabValueRequestSpec(fields ...string) requestSpec {
	spec := tabRequestSpec()
	spec.allowed = append(spec.allowed, fields...)
	spec.required = append(spec.required, fields[0])
	if len(fields) > 1 {
		spec.required = append(spec.required, fields[1:]...)
	}
	return spec
}

func decodeRequest(raw string) (request, error) {
	if len(raw) > maxBrowserInputBytes {
		return request{}, errors.New("browser input exceeds the maximum size")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &fields); err != nil {
		return request{}, errors.New("browser input must be valid JSON")
	}
	var action browserdomain.Action
	if err := json.Unmarshal(fields["action"], &action); err != nil || action == "" {
		return request{}, errors.New("browser action is required")
	}
	spec, ok := requestSpecs[action]
	if !ok {
		return request{}, errors.New("browser action is not supported")
	}
	allowed := make(map[string]struct{}, len(spec.allowed))
	for _, name := range spec.allowed {
		allowed[name] = struct{}{}
	}
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			return request{}, fmt.Errorf("browser field %q is not valid for action %q", name, action)
		}
	}
	for _, name := range spec.required {
		if _, ok := fields[name]; !ok {
			return request{}, fmt.Errorf("browser field %q is required for action %q", name, action)
		}
	}

	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var decoded request
	if err := decoder.Decode(&decoded); err != nil {
		return request{}, errors.New("browser input has an invalid field type")
	}
	if decoded.TimeoutMS < 0 || decoded.TimeoutMS > int64((2*time.Minute)/time.Millisecond) {
		return request{}, errors.New("browser timeout_ms must be between zero and 120000")
	}
	if decoded.X < -100000 || decoded.X > 100000 || decoded.Y < -100000 || decoded.Y > 100000 {
		return request{}, errors.New("browser scroll offsets must be between -100000 and 100000")
	}
	if err := checkStringLengths(decoded); err != nil {
		return request{}, err
	}
	for _, name := range getNonEmptyFields(decoded.Action) {
		if strings.TrimSpace(getStringField(decoded, name)) == "" {
			return request{}, fmt.Errorf("browser field %q must not be empty for action %q", name, decoded.Action)
		}
	}
	if decoded.Action == browserdomain.ActionWait {
		switch decoded.Condition {
		case browserdomain.WaitLoad:
		case browserdomain.WaitText, browserdomain.WaitURL:
			if strings.TrimSpace(decoded.Value) == "" {
				return request{}, errors.New("browser wait value is required for text and URL conditions")
			}
		case browserdomain.WaitVisible:
			if strings.TrimSpace(decoded.Ref) == "" {
				return request{}, errors.New("browser wait ref is required for the visible condition")
			}
		default:
			return request{}, errors.New("browser wait condition must be one of: load, text, url, visible")
		}
	}
	return decoded, nil
}

func checkStringLengths(value request) error {
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{name: "profile", value: value.Profile, limit: maxBrowserIDLength},
		{name: "session_id", value: value.SessionID, limit: maxBrowserIDLength},
		{name: "tab_id", value: value.TabID, limit: maxBrowserIDLength},
		{name: "url", value: value.URL, limit: maxBrowserURLLength},
		{name: "ref", value: value.Ref, limit: maxBrowserRefLength},
		{name: "text", value: value.Text, limit: maxBrowserTextLength},
		{name: "value", value: value.Value, limit: maxBrowserValueLength},
		{name: "key", value: value.Key, limit: maxBrowserKeyLength},
	}
	for _, field := range fields {
		if len(field.value) > field.limit {
			return fmt.Errorf("browser field %q exceeds the maximum length", field.name)
		}
	}
	return nil
}

func getNonEmptyFields(action browserdomain.Action) []string {
	switch action {
	case browserdomain.ActionStop, browserdomain.ActionTabs:
		return []string{"session_id"}
	case browserdomain.ActionOpen:
		return []string{"session_id", "url"}
	case browserdomain.ActionFocus, browserdomain.ActionClose, browserdomain.ActionReload,
		browserdomain.ActionSnapshot, browserdomain.ActionBack, browserdomain.ActionForward:
		return []string{"session_id", "tab_id"}
	case browserdomain.ActionNavigate:
		return []string{"session_id", "tab_id", "url"}
	case browserdomain.ActionClick:
		return []string{"session_id", "tab_id", "ref"}
	case browserdomain.ActionType, browserdomain.ActionSelect:
		return []string{"session_id", "tab_id", "ref"}
	case browserdomain.ActionPress:
		return []string{"session_id", "tab_id", "key"}
	case browserdomain.ActionScroll, browserdomain.ActionWait:
		return []string{"session_id", "tab_id"}
	default:
		return nil
	}
}

func getStringField(value request, name string) string {
	switch name {
	case "session_id":
		return value.SessionID
	case "tab_id":
		return value.TabID
	case "url":
		return value.URL
	case "ref":
		return value.Ref
	case "key":
		return value.Key
	default:
		return ""
	}
}

func actionRequestFromRequest(r request) browserdomain.ActionRequest {
	return browserdomain.ActionRequest{
		Profile: r.Profile, SessionID: r.SessionID, TabID: r.TabID, URL: r.URL, Ref: r.Ref,
		Text: r.Text, Value: r.Value, Key: r.Key, X: r.X, Y: r.Y, Condition: r.Condition,
		Timeout: time.Duration(r.TimeoutMS) * time.Millisecond, Replace: r.Replace,
	}
}

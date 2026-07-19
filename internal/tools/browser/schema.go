package browser

import browserdomain "github.com/wandxy/morph/internal/browser"

func inputSchema() map[string]any {
	branches := make([]any, 0, len(requestSpecs))
	for _, action := range supportedActions() {
		spec := requestSpecs[action]
		properties := map[string]any{
			"action": map[string]any{"type": "string", "const": string(action)},
		}
		for _, field := range spec.allowed {
			if field == "action" {
				continue
			}
			properties[field] = schemaForField(field)
		}
		branches = append(branches, map[string]any{
			"type": "object", "properties": properties, "required": spec.required, "additionalProperties": false,
		})
	}
	return map[string]any{"oneOf": branches}
}

func schemaForField(name string) map[string]any {
	switch name {
	case "replace", "full_page":
		return map[string]any{"type": "boolean"}
	case "limit":
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 200}
	case "x", "y":
		return map[string]any{"type": "integer", "minimum": -100000, "maximum": 100000}
	case "timeout_ms":
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 120000}
	case "condition":
		return map[string]any{"type": "string", "enum": []string{"load", "text", "url", "visible"}}
	default:
		return map[string]any{"type": "string", "maxLength": maxLengthForField(name)}
	}
}

func maxLengthForField(name string) int {
	switch name {
	case "url", "path":
		return maxBrowserURLLength
	case "text":
		return maxBrowserTextLength
	case "value":
		return maxBrowserValueLength
	case "key":
		return maxBrowserKeyLength
	case "ref":
		return maxBrowserRefLength
	default:
		return maxBrowserIDLength
	}
}

func supportedActions() []browserdomain.Action {
	return []browserdomain.Action{
		browserdomain.ActionStatus, browserdomain.ActionProfiles, browserdomain.ActionStart, browserdomain.ActionStop,
		browserdomain.ActionTabs, browserdomain.ActionOpen, browserdomain.ActionFocus, browserdomain.ActionClose,
		browserdomain.ActionNavigate, browserdomain.ActionReload, browserdomain.ActionSnapshot, browserdomain.ActionClick,
		browserdomain.ActionScreenshot, browserdomain.ActionPDF, browserdomain.ActionConsole,
		browserdomain.ActionType, browserdomain.ActionPress, browserdomain.ActionScroll, browserdomain.ActionSelect,
		browserdomain.ActionUpload, browserdomain.ActionDownload, browserdomain.ActionAcceptDialog,
		browserdomain.ActionDismissDialog, browserdomain.ActionWait, browserdomain.ActionBack, browserdomain.ActionForward,
	}
}

package tui

import (
	"bytes"
	"encoding/json"

	"github.com/wandxy/morph/pkg/str"
)

func getUserFacingErrorMessage(message string) string {
	stringValue1 := str.String(message)
	message = stringValue1.Trim()
	if message == "" {
		return ""
	}

	if providerMessage := getProviderErrorMessage(message); providerMessage != "" {
		return "Model provider rejected the request: " + providerMessage
	}

	return message
}

func getProviderErrorMessage(message string) string {
	for index, char := range message {
		if char != '{' {
			continue
		}

		var payload map[string]any
		decoder := json.NewDecoder(bytes.NewBufferString(message[index:]))
		if err := decoder.Decode(&payload); err != nil {
			continue
		}
		if errorMessage := getErrorMessageFromPayload(payload); errorMessage != "" {
			return errorMessage
		}
	}

	return ""
}

func getErrorMessageFromPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	if value, ok := payload["error"]; ok {
		if message := getErrorMessageFromValue(value); message != "" {
			return message
		}
	}

	return getStringPayloadField(payload, "message")
}

func getErrorMessageFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		stringValue2 := str.String(typed)
		return stringValue2.Trim()
	case map[string]any:
		return getStringPayloadField(typed, "message")
	default:
		return ""
	}
}

func getStringPayloadField(payload map[string]any, field string) string {
	value, _ := payload[field].(string)
	stringValue3 := str.String(value)
	return stringValue3.Trim()
}

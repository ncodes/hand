package provider_ollama

import (
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

func NormalizeModelID(value string) string {
	stringValue1 := str.String(value)
	value = stringValue1.Trim()
	if provider, model, ok := strings.Cut(value, "/"); ok {
		providerValue := str.String(provider)
		if !strings.EqualFold(providerValue.Trim(), "ollama") {
			return value
		}
		stringValue3 := str.String(model)
		return stringValue3.Trim()
	}

	return value
}

func NormalizeModelIDForComparison(value string) string {
	value = NormalizeModelID(value)
	if value == "" || strings.Contains(value, ":") {
		return value
	}

	return value + ":latest"
}

func ModelIDMatches(installed string, requested string) bool {
	return strings.EqualFold(
		NormalizeModelIDForComparison(installed),
		NormalizeModelIDForComparison(requested),
	)
}

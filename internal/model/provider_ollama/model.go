package provider_ollama

import (
	"strings"
)

func NormalizeModelID(value string) string {
	value = strings.TrimSpace(value)
	if provider, model, ok := strings.Cut(value, "/"); ok &&
		strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return strings.TrimSpace(model)
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

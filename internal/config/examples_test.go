package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigExamples_EnvFilesListSupportedEnvironmentKeys(t *testing.T) {
	expected := supportedEnvironmentKeys(t)
	for _, file := range []struct {
		path     string
		optional bool
	}{
		{path: filepath.Join("..", "..", ".env"), optional: true},
		{path: filepath.Join("..", "..", "example.env")},
	} {
		t.Run(file.path, func(t *testing.T) {
			content, ok := readOptionalTextFile(t, file.path)
			if !ok && file.optional {
				t.Skip("local env file is not present")
			}
			require.True(t, ok)

			for _, key := range expected {
				require.Regexp(t, regexp.MustCompile(`(?m)^#?\s*`+regexp.QuoteMeta(key)+`=`), content, key)
			}
			for _, match := range regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]*)=`).FindAllStringSubmatch(content, -1) {
				require.Truef(
					t,
					strings.HasPrefix(match[1], "HAND_") || hasNativeProviderEnvKey(match[1]),
					"env key %q must use HAND_ prefix or be a provider-native credential key",
					match[1],
				)
			}
		})
	}
}

func hasNativeProviderEnvKey(key string) bool {
	switch key {
	case "OPENAI_API_KEY", "OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "COPILOT_GITHUB_TOKEN":
		return true
	default:
		return false
	}
}

func TestConfigExamples_YAMLFilesListSupportedConfigPaths(t *testing.T) {
	for _, file := range []struct {
		path     string
		optional bool
	}{
		{path: filepath.Join("..", "..", "config.yaml"), optional: true},
		{path: filepath.Join("..", "..", "example.yaml")},
	} {
		t.Run(file.path, func(t *testing.T) {
			content, ok := readOptionalTextFile(t, file.path)
			if !ok && file.optional {
				t.Skip("local YAML config file is not present")
			}
			require.True(t, ok)

			rootKeys := []string{"name", "platform", "search", "reranker", "trace"}
			if !file.optional {
				rootKeys = append(rootKeys, "memory")
			}
			requireYAMLKeys(t, content, "", rootKeys)
			requireYAMLKeys(t, content, "models", []string{
				"maxRetries",
				"providers",
				"main",
				"summary",
				"embedding",
			})
			requireYAMLKeys(t, content, "main", []string{
				"name",
				"provider",
				"api",
				"baseUrl",
				"stream",
				"contextLength",
			})
			requireYAMLKeys(t, content, "summary", []string{"name", "provider", "api", "baseUrl"})
			requireYAMLKeys(t, content, "embedding", []string{"name", "provider"})
			requireYAMLKeys(t, content, "rpc", []string{"address", "port"})
			requireYAMLKeys(t, content, "fs", []string{"roots"})
			requireYAMLKeys(t, content, "exec", []string{"allow", "ask", "deny"})
			requireYAMLKeys(t, content, "storage", []string{"backend"})
			requireYAMLKeys(t, content, "session", []string{
				"maxIterations",
				"instruct",
				"defaultIdleExpiry",
				"archiveRetention",
			})
			requireYAMLKeys(t, content, "vector", []string{
				"enabled",
				"required",
				"rebuildBatchSize",
			})
			requireYAMLKeys(t, content, "search", []string{"enableRerank"})
			if !file.optional {
				requireYAMLKeys(t, content, "memory", []string{"enabled", "provider"})
			}
			requireYAMLKeys(t, content, "reranker", []string{
				"enabled",
				"type",
				"model",
				"maxCandidates",
				"maxCandidateTextChars",
				"maxOutputTokens",
				"overrides",
			})
			requireYAMLKeys(t, content, "compaction", []string{"enabled", "triggerPercent", "warnPercent"})
			requireYAMLKeys(t, content, "cap", []string{"fs", "net", "exec", "mem", "browser"})
			requireYAMLKeys(t, content, "log", []string{"level", "noColor"})
			requireYAMLKeys(t, content, "debug", []string{"requests"})
			requireYAMLKeys(t, content, "trace", []string{"enabled", "disk", "database"})
			requireYAMLKeys(t, content, "web", []string{
				"provider",
				"apiKey",
				"baseUrl",
				"maxCharPerResult",
				"maxExtractCharPerResult",
				"maxExtractResponseBytes",
				"cacheTTL",
				"blockedDomains",
				"native",
				"enabled",
				"domains",
				"files",
				"extractMinSummarizeChars",
				"extractMaxSummaryChars",
				"extractMaxSummaryChunkChars",
				"extractRefusalThresholdChars",
			})
			requireYAMLKeys(t, content, "native", []string{
				"allowedHosts",
				"blockedHosts",
				"allowedHostFiles",
				"blockedHostFiles",
			})
			requireYAMLKeys(t, content, "rules", []string{"files"})
		})
	}
}

func supportedEnvironmentKeys(t *testing.T) []string {
	t.Helper()

	content := readTextFile(t, "env.go")
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\("([A-Z0-9_]+)"\)`),
		regexp.MustCompile(`parseOptionalBoolEnv\("([A-Z0-9_]+)"\)`),
	}
	seen := map[string]struct{}{}
	var keys []string
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(content, -1) {
			if _, ok := seen[match[1]]; ok {
				continue
			}
			seen[match[1]] = struct{}{}
			keys = append(keys, match[1])
		}
	}

	require.NotEmpty(t, keys)
	return keys
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func readOptionalTextFile(t *testing.T, path string) (string, bool) {
	t.Helper()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	require.NoError(t, err)

	return string(data), true
}

func requireYAMLKeys(t *testing.T, content, section string, keys []string) {
	t.Helper()

	if section != "" {
		require.Regexp(t, regexp.MustCompile(`(?m)^#?\s*`+regexp.QuoteMeta(section)+`:`), content, section)
	}
	for _, key := range keys {
		var pattern string
		if section == "" {
			pattern = `(?m)^#?\s*` + regexp.QuoteMeta(key) + `:`
		} else {
			pattern = `(?m)^#?\s{2,}` + regexp.QuoteMeta(key) + `:`
		}
		require.Regexp(t, regexp.MustCompile(pattern), content, section+"."+key)
	}
}

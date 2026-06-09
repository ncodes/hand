package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/constants"
)

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if value := strings.TrimSpace(os.Getenv("HAND_NAME")); value != "" {
		cfg.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL")); value != "" {
		cfg.Models.Main.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY")); value != "" {
		cfg.Models.Summary.Name = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MODEL_STREAM"); ok {
		cfg.Models.Main.Stream = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_CONTEXT_LENGTH")); value != "" {
		if contextLength, err := strconv.Atoi(value); err == nil {
			cfg.Models.Main.ContextLength = contextLength
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_MAX_RETRIES")); value != "" {
		if retries, err := strconv.Atoi(value); err == nil {
			cfg.Models.MaxRetries = &retries
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_PROVIDER")); value != "" {
		cfg.Models.Main.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_EMBEDDING_PROVIDER")); value != "" {
		cfg.Models.Embedding.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_EMBEDDING_MODEL")); value != "" {
		cfg.Models.Embedding.Name = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_BASE_URL")); value != "" {
		cfg.Models.Main.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_PROVIDER")); value != "" {
		cfg.Models.Summary.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_BASE_URL")); value != "" {
		cfg.Models.Summary.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_API")); value != "" {
		cfg.Models.Main.API = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MODEL_SUMMARY_API")); value != "" {
		cfg.Models.Summary.API = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RPC_ADDRESS")); value != "" {
		cfg.RPC.Address = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RPC_PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.RPC.Port = port
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_GATEWAY_ENABLED"); ok {
		cfg.Gateway.Enabled = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_ADDRESS")); value != "" {
		cfg.Gateway.Address = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.Gateway.Port = port
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_AUTH_TOKEN")); value != "" {
		cfg.Gateway.AuthToken = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_PAIRING_SECRET")); value != "" {
		cfg.Gateway.PairingSecret = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_ALLOWED_USERS")); value != "" {
		cfg.Gateway.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_GATEWAY_TELEGRAM_ENABLED"); ok {
		cfg.Gateway.Telegram.Enabled = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_TELEGRAM_MODE")); value != "" {
		cfg.Gateway.Telegram.Mode = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_TELEGRAM_BOT_TOKEN")); value != "" {
		cfg.Gateway.Telegram.BotToken = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET")); value != "" {
		cfg.Gateway.Telegram.WebhookSecret = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_TELEGRAM_ALLOWED_USERS")); value != "" {
		cfg.Gateway.Telegram.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_GATEWAY_SLACK_ENABLED"); ok {
		cfg.Gateway.Slack.Enabled = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_SLACK_MODE")); value != "" {
		cfg.Gateway.Slack.Mode = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_SLACK_BOT_TOKEN")); value != "" {
		cfg.Gateway.Slack.BotToken = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_SLACK_APP_TOKEN")); value != "" {
		cfg.Gateway.Slack.AppToken = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_GATEWAY_SLACK_SIGNING_SECRET")); value != "" {
		cfg.Gateway.Slack.SigningSecret = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_MAX_ITERATIONS")); value != "" {
		if maxIterations, err := strconv.Atoi(value); err == nil {
			cfg.Session.MaxIterations = maxIterations
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_LOG_LEVEL")); value != "" {
		cfg.Log.Level = value
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_LOG_NO_COLOR"))); value != "" {
		cfg.Log.NoColor = value == "1" || value == "true" || value == "yes"
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_DEBUG_REQUESTS"))); value != "" {
		cfg.Debug.Requests = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_INPUT"); ok {
		cfg.Safety.Input = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_OUTPUT"); ok {
		cfg.Safety.Output = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SAFETY_PII"); ok {
		cfg.Safety.PII = new(value)
	}
	if value := strings.TrimSpace(strings.ToLower(os.Getenv("HAND_TRACE_ENABLED"))); value != "" {
		cfg.Trace.Enabled = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("HAND_TRACE_DISK_ENABLED"); ok {
		cfg.Trace.Disk.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_TRACE_DISK_DIR")); value != "" {
		cfg.Trace.Disk.Dir = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_TRACE_DATABASE_ENABLED"); ok {
		cfg.Trace.Database.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION")); value != "" {
		if maxEvents, err := strconv.Atoi(value); err == nil {
			cfg.Trace.Database.MaxEventsPerSession = maxEvents
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_TUI_THINKING_COMPOSER"); ok {
		cfg.TUI.ThinkingComposer = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_PROVIDER")); value != "" {
		cfg.Web.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_API_KEY")); value != "" {
		cfg.Web.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BASE_URL")); value != "" {
		cfg.Web.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_EXTRACT_CHAR_PER_RESULT")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractCharPerResult = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_MAX_EXTRACT_RESPONSE_BYTES")); value != "" {
		if bytes, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractResponseBytes = bytes
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_CACHE_TTL")); value != "" {
		cfg.Web.CacheTTL = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_WEB_BLOCKED_DOMAINS_ENABLED"); ok {
		cfg.Web.BlockedDomainsEnabled = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BLOCKED_DOMAINS")); value != "" {
		cfg.Web.BlockedDomains = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_BLOCKED_DOMAIN_FILES")); value != "" {
		cfg.Web.BlockedDomainFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_ALLOWED_HOSTS")); value != "" {
		cfg.Web.NativeAllowedHosts = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_BLOCKED_HOSTS")); value != "" {
		cfg.Web.NativeBlockedHosts = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_ALLOWED_HOST_FILES")); value != "" {
		cfg.Web.NativeAllowedHostFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_NATIVE_BLOCKED_HOST_FILES")); value != "" {
		cfg.Web.NativeBlockedHostFiles = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MIN_SUMMARIZE_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMinSummarizeChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MAX_SUMMARY_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChunkChars = chars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS")); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractRefusalThresholdChars = chars
		}
	}
	if cfg.Web.Provider == "" {
		switch {
		case strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_KEY")) != "" || strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_URL")) != "":
			cfg.Web.Provider = constants.WebProviderFirecrawl
		case strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderParallel
		case strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderTavily
		case strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY")) != "":
			cfg.Web.Provider = constants.WebProviderExa
		}
	}
	if cfg.Web.APIKey == "" {
		switch strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) {
		case constants.WebProviderFirecrawl:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_KEY"))
		case constants.WebProviderParallel:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_PARALLEL_API_KEY"))
		case constants.WebProviderTavily:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_TAVILY_API_KEY"))
		case constants.WebProviderExa:
			cfg.Web.APIKey = strings.TrimSpace(os.Getenv("HAND_EXA_API_KEY"))
		}
	}
	if cfg.Web.BaseURL == "" && strings.TrimSpace(strings.ToLower(cfg.Web.Provider)) == constants.WebProviderFirecrawl {
		cfg.Web.BaseURL = strings.TrimSpace(os.Getenv("HAND_FIRECRAWL_API_URL"))
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RULES_FILES")); value != "" {
		cfg.Rules.Files = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_INSTRUCT")); value != "" {
		cfg.Session.Instruct = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_PLATFORM")); value != "" {
		cfg.Platform = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_FS_ROOTS")); value != "" {
		cfg.FS.Roots = splitAndTrimCSV(value)
	}

	if value, ok := parseOptionalBoolEnv("HAND_CAP_FS"); ok {
		cfg.Cap.Filesystem = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_NET"); ok {
		cfg.Cap.Network = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_EXEC"); ok {
		cfg.Cap.Exec = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_MEM"); ok {
		cfg.Cap.Memory = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_CAP_BROWSER"); ok {
		cfg.Cap.Browser = new(value)
	}

	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_ALLOW")); value != "" {
		cfg.Exec.Allow = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_ASK")); value != "" {
		cfg.Exec.Ask = splitAndTrimCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_EXEC_DENY")); value != "" {
		cfg.Exec.Deny = splitAndTrimCSV(value)
	}

	if value := strings.TrimSpace(os.Getenv("HAND_STORAGE_BACKEND")); value != "" {
		cfg.Storage.Backend = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_DEFAULT_IDLE_EXPIRY")); value != "" {
		cfg.Session.DefaultIdleExpiry = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SESSION_ARCHIVE_RETENTION")); value != "" {
		cfg.Session.ArchiveRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_VECTOR_ENABLED"); ok {
		cfg.Search.Vector.Enabled = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_ENABLED"); ok {
		cfg.Memory.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROVIDER")); value != "" {
		cfg.Memory.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_BACKEND")); value != "" {
		cfg.Memory.Backend = value
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_PINNED_ENABLED"); ok {
		cfg.Memory.Pinned.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_RETRIEVAL_ENABLED"); ok {
		cfg.Memory.Retrieval.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_FLUSH_ENABLED"); ok {
		cfg.Memory.Flush.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_MAX_CALLS")); value != "" {
		if maxCalls, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Flush.MaxCalls = maxCalls
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_MAX_OUTPUT_TOKENS")); value != "" {
		if maxOutputTokens, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.Memory.Flush.MaxOutputTokens = maxOutputTokens
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_FLUSH_TIMEOUT")); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			cfg.Memory.Flush.Timeout = timeout
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PINNED_MAX_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxChars = maxChars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PINNED_MAX_ITEM_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxItemChars = maxChars
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_EPISODIC_ENABLED"); ok {
		cfg.Memory.Episodic.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_INTERVAL")); value != "" {
		cfg.Memory.Episodic.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_IDLE_AFTER")); value != "" {
		cfg.Memory.Episodic.IdleAfter = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MIN_MESSAGES")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MinMessages = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_WINDOW_SIZE")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.WindowSize = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOWS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindows = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowChars = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowTokens = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_EPISODIC_MAX_RETRIES")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxRetries = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_REFLECTION_ENABLED"); ok {
		cfg.Memory.Reflection.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_INTERVAL")); value != "" {
		cfg.Memory.Reflection.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.Limit = count
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_REFLECTION_RELATED_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.RelatedLimit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_PROMOTION_ENABLED"); ok {
		cfg.Memory.Promotion.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROMOTION_INTERVAL")); value != "" {
		cfg.Memory.Promotion.Interval = parseDurationOrZero(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_MEMORY_PROMOTION_LIMIT")); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Promotion.Limit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_MEMORY_WRITE_ENABLED"); ok {
		cfg.Memory.Write.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_VECTOR_REQUIRED"); ok {
		cfg.Search.Vector.Required = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE")); value != "" {
		if batchSize, err := strconv.Atoi(value); err == nil {
			cfg.Search.Vector.RebuildBatchSize = batchSize
		}
	}
	if value, ok := parseOptionalBoolEnv("HAND_RERANKER_ENABLED"); ok {
		cfg.Reranker.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("HAND_SEARCH_ENABLE_RERANK"); ok {
		cfg.Search.EnableRerank = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_TYPE")); value != "" {
		cfg.Reranker.Type = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MODEL")); value != "" {
		cfg.Reranker.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_CANDIDATES")); value != "" {
		if maxCandidates, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidates = maxCandidates
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS")); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidateTextChars = maxChars
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_MAX_OUTPUT_TOKENS")); value != "" {
		if maxTokens, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxOutputTokens = maxTokens
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_RERANKER_OVERRIDES")); value != "" {
		var overrides map[string]RerankerOverrideConfig
		if err := json.Unmarshal([]byte(value), &overrides); err == nil {
			cfg.Reranker.Overrides = overrides
		}
	}

	if value, ok := parseOptionalBoolEnv("HAND_COMPACTION_ENABLED"); ok {
		cfg.Compaction.Enabled = new(value)
	}
	if value := strings.TrimSpace(os.Getenv("HAND_COMPACTION_TRIGGER_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.TriggerPercent = percent
		}
	}
	if value := strings.TrimSpace(os.Getenv("HAND_COMPACTION_WARN_PERCENT")); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.WarnPercent = percent
		}
	}
}

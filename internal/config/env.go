package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/stringx"
)

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if value := stringx.String(os.Getenv("MORPH_NAME")).Trim(); value != "" {
		cfg.Name = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL")).Trim(); value != "" {
		cfg.Models.Main.Name = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_SUMMARY")).Trim(); value != "" {
		cfg.Models.Summary.Name = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MODEL_STREAM"); ok {
		cfg.Models.Main.Stream = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_CONTEXT_LENGTH")).Trim(); value != "" {
		if contextLength, err := strconv.Atoi(value); err == nil {
			cfg.Models.Main.ContextLength = contextLength
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_MAX_RETRIES")).Trim(); value != "" {
		if retries, err := strconv.Atoi(value); err == nil {
			cfg.Models.MaxRetries = &retries
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_PROVIDER")).Trim(); value != "" {
		cfg.Models.Main.Provider = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_EMBEDDING_PROVIDER")).Trim(); value != "" {
		cfg.Models.Embedding.Provider = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_EMBEDDING_MODEL")).Trim(); value != "" {
		cfg.Models.Embedding.Name = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_BASE_URL")).Trim(); value != "" {
		cfg.Models.Main.BaseURL = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_SUMMARY_PROVIDER")).Trim(); value != "" {
		cfg.Models.Summary.Provider = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_SUMMARY_BASE_URL")).Trim(); value != "" {
		cfg.Models.Summary.BaseURL = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_API")).Trim(); value != "" {
		cfg.Models.Main.API = value
	}
	if value := stringx.String(os.Getenv("MORPH_MODEL_SUMMARY_API")).Trim(); value != "" {
		cfg.Models.Summary.API = value
	}
	if value := stringx.String(os.Getenv("MORPH_RPC_ADDRESS")).Trim(); value != "" {
		cfg.RPC.Address = value
	}
	if value := stringx.String(os.Getenv("MORPH_RPC_PORT")).Trim(); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.RPC.Port = port
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_ENABLED"); ok {
		cfg.Gateway.Enabled = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_ADDRESS")).Trim(); value != "" {
		cfg.Gateway.Address = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_PORT")).Trim(); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.Gateway.Port = port
		}
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_AUTH_TOKEN")).Trim(); value != "" {
		cfg.Gateway.AuthToken = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_PAIRING_SECRET")).Trim(); value != "" {
		cfg.Gateway.PairingSecret = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_ALLOWED_USERS")).Trim(); value != "" {
		cfg.Gateway.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_TELEGRAM_ENABLED"); ok {
		cfg.Gateway.Telegram.Enabled = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_MODE")).Trim(); value != "" {
		cfg.Gateway.Telegram.Mode = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_BOT_TOKEN")).Trim(); value != "" {
		cfg.Gateway.Telegram.BotToken = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_WEBHOOK_SECRET")).Trim(); value != "" {
		cfg.Gateway.Telegram.WebhookSecret = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_ALLOWED_USERS")).Trim(); value != "" {
		cfg.Gateway.Telegram.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_SLACK_ENABLED"); ok {
		cfg.Gateway.Slack.Enabled = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_MODE")).Trim(); value != "" {
		cfg.Gateway.Slack.Mode = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_RESPONSE_MODE")).Trim(); value != "" {
		cfg.Gateway.Slack.ResponseMode = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_BOT_TOKEN")).Trim(); value != "" {
		cfg.Gateway.Slack.BotToken = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_APP_TOKEN")).Trim(); value != "" {
		cfg.Gateway.Slack.AppToken = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_SIGNING_SECRET")).Trim(); value != "" {
		cfg.Gateway.Slack.SigningSecret = value
	}
	if value := stringx.String(os.Getenv("MORPH_GATEWAY_SLACK_ALLOWED_USERS")).Trim(); value != "" {
		cfg.Gateway.Slack.AllowedUsers = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_SESSION_MAX_ITERATIONS")).Trim(); value != "" {
		if maxIterations, err := strconv.Atoi(value); err == nil {
			cfg.Session.MaxIterations = maxIterations
		}
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_LEVEL")).Trim(); value != "" {
		cfg.Log.Level = value
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_FILE")).Trim(); value != "" {
		cfg.Log.File = value
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_MAX_SIZE_MB")).Trim(); value != "" {
		if maxSizeMB, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxSizeMB = maxSizeMB
		}
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_MAX_BACKUPS")).Trim(); value != "" {
		if maxBackups, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxBackups = maxBackups
		}
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_MAX_AGE_DAYS")).Trim(); value != "" {
		if maxAgeDays, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxAgeDays = maxAgeDays
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_LOG_COMPRESS"); ok {
		cfg.Log.Compress = value
	}
	if value := stringx.String(os.Getenv("MORPH_LOG_NO_COLOR")).Normalized(); value != "" {
		cfg.Log.NoColor = value == "1" || value == "true" || value == "yes"
	}
	if value := stringx.String(os.Getenv("MORPH_DEBUG_REQUESTS")).Normalized(); value != "" {
		cfg.Debug.Requests = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_INPUT"); ok {
		cfg.Safety.Input = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_OUTPUT"); ok {
		cfg.Safety.Output = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_PII"); ok {
		cfg.Safety.PII = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_TRACE_ENABLED")).Normalized(); value != "" {
		cfg.Trace.Enabled = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TRACE_DISK_ENABLED"); ok {
		cfg.Trace.Disk.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_TRACE_DISK_DIR")).Trim(); value != "" {
		cfg.Trace.Disk.Dir = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TRACE_DATABASE_ENABLED"); ok {
		cfg.Trace.Database.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_TRACE_DATABASE_MAX_EVENTS_PER_SESSION")).Trim(); value != "" {
		if maxEvents, err := strconv.Atoi(value); err == nil {
			cfg.Trace.Database.MaxEventsPerSession = maxEvents
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TUI_THINKING_COMPOSER"); ok {
		cfg.TUI.ThinkingComposer = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_PROVIDER")).Trim(); value != "" {
		cfg.Web.Provider = value
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_API_KEY")).Trim(); value != "" {
		cfg.Web.APIKey = value
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_BASE_URL")).Trim(); value != "" {
		cfg.Web.BaseURL = value
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_MAX_CHAR_PER_RESULT")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxCharPerResult = chars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_MAX_EXTRACT_CHAR_PER_RESULT")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractCharPerResult = chars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_MAX_EXTRACT_RESPONSE_BYTES")).Trim(); value != "" {
		if bytes, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractResponseBytes = bytes
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_CACHE_TTL")).Trim(); value != "" {
		cfg.Web.CacheTTL = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_WEB_BLOCKED_DOMAINS_ENABLED"); ok {
		cfg.Web.BlockedDomainsEnabled = value
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_BLOCKED_DOMAINS")).Trim(); value != "" {
		cfg.Web.BlockedDomains = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_BLOCKED_DOMAIN_FILES")).Trim(); value != "" {
		cfg.Web.BlockedDomainFiles = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_NATIVE_ALLOWED_HOSTS")).Trim(); value != "" {
		cfg.Web.NativeAllowedHosts = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_NATIVE_BLOCKED_HOSTS")).Trim(); value != "" {
		cfg.Web.NativeBlockedHosts = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_NATIVE_ALLOWED_HOST_FILES")).Trim(); value != "" {
		cfg.Web.NativeAllowedHostFiles = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_NATIVE_BLOCKED_HOST_FILES")).Trim(); value != "" {
		cfg.Web.NativeBlockedHostFiles = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_EXTRACT_MIN_SUMMARIZE_CHARS")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMinSummarizeChars = chars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_EXTRACT_MAX_SUMMARY_CHARS")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChars = chars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChunkChars = chars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS")).Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractRefusalThresholdChars = chars
		}
	}
	if cfg.Web.Provider == "" {
		switch {
		case stringx.String(os.Getenv("MORPH_FIRECRAWL_API_KEY")).Trim() != "" || stringx.String(os.Getenv("MORPH_FIRECRAWL_API_URL")).Trim() != "":
			cfg.Web.Provider = constants.WebProviderFirecrawl
		case stringx.String(os.Getenv("MORPH_PARALLEL_API_KEY")).Trim() != "":
			cfg.Web.Provider = constants.WebProviderParallel
		case stringx.String(os.Getenv("MORPH_TAVILY_API_KEY")).Trim() != "":
			cfg.Web.Provider = constants.WebProviderTavily
		case stringx.String(os.Getenv("MORPH_EXA_API_KEY")).Trim() != "":
			cfg.Web.Provider = constants.WebProviderExa
		}
	}
	if cfg.Web.APIKey == "" {
		switch stringx.String(cfg.Web.Provider).Normalized() {
		case constants.WebProviderFirecrawl:
			cfg.Web.APIKey = stringx.String(os.Getenv("MORPH_FIRECRAWL_API_KEY")).Trim()
		case constants.WebProviderParallel:
			cfg.Web.APIKey = stringx.String(os.Getenv("MORPH_PARALLEL_API_KEY")).Trim()
		case constants.WebProviderTavily:
			cfg.Web.APIKey = stringx.String(os.Getenv("MORPH_TAVILY_API_KEY")).Trim()
		case constants.WebProviderExa:
			cfg.Web.APIKey = stringx.String(os.Getenv("MORPH_EXA_API_KEY")).Trim()
		}
	}
	if cfg.Web.BaseURL == "" && stringx.String(cfg.Web.Provider).Normalized() == constants.WebProviderFirecrawl {
		cfg.Web.BaseURL = stringx.String(os.Getenv("MORPH_FIRECRAWL_API_URL")).Trim()
	}
	if value := stringx.String(os.Getenv("MORPH_RULES_FILES")).Trim(); value != "" {
		cfg.Rules.Files = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_SESSION_INSTRUCT")).Trim(); value != "" {
		cfg.Session.Instruct = value
	}
	if value := stringx.String(os.Getenv("MORPH_PLATFORM")).Trim(); value != "" {
		cfg.Platform = value
	}
	if value := stringx.String(os.Getenv("MORPH_FS_ROOTS")).Trim(); value != "" {
		cfg.FS.Roots = splitAndTrimCSV(value)
	}

	if value, ok := parseOptionalBoolEnv("MORPH_CAP_FS"); ok {
		cfg.Cap.Filesystem = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_NET"); ok {
		cfg.Cap.Network = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_EXEC"); ok {
		cfg.Cap.Exec = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_MEM"); ok {
		cfg.Cap.Memory = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_BROWSER"); ok {
		cfg.Cap.Browser = new(value)
	}

	if value := stringx.String(os.Getenv("MORPH_EXEC_ALLOW")).Trim(); value != "" {
		cfg.Exec.Allow = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_EXEC_ASK")).Trim(); value != "" {
		cfg.Exec.Ask = splitAndTrimCSV(value)
	}
	if value := stringx.String(os.Getenv("MORPH_EXEC_DENY")).Trim(); value != "" {
		cfg.Exec.Deny = splitAndTrimCSV(value)
	}

	if value := stringx.String(os.Getenv("MORPH_STORAGE_BACKEND")).Trim(); value != "" {
		cfg.Storage.Backend = value
	}
	if value := stringx.String(os.Getenv("MORPH_SESSION_DEFAULT_IDLE_EXPIRY")).Trim(); value != "" {
		cfg.Session.DefaultIdleExpiry = parseDurationOrZero(value)
	}
	if value := stringx.String(os.Getenv("MORPH_SESSION_ARCHIVE_RETENTION")).Trim(); value != "" {
		cfg.Session.ArchiveRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_VECTOR_ENABLED"); ok {
		cfg.Search.Vector.Enabled = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_ENABLED"); ok {
		cfg.Memory.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PROVIDER")).Trim(); value != "" {
		cfg.Memory.Provider = value
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_BACKEND")).Trim(); value != "" {
		cfg.Memory.Backend = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_PINNED_ENABLED"); ok {
		cfg.Memory.Pinned.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_RETRIEVAL_ENABLED"); ok {
		cfg.Memory.Retrieval.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_FLUSH_ENABLED"); ok {
		cfg.Memory.Flush.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_FLUSH_MAX_CALLS")).Trim(); value != "" {
		if maxCalls, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Flush.MaxCalls = maxCalls
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_FLUSH_MAX_OUTPUT_TOKENS")).Trim(); value != "" {
		if maxOutputTokens, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.Memory.Flush.MaxOutputTokens = maxOutputTokens
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_FLUSH_TIMEOUT")).Trim(); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			cfg.Memory.Flush.Timeout = timeout
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PINNED_MAX_CHARS")).Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxChars = maxChars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PINNED_MAX_ITEM_CHARS")).Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxItemChars = maxChars
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_EPISODIC_ENABLED"); ok {
		cfg.Memory.Episodic.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_INTERVAL")).Trim(); value != "" {
		cfg.Memory.Episodic.Interval = parseDurationOrZero(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_IDLE_AFTER")).Trim(); value != "" {
		cfg.Memory.Episodic.IdleAfter = parseDurationOrZero(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_MIN_MESSAGES")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MinMessages = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_WINDOW_SIZE")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.WindowSize = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOWS")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindows = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOW_CHARS")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowChars = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOW_TOKENS")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowTokens = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_RETRIES")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxRetries = count
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_REFLECTION_ENABLED"); ok {
		cfg.Memory.Reflection.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_REFLECTION_INTERVAL")).Trim(); value != "" {
		cfg.Memory.Reflection.Interval = parseDurationOrZero(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_REFLECTION_LIMIT")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.Limit = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_REFLECTION_RELATED_LIMIT")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.RelatedLimit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_PROMOTION_ENABLED"); ok {
		cfg.Memory.Promotion.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PROMOTION_INTERVAL")).Trim(); value != "" {
		cfg.Memory.Promotion.Interval = parseDurationOrZero(value)
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PROMOTION_LIMIT")).Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Promotion.Limit = count
		}
	}
	if value := stringx.String(os.Getenv("MORPH_MEMORY_PROMOTION_EVALUATED_RETENTION")).Trim(); value != "" {
		cfg.Memory.Promotion.EvaluatedRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_WRITE_ENABLED"); ok {
		cfg.Memory.Write.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_VECTOR_REQUIRED"); ok {
		cfg.Search.Vector.Required = value
	}
	if value := stringx.String(os.Getenv("MORPH_SEARCH_VECTOR_REBUILD_BATCH_SIZE")).Trim(); value != "" {
		if batchSize, err := strconv.Atoi(value); err == nil {
			cfg.Search.Vector.RebuildBatchSize = batchSize
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_RERANKER_ENABLED"); ok {
		cfg.Reranker.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_ENABLE_RERANK"); ok {
		cfg.Search.EnableRerank = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_TYPE")).Trim(); value != "" {
		cfg.Reranker.Type = value
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_MODEL")).Trim(); value != "" {
		cfg.Reranker.Model = value
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_MAX_CANDIDATES")).Trim(); value != "" {
		if maxCandidates, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidates = maxCandidates
		}
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_MAX_CANDIDATE_TEXT_CHARS")).Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidateTextChars = maxChars
		}
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_MAX_OUTPUT_TOKENS")).Trim(); value != "" {
		if maxTokens, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxOutputTokens = maxTokens
		}
	}
	if value := stringx.String(os.Getenv("MORPH_RERANKER_OVERRIDES")).Trim(); value != "" {
		var overrides map[string]RerankerOverrideConfig
		if err := json.Unmarshal([]byte(value), &overrides); err == nil {
			cfg.Reranker.Overrides = overrides
		}
	}

	if value, ok := parseOptionalBoolEnv("MORPH_COMPACTION_ENABLED"); ok {
		cfg.Compaction.Enabled = new(value)
	}
	if value := stringx.String(os.Getenv("MORPH_COMPACTION_TRIGGER_PERCENT")).Trim(); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.TriggerPercent = percent
		}
	}
	if value := stringx.String(os.Getenv("MORPH_COMPACTION_WARN_PERCENT")).Trim(); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.WarnPercent = percent
		}
	}
}
